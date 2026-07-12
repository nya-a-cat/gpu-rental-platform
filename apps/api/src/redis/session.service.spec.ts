import type { ConfigService } from "@nestjs/config";
import { UserRole } from "@gpu-rental/contracts";
import { describe, expect, it, vi } from "vitest";

import type { RedisService } from "./redis.service";
import { SessionService } from "./session.service";

describe("SessionService", () => {
  it("stores only a token digest and applies the configured TTL", async () => {
    const pipeline = {
      set: vi.fn(),
      sadd: vi.fn(),
      expire: vi.fn(),
      exec: vi.fn().mockResolvedValue([]),
    };
    const client = { multi: vi.fn().mockReturnValue(pipeline) };
    const config = { get: vi.fn().mockReturnValue(120) };
    const service = new SessionService(
      { client } as unknown as RedisService,
      config as unknown as ConfigService,
    );

    const token = await service.create({
      userId: "user-1",
      username: "operator",
      role: UserRole.User,
    });

    const sessionKey = pipeline.set.mock.calls[0]?.[0] as string;
    expect(sessionKey).toMatch(/^session:[a-f0-9]{64}$/);
    expect(sessionKey).not.toContain(token);
    expect(pipeline.set).toHaveBeenCalledWith(
      sessionKey,
      expect.not.stringContaining(token),
      "EX",
      120,
    );
    expect(pipeline.expire).toHaveBeenCalledWith("user-sessions:user-1", 120);
  });
});
