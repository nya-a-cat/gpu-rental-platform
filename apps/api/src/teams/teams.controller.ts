import { Body, Controller, Get, Param, Post, UseGuards } from "@nestjs/common";
import { ApiCookieAuth, ApiTags } from "@nestjs/swagger";
import type { TeamView } from "@gpu-rental/contracts";

import { CurrentUser } from "../auth/current-user.decorator";
import { SESSION_COOKIE_NAME } from "../auth/session-cookie";
import { SessionAuthGuard } from "../auth/session-auth.guard";
import type { SessionIdentity } from "../redis/session.service";
import { AddTeamMemberDto, CreateProjectDto, CreateTeamDto } from "./teams.dto";
import { TeamsService } from "./teams.service";

@ApiTags("teams")
@ApiCookieAuth(SESSION_COOKIE_NAME)
@UseGuards(SessionAuthGuard)
@Controller("teams")
export class TeamsController {
  constructor(private readonly teams: TeamsService) {}

  @Get("me")
  listMine(@CurrentUser() user: SessionIdentity): Promise<TeamView[]> {
    return this.teams.listMine(user.userId);
  }

  @Post()
  create(
    @CurrentUser() user: SessionIdentity,
    @Body() input: CreateTeamDto,
  ): Promise<TeamView> {
    return this.teams.create(user.userId, user.username, input);
  }

  @Post(":id/members")
  addMember(
    @CurrentUser() user: SessionIdentity,
    @Param("id") id: string,
    @Body() input: AddTeamMemberDto,
  ): Promise<TeamView> {
    return this.teams.addMember(id, user.userId, input);
  }

  @Post(":id/projects")
  createProject(
    @CurrentUser() user: SessionIdentity,
    @Param("id") id: string,
    @Body() input: CreateProjectDto,
  ): Promise<TeamView> {
    return this.teams.createProject(id, user.userId, input);
  }
}
