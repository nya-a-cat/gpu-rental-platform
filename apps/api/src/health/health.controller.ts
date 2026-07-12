import { Controller, Get } from "@nestjs/common";
import { ApiTags } from "@nestjs/swagger";
import type { HealthResponse } from "@gpu-rental/contracts";

import { HealthService } from "./health.service";

@ApiTags("health")
@Controller("health")
export class HealthController {
  constructor(private readonly health: HealthService) {}

  @Get("live")
  live(): HealthResponse {
    return this.health.live();
  }

  @Get("ready")
  ready(): Promise<HealthResponse> {
    return this.health.ready();
  }
}
