import {
  Controller,
  Get,
  HttpCode,
  HttpStatus,
  Param,
  Post,
  Query,
  UseGuards,
} from "@nestjs/common";
import { ApiCookieAuth, ApiOperation, ApiTags } from "@nestjs/swagger";
import type { InstanceView, PaginatedResponse } from "@gpu-rental/contracts";

import { CurrentUser } from "../auth/current-user.decorator";
import { SESSION_COOKIE_NAME } from "../auth/session-cookie";
import { SessionAuthGuard } from "../auth/session-auth.guard";
import type { SessionIdentity } from "../redis/session.service";
import { InstanceQueryDto } from "./instances.dto";
import { InstancesService } from "./instances.service";

@ApiTags("instances")
@ApiCookieAuth(SESSION_COOKIE_NAME)
@UseGuards(SessionAuthGuard)
@Controller("instances")
export class InstancesController {
  constructor(private readonly instances: InstancesService) {}

  @Get("me")
  listMine(
    @CurrentUser() user: SessionIdentity,
    @Query() query: InstanceQueryDto,
  ): Promise<PaginatedResponse<InstanceView>> {
    return this.instances.listMine(user.userId, query);
  }

  @Get(":id")
  getMine(
    @CurrentUser() user: SessionIdentity,
    @Param("id") id: string,
  ): Promise<InstanceView> {
    return this.instances.getMine(id, user.userId);
  }

  @Post(":id/start")
  @HttpCode(HttpStatus.OK)
  @ApiOperation({ summary: "Start a stopped simulated instance" })
  start(
    @CurrentUser() user: SessionIdentity,
    @Param("id") id: string,
  ): Promise<InstanceView> {
    return this.instances.start(id, user.userId);
  }

  @Post(":id/stop")
  @HttpCode(HttpStatus.OK)
  @ApiOperation({ summary: "Stop a running simulated instance" })
  stop(
    @CurrentUser() user: SessionIdentity,
    @Param("id") id: string,
  ): Promise<InstanceView> {
    return this.instances.stop(id, user.userId);
  }

  @Post(":id/terminate")
  @HttpCode(HttpStatus.OK)
  @ApiOperation({
    summary: "Terminate a simulated instance and return its order",
  })
  terminate(
    @CurrentUser() user: SessionIdentity,
    @Param("id") id: string,
  ): Promise<InstanceView> {
    return this.instances.terminate(id, user.userId);
  }
}
