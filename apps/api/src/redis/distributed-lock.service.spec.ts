import { describe, expect, it, vi } from "vitest";

import { DistributedLockService } from "./distributed-lock.service";
import type { RedisService } from "./redis.service";

describe("DistributedLockService", () => {
  it("releases only with the token used to acquire the lock", async () => {
    const client = {
      set: vi.fn().mockResolvedValue("OK"),
      eval: vi.fn().mockResolvedValue(1),
    };
    const service = new DistributedLockService({
      client,
    } as unknown as RedisService);

    await expect(
      service.withResourceLock("gpu-1", async () => "completed"),
    ).resolves.toBe("completed");

    const acquisitionToken = client.set.mock.calls[0]?.[1];
    expect(client.set).toHaveBeenCalledWith(
      "lock:gpu-resource:gpu-1",
      expect.any(String),
      "EX",
      10,
      "NX",
    );
    expect(client.eval).toHaveBeenCalledWith(
      expect.stringContaining('redis.call("get", KEYS[1])'),
      1,
      "lock:gpu-resource:gpu-1",
      acquisitionToken,
    );
  });

  it("returns a conflict when another request owns the lock", async () => {
    const client = {
      set: vi.fn().mockResolvedValue(null),
      eval: vi.fn(),
    };
    const service = new DistributedLockService({
      client,
    } as unknown as RedisService);

    await expect(
      service.withResourceLock("gpu-1", async () => undefined),
    ).rejects.toMatchObject({ code: "RESOURCE_BUSY" });
    expect(client.eval).not.toHaveBeenCalled();
  });
});
