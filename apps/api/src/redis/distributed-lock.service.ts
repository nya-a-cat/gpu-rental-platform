import { randomBytes } from "node:crypto";

import { HttpStatus, Injectable, Logger } from "@nestjs/common";

import { DomainException } from "../common/domain-exception";
import { RedisService } from "./redis.service";

const RELEASE_SCRIPT = `
if redis.call("get", KEYS[1]) == ARGV[1] then
  return redis.call("del", KEYS[1])
end
return 0
`;

@Injectable()
export class DistributedLockService {
  private readonly logger = new Logger(DistributedLockService.name);

  constructor(private readonly redis: RedisService) {}

  async withResourceLock<T>(
    resourceId: string,
    action: () => Promise<T>,
    ttlSeconds = 10,
  ): Promise<T> {
    const key = `lock:gpu-resource:${resourceId}`;
    const token = randomBytes(24).toString("base64url");
    const acquired = await this.redis.client.set(
      key,
      token,
      "EX",
      ttlSeconds,
      "NX",
    );

    if (acquired !== "OK") {
      throw new DomainException(
        "RESOURCE_BUSY",
        "The GPU resource is currently being updated",
        HttpStatus.CONFLICT,
      );
    }

    try {
      return await action();
    } finally {
      // 仅锁持有者可释放，避免过期请求误删后继请求创建的新锁。
      await this.redis.client
        .eval(RELEASE_SCRIPT, 1, key, token)
        .catch((error: unknown) => {
          this.logger.warn(
            `Resource lock release failed for ${resourceId}: ${error instanceof Error ? error.message : String(error)}`,
          );
        });
    }
  }
}
