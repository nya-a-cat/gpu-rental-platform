import {
  Body,
  Controller,
  HttpCode,
  HttpStatus,
  Patch,
  Res,
  UseGuards,
} from "@nestjs/common";
import { ConfigService } from "@nestjs/config";
import { ApiCookieAuth, ApiOperation, ApiTags } from "@nestjs/swagger";
import type { Response } from "express";

import { CurrentUser } from "../auth/current-user.decorator";
import {
  SESSION_COOKIE_NAME,
  sessionCookieOptions,
} from "../auth/session-cookie";
import { SessionAuthGuard } from "../auth/session-auth.guard";
import type { SessionIdentity } from "../redis/session.service";
import { ChangePasswordDto } from "./users.dto";
import { UsersService } from "./users.service";

@ApiTags("users")
@ApiCookieAuth(SESSION_COOKIE_NAME)
@UseGuards(SessionAuthGuard)
@Controller("users")
export class UsersController {
  constructor(
    private readonly users: UsersService,
    private readonly config: ConfigService,
  ) {}

  @Patch("me/password")
  @HttpCode(HttpStatus.NO_CONTENT)
  @ApiOperation({ summary: "Change password and revoke every active session" })
  async changePassword(
    @CurrentUser() user: SessionIdentity,
    @Body() input: ChangePasswordDto,
    @Res({ passthrough: true }) response: Response,
  ): Promise<void> {
    await this.users.changePassword(user.userId, input);
    response.clearCookie(
      SESSION_COOKIE_NAME,
      sessionCookieOptions(this.config, false),
    );
  }
}
