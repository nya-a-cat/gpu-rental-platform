import {
  GpuAvailability,
  InstanceStatus,
  OrderStatus,
} from "@gpu-rental/contracts";
import { describe, expect, it } from "vitest";

import { DemoGateway, type StorageLike } from "../data/demo-gateway";

class MemoryStorage implements StorageLike {
  private readonly data = new Map<string, string>();
  getItem(key: string): string | null {
    return this.data.get(key) ?? null;
  }
  removeItem(key: string): void {
    this.data.delete(key);
  }
  setItem(key: string, value: string): void {
    this.data.set(key, value);
  }
}

describe("DemoGateway", () => {
  it("reserves one available resource and returns the order", async () => {
    const storage = new MemoryStorage();
    let now = new Date("2026-07-13T08:00:00.000Z");
    const gateway = new DemoGateway(storage, () => now);
    await gateway.resetDemo();
    setSession(storage, "demo-user");

    const resources = await gateway.listResources({
      availability: GpuAvailability.Available,
    });
    const resource = resources.items[0];
    expect(resource).toBeDefined();

    const order = await gateway.createOrder({
      gpuResourceId: resource!.id,
      durationHours: 4,
      environmentTemplateId: "cuda-development",
      instanceName: "gateway-test-instance",
    });
    expect(order.status).toBe(OrderStatus.Active);

    const instance = (await gateway.listMyInstances()).items.find(
      (candidate) => candidate.orderId === order.id,
    );
    expect(instance).toMatchObject({
      name: "gateway-test-instance",
      status: InstanceStatus.Running,
      environmentTemplateId: "cuda-development",
      simulated: true,
    });
    expect(instance!.access.sshCommand).toContain(".simulated.invalid");
    now = new Date("2026-07-13T09:30:00.000Z");
    const stopped = await gateway.stopInstance(instance!.id);
    expect(stopped.status).toBe(InstanceStatus.Stopped);
    expect(stopped.billableSeconds).toBe(5_400);
    expect(stopped.accruedCostCents).toBe(
      Math.ceil(resource!.hourlyPriceCents * 1.5),
    );
    const restarted = await gateway.startInstance(instance!.id);
    expect(restarted.status).toBe(InstanceStatus.Running);

    const updated = await gateway.returnOrder(order.id);
    expect(updated.status).toBe(OrderStatus.Returned);
    expect((await gateway.getInstance(instance!.id)).status).toBe(
      InstanceStatus.Terminated,
    );
    expect((await gateway.getResource(resource!.id)).availability).toBe(
      GpuAvailability.Available,
    );
  });

  it("rejects admin operations for an operator", async () => {
    const storage = new MemoryStorage();
    const gateway = new DemoGateway(storage);
    await gateway.resetDemo();
    setSession(storage, "demo-user");
    await expect(gateway.getAdminOverview()).rejects.toMatchObject({
      code: "FORBIDDEN",
      status: 403,
    });
  });
});

function setSession(storage: StorageLike, userId: string): void {
  const key = "gpu-rental-demo-state-v2";
  const state = JSON.parse(storage.getItem(key)!) as {
    currentUserId: string | null;
  };
  state.currentUserId = userId;
  storage.setItem(key, JSON.stringify(state));
}
