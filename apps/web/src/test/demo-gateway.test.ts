import { GpuAvailability, OrderStatus } from "@gpu-rental/contracts";
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
    const now = new Date("2026-07-13T08:00:00.000Z");
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
    });
    expect(order.status).toBe(OrderStatus.Active);

    const updated = await gateway.returnOrder(order.id);
    expect(updated.status).toBe(OrderStatus.Returned);
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
  const key = "gpu-rental-demo-state-v1";
  const state = JSON.parse(storage.getItem(key)!) as {
    currentUserId: string | null;
  };
  state.currentUserId = userId;
  storage.setItem(key, JSON.stringify(state));
}
