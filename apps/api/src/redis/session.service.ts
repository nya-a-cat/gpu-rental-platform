import { createHash, randomBytes } from "node:crypto";

import { ConfigService } from "@nestjs/config";
import { Inject, Injectable } from "@nestjs/common";
import { UserRole } from "@gpu-rental/contracts";

import { RedisService } from "./redis.service";

export interface SessionIdentity {
  userId: string;
  username: string;
  role: UserRole;
}

@Injectable()
export class SessionService {
  private readonly ttlSeconds: number;

  constructor(
    private readonly redis: RedisService,
    @Inject(ConfigService) config: ConfigService,
  ) {
    this.ttlSeconds = config.get<number>("SESSION_TTL_SECONDS", 86_400);
  }

  async create(identity: SessionIdentity): Promise<string> {
    const token = randomBytes(32).toString("base64url");
    const tokenHash = this.hash(token);
    const sessionKey = this.sessionKey(tokenHash);
    const registryKey = this.registryKey(identity.userId);
    const pipeline = this.redis.client.multi();
    pipeline.set(sessionKey, JSON.stringify(identity), "EX", this.ttlSeconds);
    pipeline.sadd(registryKey, tokenHash);
    pipeline.expire(registryKey, this.ttlSeconds);
    await pipeline.exec();
    return token;
  }

  async resolve(token: string | undefined): Promise<SessionIdentity | null> {
    if (!token) {
      return null;
    }
    const value = await this.redis.client.get(
      this.sessionKey(this.hash(token)),
    );
    if (!value) {
      return null;
    }
    try {
      const identity = JSON.parse(value) as Partial<SessionIdentity>;
      if (
        typeof identity.userId !== "string" ||
        typeof identity.username !== "string" ||
        !Object.values(UserRole).includes(identity.role as UserRole)
      ) {
        return null;
      }
      return identity as SessionIdentity;
    } catch {
      return null;
    }
  }

  async revoke(token: string | undefined): Promise<void> {
    if (!token) {
      return;
    }
    const tokenHash = this.hash(token);
    const sessionKey = this.sessionKey(tokenHash);
    const value = await this.redis.client.get(sessionKey);
    const identity = value ? this.parseIdentity(value) : null;
    const pipeline = this.redis.client.multi();
    pipeline.del(sessionKey);
    if (identity) {
      pipeline.srem(this.registryKey(identity.userId), tokenHash);
    }
    await pipeline.exec();
  }

  async revokeAll(userId: string): Promise<void> {
    const registryKey = this.registryKey(userId);
    const tokenHashes = await this.redis.client.smembers(registryKey);
    const pipeline = this.redis.client.multi();
    for (const tokenHash of tokenHashes) {
      pipeline.del(this.sessionKey(tokenHash));
    }
    pipeline.del(registryKey);
    await pipeline.exec();
  }

  private hash(token: string): string {
    return createHash("sha256").update(token).digest("hex");
  }

  private parseIdentity(value: string): SessionIdentity | null {
    try {
      const identity = JSON.parse(value) as Partial<SessionIdentity>;
      return typeof identity.userId === "string"
        ? (identity as SessionIdentity)
        : null;
    } catch {
      return null;
    }
  }

  private sessionKey(tokenHash: string): string {
    return `session:${tokenHash}`;
  }

  private registryKey(userId: string): string {
    return `user-sessions:${userId}`;
  }
}
