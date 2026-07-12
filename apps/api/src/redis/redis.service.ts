import { Inject, Injectable, type OnModuleDestroy } from "@nestjs/common";
import type Redis from "ioredis";

import { REDIS_CLIENT } from "./redis.constants";

@Injectable()
export class RedisService implements OnModuleDestroy {
  constructor(@Inject(REDIS_CLIENT) readonly client: Redis) {}

  async ping(): Promise<string> {
    return this.client.ping();
  }

  async onModuleDestroy(): Promise<void> {
    if (this.client.status !== "end") {
      await this.client.quit().catch(() => this.client.disconnect());
    }
  }
}
