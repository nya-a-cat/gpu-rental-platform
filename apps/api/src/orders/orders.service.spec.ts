import { OrderStatus } from "@gpu-rental/contracts";
import { Types, type Model } from "mongoose";
import { describe, expect, it, vi } from "vitest";

import type { GpuResource } from "../gpu-resources/gpu-resource.schema";
import type { DistributedLockService } from "../redis/distributed-lock.service";
import { Order, type OrderDocument } from "./order.schema";
import { OrdersService } from "./orders.service";

function orderRecord(status: OrderStatus): OrderDocument {
  const now = new Date("2026-01-01T00:00:00.000Z");
  return {
    _id: new Types.ObjectId(),
    userId: new Types.ObjectId(),
    gpuResourceId: new Types.ObjectId(),
    gpuName: "cn-east-a100-01",
    gpuModel: "NVIDIA A100",
    gpuMemoryGb: 80,
    region: "cn-east",
    hourlyPriceCents: 2200,
    durationHours: 2,
    totalPriceCents: 4400,
    status,
    startsAt: now,
    endsAt: new Date(now.getTime() + 7_200_000),
    returnedAt: status === OrderStatus.Returned ? now : null,
    cancelledAt: status === OrderStatus.Cancelled ? now : null,
    createdAt: now,
    updatedAt: now,
  } as unknown as OrderDocument;
}

describe("OrdersService terminal transitions", () => {
  it("treats repeated returns as idempotent", async () => {
    const existing = orderRecord(OrderStatus.Returned);
    const orderModel = {
      findOneAndUpdate: vi.fn().mockReturnValue({
        exec: vi.fn().mockResolvedValue(null),
      }),
      findOne: vi.fn().mockReturnValue({
        exec: vi.fn().mockResolvedValue(existing),
      }),
    };
    const service = new OrdersService(
      orderModel as unknown as Model<Order>,
      {} as unknown as Model<GpuResource>,
      {} as unknown as DistributedLockService,
    );

    await expect(
      service.returnOrder(existing._id.toString(), existing.userId.toString()),
    ).resolves.toMatchObject({ status: OrderStatus.Returned });
  });

  it("rejects a return after another terminal transition", async () => {
    const existing = orderRecord(OrderStatus.Expired);
    const orderModel = {
      findOneAndUpdate: vi.fn().mockReturnValue({
        exec: vi.fn().mockResolvedValue(null),
      }),
      findOne: vi.fn().mockReturnValue({
        exec: vi.fn().mockResolvedValue(existing),
      }),
    };
    const service = new OrdersService(
      orderModel as unknown as Model<Order>,
      {} as unknown as Model<GpuResource>,
      {} as unknown as DistributedLockService,
    );

    await expect(
      service.returnOrder(existing._id.toString(), existing.userId.toString()),
    ).rejects.toMatchObject({ code: "ORDER_ALREADY_TERMINAL" });
  });
});
