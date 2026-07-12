import { OrderStatus } from "@gpu-rental/contracts";
import { describe, expect, it } from "vitest";

import { OrderSchema } from "./order.schema";

describe("OrderSchema", () => {
  it("enforces one active order per GPU without restricting terminal history", () => {
    const index = OrderSchema.indexes().find(
      ([fields]) =>
        fields.gpuResourceId === 1 && Object.keys(fields).length === 1,
    );

    expect(index?.[1]).toMatchObject({
      unique: true,
      partialFilterExpression: { status: OrderStatus.Active },
    });
  });
});
