import {
  Body,
  Controller,
  Delete,
  Get,
  HttpCode,
  HttpStatus,
  Param,
  Post,
  UseGuards,
} from "@nestjs/common";
import { ApiCookieAuth, ApiOperation, ApiTags } from "@nestjs/swagger";
import type {
  ApiKeyView,
  CloudAccountView,
  NetworkRuleView,
  NotificationView,
  SshKeyView,
  VolumeView,
} from "@gpu-rental/contracts";

import { CurrentUser } from "../auth/current-user.decorator";
import { SESSION_COOKIE_NAME } from "../auth/session-cookie";
import { SessionAuthGuard } from "../auth/session-auth.guard";
import type { SessionIdentity } from "../redis/session.service";
import {
  AttachVolumeDto,
  CreateApiKeyDto,
  CreateNetworkRuleDto,
  CreateSnapshotDto,
  CreateSshKeyDto,
  CreateVolumeDto,
  TopUpDto,
} from "./cloud-accounts.dto";
import { CloudAccountsService } from "./cloud-accounts.service";

@ApiTags("cloud-account")
@ApiCookieAuth(SESSION_COOKIE_NAME)
@UseGuards(SessionAuthGuard)
@Controller("cloud-account")
export class CloudAccountsController {
  constructor(private readonly accounts: CloudAccountsService) {}

  @Get()
  @ApiOperation({ summary: "Get wallet and cloud operations workspace" })
  getAccount(@CurrentUser() user: SessionIdentity): Promise<CloudAccountView> {
    return this.accounts.getAccount(user.userId);
  }

  @Post("top-ups")
  topUp(
    @CurrentUser() user: SessionIdentity,
    @Body() input: TopUpDto,
  ): Promise<CloudAccountView> {
    return this.accounts.topUp(user.userId, input);
  }

  @Post("ssh-keys")
  createSshKey(
    @CurrentUser() user: SessionIdentity,
    @Body() input: CreateSshKeyDto,
  ): Promise<SshKeyView> {
    return this.accounts.createSshKey(user.userId, input);
  }

  @Delete("ssh-keys/:id")
  @HttpCode(HttpStatus.NO_CONTENT)
  deleteSshKey(
    @CurrentUser() user: SessionIdentity,
    @Param("id") id: string,
  ): Promise<void> {
    return this.accounts.deleteSshKey(user.userId, id);
  }

  @Post("api-keys")
  createApiKey(
    @CurrentUser() user: SessionIdentity,
    @Body() input: CreateApiKeyDto,
  ): Promise<ApiKeyView> {
    return this.accounts.createApiKey(user.userId, input);
  }

  @Delete("api-keys/:id")
  @HttpCode(HttpStatus.NO_CONTENT)
  deleteApiKey(
    @CurrentUser() user: SessionIdentity,
    @Param("id") id: string,
  ): Promise<void> {
    return this.accounts.deleteApiKey(user.userId, id);
  }

  @Post("network-rules")
  createNetworkRule(
    @CurrentUser() user: SessionIdentity,
    @Body() input: CreateNetworkRuleDto,
  ): Promise<NetworkRuleView> {
    return this.accounts.createNetworkRule(user.userId, input);
  }

  @Delete("network-rules/:id")
  @HttpCode(HttpStatus.NO_CONTENT)
  deleteNetworkRule(
    @CurrentUser() user: SessionIdentity,
    @Param("id") id: string,
  ): Promise<void> {
    return this.accounts.deleteNetworkRule(user.userId, id);
  }

  @Post("volumes")
  createVolume(
    @CurrentUser() user: SessionIdentity,
    @Body() input: CreateVolumeDto,
  ): Promise<VolumeView> {
    return this.accounts.createVolume(user.userId, input);
  }

  @Post("volumes/:id/attach")
  attachVolume(
    @CurrentUser() user: SessionIdentity,
    @Param("id") id: string,
    @Body() input: AttachVolumeDto,
  ): Promise<VolumeView> {
    return this.accounts.attachVolume(user.userId, id, input);
  }

  @Post("volumes/:id/detach")
  detachVolume(
    @CurrentUser() user: SessionIdentity,
    @Param("id") id: string,
  ): Promise<VolumeView> {
    return this.accounts.detachVolume(user.userId, id);
  }

  @Post("volumes/:id/snapshots")
  createSnapshot(
    @CurrentUser() user: SessionIdentity,
    @Param("id") id: string,
    @Body() input: CreateSnapshotDto,
  ): Promise<VolumeView> {
    return this.accounts.createSnapshot(user.userId, id, input);
  }

  @Delete("volumes/:id")
  deleteVolume(
    @CurrentUser() user: SessionIdentity,
    @Param("id") id: string,
  ): Promise<VolumeView> {
    return this.accounts.deleteVolume(user.userId, id);
  }

  @Post("notifications/read-all")
  @HttpCode(HttpStatus.NO_CONTENT)
  markAllNotificationsRead(
    @CurrentUser() user: SessionIdentity,
  ): Promise<void> {
    return this.accounts.markAllNotificationsRead(user.userId);
  }

  @Post("notifications/:id/read")
  markNotificationRead(
    @CurrentUser() user: SessionIdentity,
    @Param("id") id: string,
  ): Promise<NotificationView> {
    return this.accounts.markNotificationRead(user.userId, id);
  }
}
