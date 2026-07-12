import { Global, Module } from "@nestjs/common";
import { ConfigService } from "@nestjs/config";
import Redis from "ioredis";

import { DistributedLockService } from "./distributed-lock.service";
import { REDIS_CLIENT } from "./redis.constants";
import { RedisService } from "./redis.service";
import { SessionService } from "./session.service";

@Global()
@Module({
  providers: [
    {
      provide: REDIS_CLIENT,
      inject: [ConfigService],
      useFactory: (config: ConfigService): Redis =>
        new Redis(config.getOrThrow<string>("REDIS_URL"), {
          enableReadyCheck: true,
          maxRetriesPerRequest: 2,
        }),
    },
    RedisService,
    DistributedLockService,
    SessionService,
  ],
  exports: [RedisService, DistributedLockService, SessionService],
})
export class RedisModule {}
