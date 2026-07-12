import type { INestApplication } from "@nestjs/common";
import { ValidationPipe } from "@nestjs/common";
import { Test } from "@nestjs/testing";
import { getConnectionToken } from "@nestjs/mongoose";
import { GpuListingStatus, OrderStatus } from "@gpu-rental/contracts";
import cookieParser from "cookie-parser";
import { Types, type Connection } from "mongoose";
import request from "supertest";
import { afterAll, beforeAll, describe, expect, it } from "vitest";

import { AppModule } from "../src/app.module";
import { GpuResourcesService } from "../src/gpu-resources/gpu-resources.service";

function getSessionCookie(response: request.Response): string {
  const header = response.headers["set-cookie"];
  const cookie = Array.isArray(header) ? header[0] : header;
  if (!cookie) {
    throw new Error("Authentication response did not set a session cookie");
  }
  return cookie.split(";", 1)[0] ?? cookie;
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

    mongo = app.get<Connection>(getConnectionToken());
    if (!/(_ci|_test)$/.test(mongo.name)) {
      throw new Error(
        `Refusing to clean non-test MongoDB database: ${mongo.name}`,
      );
    }
    await Promise.all([
      mongo.collection("orders").deleteMany({}),
      mongo.collection("gpu_resources").deleteMany({}),
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
    const resource = await app.get(GpuResourcesService).create({
      name: "ci-a100-01",
      model: "NVIDIA A100",
      memoryGb: 80,
      region: "ci-region",
      hourlyPriceCents: 2000,
      tags: ["80GB"],
      listingStatus: GpuListingStatus.Online,
    });

    const responses = await Promise.all(
      Array.from({ length: 20 }, () =>
        request(app.getHttpServer())
          .post("/api/orders")
          .set("Cookie", cookie)
          .send({ gpuResourceId: resource.id, durationHours: 2 }),
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
        gpuResourceId: new Types.ObjectId(resource.id),
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
    const resource = await app.get(GpuResourcesService).create({
      name: "ci-4090-02",
      model: "NVIDIA RTX 4090",
      memoryGb: 24,
      region: "ci-region",
      hourlyPriceCents: 600,
      listingStatus: GpuListingStatus.Online,
    });
    const order = await request(app.getHttpServer())
      .post("/api/orders")
      .set("Cookie", ownerCookie)
      .send({ gpuResourceId: resource.id, durationHours: 1 })
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
