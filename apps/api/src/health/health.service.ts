import { HttpStatus, Injectable } from "@nestjs/common";
import { InjectConnection } from "@nestjs/mongoose";
import type { HealthResponse } from "@gpu-rental/contracts";
import type { Connection } from "mongoose";

import { DomainException } from "../common/domain-exception";
import { RedisService } from "../redis/redis.service";

@Injectable()
export class HealthService {
  constructor(
    @InjectConnection() private readonly mongo: Connection,
    private readonly redis: RedisService,
  ) {}

  live(): HealthResponse {
    return { status: "ok" };
  }

  async ready(): Promise<HealthResponse> {
    try {
      if (this.mongo.readyState !== 1 || !this.mongo.db) {
        throw new Error("MongoDB is not connected");
      }
      await Promise.all([this.mongo.db.admin().ping(), this.redis.ping()]);
      return { status: "ok" };
    } catch {
      throw new DomainException(
        "DEPENDENCY_UNAVAILABLE",
        "MongoDB or Redis is not ready",
        HttpStatus.SERVICE_UNAVAILABLE,
      );
    }
  }
}
