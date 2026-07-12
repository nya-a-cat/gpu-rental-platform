import {
  Body,
  Controller,
  Get,
  HttpCode,
  HttpStatus,
  Post,
  Req,
  Res,
  UseGuards,
} from "@nestjs/common";
import { ConfigService } from "@nestjs/config";
import { ApiCookieAuth, ApiOperation, ApiTags } from "@nestjs/swagger";
import type { AuthResponse } from "@gpu-rental/contracts";
import type { Request, Response } from "express";

import { SessionService, type SessionIdentity } from "../redis/session.service";
import type { AuthenticatedRequest } from "./authenticated-request";
import { AuthService } from "./auth.service";
import { LoginDto, RegisterDto } from "./auth.dto";
import { CurrentUser } from "./current-user.decorator";
import { SESSION_COOKIE_NAME, sessionCookieOptions } from "./session-cookie";
import { SessionAuthGuard } from "./session-auth.guard";

@ApiTags("auth")
@Controller("auth")
export class AuthController {
  constructor(
    private readonly auth: AuthService,
    private readonly sessions: SessionService,
    private readonly config: ConfigService,
  ) {}

  @Post("register")
  @ApiOperation({ summary: "Register a regular user" })
  async register(
    @Body() input: RegisterDto,
    @Res({ passthrough: true }) response: Response,
  ): Promise<AuthResponse> {
    const { sessionToken, user } = await this.auth.register(input);
    response.cookie(
      SESSION_COOKIE_NAME,
      sessionToken,
      sessionCookieOptions(this.config),
    );
    return { user };
  }

  @Post("login")
  @HttpCode(HttpStatus.OK)
  @ApiOperation({ summary: "Create a server-side session" })
  async login(
    @Body() input: LoginDto,
    @Res({ passthrough: true }) response: Response,
  ): Promise<AuthResponse> {
    const { sessionToken, user } = await this.auth.login(input);
    response.cookie(
      SESSION_COOKIE_NAME,
      sessionToken,
      sessionCookieOptions(this.config),
    );
    return { user };
  }

  @Get("me")
  @UseGuards(SessionAuthGuard)
  @ApiCookieAuth(SESSION_COOKIE_NAME)
  async me(@CurrentUser() user: SessionIdentity): Promise<AuthResponse> {
    return this.auth.getCurrentUser(user.userId);
  }

  @Post("logout")
  @HttpCode(HttpStatus.NO_CONTENT)
  async logout(
    @Req() request: Request,
    @Res({ passthrough: true }) response: Response,
  ): Promise<void> {
    await this.sessions.revoke(request.cookies?.sid as string | undefined);
    response.clearCookie(
      SESSION_COOKIE_NAME,
      sessionCookieOptions(this.config, false),
    );
  }

  @Post("logout-all")
  @UseGuards(SessionAuthGuard)
  @HttpCode(HttpStatus.NO_CONTENT)
  @ApiCookieAuth(SESSION_COOKIE_NAME)
  async logoutAll(
    @Req() request: AuthenticatedRequest,
    @Res({ passthrough: true }) response: Response,
  ): Promise<void> {
    await this.sessions.revokeAll(request.auth.userId);
    response.clearCookie(
      SESSION_COOKIE_NAME,
      sessionCookieOptions(this.config, false),
    );
  }
}
