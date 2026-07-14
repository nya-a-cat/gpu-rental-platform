import type { INestApplication } from "@nestjs/common";
import { ValidationPipe } from "@nestjs/common";
import { Test } from "@nestjs/testing";
import { getConnectionToken } from "@nestjs/mongoose";
import {
  BillingEntryType,
  GpuListingStatus,
  InstanceStatus,
  NetworkProtocol,
  OrderStatus,
  ResourceMode,
  TeamRole,
  VolumeStatus,
} from "@gpu-rental/contracts";
import cookieParser from "cookie-parser";
import { Types, type Connection } from "mongoose";
import request from "supertest";
import { afterAll, beforeAll, describe, expect, it } from "vitest";

import { AppModule } from "../dist/app.module.js";

function getSessionCookie(response: request.Response): string {
  const header = response.headers["set-cookie"];
  const cookie = Array.isArray(header) ? header[0] : header;
  if (!cookie) {
    throw new Error("Authentication response did not set a session cookie");
  }
  return cookie.split(";", 1)[0] ?? cookie;
}

async function insertTestResource(
  mongo: Connection,
  resource: {
    name: string;
    model: string;
    memoryGb: number;
    region: string;
    hourlyPriceCents: number;
    tags?: string[];
  },
): Promise<string> {
  const timestamp = new Date();
  const result = await mongo.collection("gpu_resources").insertOne({
    ...resource,
    tags: resource.tags ?? [],
    listingStatus: GpuListingStatus.Online,
    resourceMode: ResourceMode.Simulated,
    createdAt: timestamp,
    updatedAt: timestamp,
  });
  return result.insertedId.toString();
}

describe("API with MongoDB and Redis", () => {
  let app: INestApplication;
  let mongo: Connection;

  beforeAll(async () => {
    const module = await Test.createTestingModule({
      imports: [AppModule],
    }).compile();
    app = module.createNestApplication();
    app.use(cookieParser());
    app.useGlobalPipes(
      new ValidationPipe({
        forbidNonWhitelisted: true,
        transform: true,
        whitelist: true,
      }),
    );
    app.setGlobalPrefix("api");
    await app.init();
    await app.listen(0, "127.0.0.1");

    mongo = app.get<Connection>(getConnectionToken());
    if (!/(_ci|_test)$/.test(mongo.name)) {
      throw new Error(
        `Refusing to clean non-test MongoDB database: ${mongo.name}`,
      );
    }
    await Promise.all([
      mongo.collection("orders").deleteMany({}),
      mongo.collection("gpu_resources").deleteMany({}),
      mongo.collection("instances").deleteMany({}),
      mongo.collection("cloud_accounts").deleteMany({}),
      mongo.collection("teams").deleteMany({}),
      mongo.collection("users").deleteMany({}),
    ]);
  });

  afterAll(async () => {
    await app?.close();
  });

  it("allows exactly one of 20 concurrent reservations", async () => {
    const registration = await request(app.getHttpServer())
      .post("/api/auth/register")
      .send({ username: "concurrency_user", password: "secure-password" })
      .expect(201);
    const cookie = getSessionCookie(registration);
    const resourceId = await insertTestResource(mongo, {
      name: "ci-a100-01",
      model: "NVIDIA A100",
      memoryGb: 80,
      region: "ci-region",
      hourlyPriceCents: 2000,
      tags: ["80GB"],
    });

    const responses = await Promise.all(
      Array.from({ length: 20 }, () =>
        request(app.getHttpServer())
          .post("/api/orders")
          .set("Cookie", cookie)
          .send({ gpuResourceId: resourceId, durationHours: 2 }),
      ),
    );
    expect(
      responses.filter((response) => response.status === 201),
    ).toHaveLength(1);
    expect(
      responses.filter((response) => response.status === 409),
    ).toHaveLength(19);
    expect(
      await mongo.collection("orders").countDocuments({
        gpuResourceId: new Types.ObjectId(resourceId),
        status: OrderStatus.Active,
      }),
    ).toBe(1);
  });

  it("revokes the current cookie through logout-all", async () => {
    const registration = await request(app.getHttpServer())
      .post("/api/auth/register")
      .send({ username: "session_user", password: "secure-password" })
      .expect(201);
    const cookie = getSessionCookie(registration);

    await request(app.getHttpServer())
      .get("/api/auth/me")
      .set("Cookie", cookie)
      .expect(200);
    await request(app.getHttpServer())
      .post("/api/auth/logout-all")
      .set("Cookie", cookie)
      .expect(204);
    await request(app.getHttpServer())
      .get("/api/auth/me")
      .set("Cookie", cookie)
      .expect(401);
  });

  it("creates and manages a simulated instance for an order", async () => {
    const registration = await request(app.getHttpServer())
      .post("/api/auth/register")
      .send({ username: "instance_user", password: "secure-password" })
      .expect(201);
    const cookie = getSessionCookie(registration);
    const resourceId = await insertTestResource(mongo, {
      name: "ci-h100-instance-01",
      model: "NVIDIA H100",
      memoryGb: 80,
      region: "ci-region",
      hourlyPriceCents: 3000,
    });

    const order = await request(app.getHttpServer())
      .post("/api/orders")
      .set("Cookie", cookie)
      .send({
        gpuResourceId: resourceId,
        durationHours: 4,
        environmentTemplateId: "cuda-development",
        instanceName: "ci-training-run",
      })
      .expect(201);

    const list = await request(app.getHttpServer())
      .get("/api/instances/me")
      .set("Cookie", cookie)
      .expect(200);
    const instance = list.body.items.find(
      (candidate: { orderId: string }) => candidate.orderId === order.body.id,
    ) as { id: string; status: InstanceStatus } | undefined;
    expect(instance?.status).toBe(InstanceStatus.Running);

    await request(app.getHttpServer())
      .post(`/api/instances/${instance!.id}/stop`)
      .set("Cookie", cookie)
      .expect(200)
      .expect(({ body }) => expect(body.status).toBe(InstanceStatus.Stopped));
    await request(app.getHttpServer())
      .post(`/api/instances/${instance!.id}/start`)
      .set("Cookie", cookie)
      .expect(200)
      .expect(({ body }) => expect(body.status).toBe(InstanceStatus.Running));
    await request(app.getHttpServer())
      .post(`/api/instances/${instance!.id}/terminate`)
      .set("Cookie", cookie)
      .expect(200)
      .expect(({ body }) =>
        expect(body.status).toBe(InstanceStatus.Terminated),
      );

    await request(app.getHttpServer())
      .get("/api/orders/me")
      .set("Cookie", cookie)
      .expect(200)
      .expect(({ body }) => {
        const returnedOrder = body.items.find(
          (candidate: { id: string }) => candidate.id === order.body.id,
        );
        expect(returnedOrder.status).toBe(OrderStatus.Returned);
      });
  });

  it("revokes every session after a password change", async () => {
    const registration = await request(app.getHttpServer())
      .post("/api/auth/register")
      .send({ username: "password_user", password: "original-password" })
      .expect(201);
    const cookie = getSessionCookie(registration);

    await request(app.getHttpServer())
      .patch("/api/users/me/password")
      .set("Cookie", cookie)
      .send({
        currentPassword: "original-password",
        newPassword: "replacement-password",
      })
      .expect(204);
    await request(app.getHttpServer())
      .get("/api/auth/me")
      .set("Cookie", cookie)
      .expect(401);
    await request(app.getHttpServer())
      .post("/api/auth/login")
      .send({ username: "password_user", password: "original-password" })
      .expect(401);
    await request(app.getHttpServer())
      .post("/api/auth/login")
      .send({ username: "password_user", password: "replacement-password" })
      .expect(200);
  });

  it("manages billing, access, storage, networking and project attribution", async () => {
    await request(app.getHttpServer())
      .post("/api/auth/register")
      .send({ username: "p1_member", password: "secure-password" })
      .expect(201);
    const registration = await request(app.getHttpServer())
      .post("/api/auth/register")
      .send({ username: "p1_owner", password: "secure-password" })
      .expect(201);
    const cookie = getSessionCookie(registration);

    await request(app.getHttpServer())
      .get("/api/cloud-account")
      .set("Cookie", cookie)
      .expect(200)
      .expect(({ body }) => expect(body.wallet.balanceCents).toBe(100_000));
    await request(app.getHttpServer())
      .post("/api/cloud-account/top-ups")
      .set("Cookie", cookie)
      .send({ amountCents: 5_000 })
      .expect(201)
      .expect(({ body }) => expect(body.wallet.balanceCents).toBe(105_000));
    await request(app.getHttpServer())
      .post("/api/cloud-account/ssh-keys")
      .set("Cookie", cookie)
      .send({
        name: "ci laptop",
        publicKey:
          "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAICIRentalPlatformKey p1",
      })
      .expect(201)
      .expect(({ body }) => expect(body.fingerprint).toMatch(/^SHA256:/));
    await request(app.getHttpServer())
      .post("/api/cloud-account/api-keys")
      .set("Cookie", cookie)
      .send({ name: "ci automation" })
      .expect(201)
      .expect(({ body }) => expect(body.token).toMatch(/^gpr_/));

    const team = await request(app.getHttpServer())
      .post("/api/teams")
      .set("Cookie", cookie)
      .send({ name: "CI research lab" })
      .expect(201);
    await request(app.getHttpServer())
      .post(`/api/teams/${team.body.id}/members`)
      .set("Cookie", cookie)
      .send({ username: "p1_member", role: TeamRole.Member })
      .expect(201);
    const projectTeam = await request(app.getHttpServer())
      .post(`/api/teams/${team.body.id}/projects`)
      .set("Cookie", cookie)
      .send({ name: "CI training", monthlyBudgetCents: 50_000 })
      .expect(201);
    const projectId = projectTeam.body.projects[0].id as string;

    const resourceId = await insertTestResource(mongo, {
      name: "ci-p1-operations-01",
      model: "NVIDIA L40S",
      memoryGb: 48,
      region: "ci-region",
      hourlyPriceCents: 1_000,
    });
    const order = await request(app.getHttpServer())
      .post("/api/orders")
      .set("Cookie", cookie)
      .send({
        gpuResourceId: resourceId,
        durationHours: 2,
        projectId,
        instanceName: "p1-ci-instance",
      })
      .expect(201);
    expect(order.body).toMatchObject({
      projectId,
      projectName: "CI training",
      teamName: "CI research lab",
      totalPriceCents: 2_000,
    });

    const instances = await request(app.getHttpServer())
      .get("/api/instances/me")
      .set("Cookie", cookie)
      .expect(200);
    const instance = instances.body.items.find(
      (candidate: { orderId: string }) => candidate.orderId === order.body.id,
    ) as { id: string };
    await request(app.getHttpServer())
      .post("/api/cloud-account/network-rules")
      .set("Cookie", cookie)
      .send({
        instanceId: instance.id,
        name: "tensorboard",
        protocol: NetworkProtocol.Tcp,
        port: 6006,
        sourceCidr: "0.0.0.0/0",
      })
      .expect(201);
    const volume = await request(app.getHttpServer())
      .post("/api/cloud-account/volumes")
      .set("Cookie", cookie)
      .send({ name: "ci-model-cache", sizeGb: 100 })
      .expect(201);
    await request(app.getHttpServer())
      .post(`/api/cloud-account/volumes/${volume.body.id}/attach`)
      .set("Cookie", cookie)
      .send({ instanceId: instance.id })
      .expect(201);
    await request(app.getHttpServer())
      .post(`/api/cloud-account/volumes/${volume.body.id}/snapshots`)
      .set("Cookie", cookie)
      .send({ name: "ci-checkpoint" })
      .expect(201);

    await request(app.getHttpServer())
      .post(`/api/instances/${instance.id}/terminate`)
      .set("Cookie", cookie)
      .expect(200);
    const account = await request(app.getHttpServer())
      .get("/api/cloud-account")
      .set("Cookie", cookie)
      .expect(200);
    expect(account.body.wallet.balanceCents).toBeGreaterThan(103_000);
    expect(
      account.body.billingEntries.some(
        (entry: { type: BillingEntryType }) =>
          entry.type === BillingEntryType.OrderRefund,
      ),
    ).toBe(true);
    expect(
      account.body.volumes.find(
        (candidate: { id: string }) => candidate.id === volume.body.id,
      ),
    ).toMatchObject({
      status: VolumeStatus.Available,
      attachedInstanceId: null,
    });

    await request(app.getHttpServer())
      .get("/api/teams/me")
      .set("Cookie", cookie)
      .expect(200)
      .expect(({ body }) =>
        expect(body[0].projects[0].bookedCostCents).toBe(2_000),
      );
    await request(app.getHttpServer())
      .post("/api/cloud-account/notifications/read-all")
      .set("Cookie", cookie)
      .expect(204);
  });

  it("allows only the owner to return an order and keeps retries idempotent", async () => {
    const ownerRegistration = await request(app.getHttpServer())
      .post("/api/auth/register")
      .send({ username: "order_owner", password: "secure-password" })
      .expect(201);
    const intruderRegistration = await request(app.getHttpServer())
      .post("/api/auth/register")
      .send({ username: "order_intruder", password: "secure-password" })
      .expect(201);
    const ownerCookie = getSessionCookie(ownerRegistration);
    const intruderCookie = getSessionCookie(intruderRegistration);
    const resourceId = await insertTestResource(mongo, {
      name: "ci-4090-02",
      model: "NVIDIA RTX 4090",
      memoryGb: 24,
      region: "ci-region",
      hourlyPriceCents: 600,
    });
    const order = await request(app.getHttpServer())
      .post("/api/orders")
      .set("Cookie", ownerCookie)
      .send({ gpuResourceId: resourceId, durationHours: 1 })
      .expect(201);
    const orderId = order.body.id as string;

    await request(app.getHttpServer())
      .post(`/api/orders/${orderId}/return`)
      .set("Cookie", intruderCookie)
      .expect(404);
    await request(app.getHttpServer())
      .post(`/api/orders/${orderId}/return`)
      .set("Cookie", ownerCookie)
      .expect(200);
    const retry = await request(app.getHttpServer())
      .post(`/api/orders/${orderId}/return`)
      .set("Cookie", ownerCookie)
      .expect(200);
    expect(retry.body.status).toBe(OrderStatus.Returned);
  });

  it("denies administration routes to a regular user", async () => {
    const registration = await request(app.getHttpServer())
      .post("/api/auth/register")
      .send({ username: "regular_user", password: "secure-password" })
      .expect(201);
    const cookie = getSessionCookie(registration);

    await request(app.getHttpServer())
      .get("/api/admin/overview")
      .set("Cookie", cookie)
      .expect(403);
  });
});
