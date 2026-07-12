import { UserRole } from "@gpu-rental/contracts";
import { Types, type Model } from "mongoose";
import { describe, expect, it, vi } from "vitest";

import type { SessionService } from "../redis/session.service";
import { User } from "../users/user.schema";
import { AuthService } from "./auth.service";

vi.mock("argon2", () => ({
  argon2id: 2,
  hash: vi.fn().mockResolvedValue("argon-hash"),
  verify: vi.fn().mockResolvedValue(true),
}));

describe("AuthService", () => {
  it("always assigns the user role during public registration", async () => {
    const now = new Date("2026-01-01T00:00:00.000Z");
    const create = vi
      .fn()
      .mockImplementation((input: Record<string, unknown>) => ({
        ...input,
        _id: new Types.ObjectId(),
        createdAt: now,
        updatedAt: now,
      }));
    const sessions = {
      create: vi.fn().mockResolvedValue("session-token"),
    };
    const service = new AuthService(
      { create } as unknown as Model<User>,
      sessions as unknown as SessionService,
    );

    const result = await service.register({
      username: "Portfolio_User",
      password: "secure-password",
    });

    expect(create).toHaveBeenCalledWith(
      expect.objectContaining({
        username: "portfolio_user",
        role: UserRole.User,
      }),
    );
    expect(result.user.role).toBe(UserRole.User);
    expect(result.sessionToken).toBe("session-token");
  });
});
