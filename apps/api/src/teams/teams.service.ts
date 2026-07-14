import { randomUUID } from "node:crypto";

import { HttpStatus, Injectable, type OnModuleInit } from "@nestjs/common";
import { InjectModel } from "@nestjs/mongoose";
import {
  NotificationType,
  TeamRole,
  type TeamView,
} from "@gpu-rental/contracts";
import { Types, type Model } from "mongoose";

import { CloudAccountsService } from "../cloud-accounts/cloud-accounts.service";
import { DomainException } from "../common/domain-exception";
import { isMongoDuplicateKeyError } from "../common/mongo-error";
import { User, type UserDocument } from "../users/user.schema";
import {
  type AddTeamMemberDto,
  type CreateProjectDto,
  type CreateTeamDto,
} from "./teams.dto";
import { type ProjectRecord, Team, type TeamDocument } from "./team.schema";

export interface ProjectAttribution {
  projectId: string;
  projectName: string;
  teamName: string;
}

@Injectable()
export class TeamsService implements OnModuleInit {
  constructor(
    @InjectModel(Team.name) private readonly teams: Model<Team>,
    @InjectModel(User.name) private readonly users: Model<User>,
    private readonly accounts: CloudAccountsService,
  ) {}

  async onModuleInit(): Promise<void> {
    await this.teams.init();
  }

  async listMine(userId: string): Promise<TeamView[]> {
    const teams = (await this.teams
      .find({ "members.userId": new Types.ObjectId(userId) })
      .sort({ createdAt: -1 })
      .exec()) as TeamDocument[];
    return teams.map((team) => this.toView(team, userId));
  }

  async create(
    userId: string,
    username: string,
    input: CreateTeamDto,
  ): Promise<TeamView> {
    const now = new Date();
    try {
      const team = (await this.teams.create({
        name: input.name.trim(),
        ownerId: new Types.ObjectId(userId),
        members: [
          {
            userId: new Types.ObjectId(userId),
            username,
            role: TeamRole.Owner,
            joinedAt: now,
          },
        ],
        projects: [],
      })) as TeamDocument;
      await this.accounts.addNotification(
        userId,
        NotificationType.Team,
        "Team created",
        `${team.name} is ready for members and projects.`,
      );
      return this.toView(team, userId);
    } catch (error) {
      if (isMongoDuplicateKeyError(error)) {
        throw new DomainException(
          "TEAM_NAME_TAKEN",
          "You already own a team with this name",
          HttpStatus.CONFLICT,
        );
      }
      throw error;
    }
  }

  async addMember(
    teamId: string,
    actorUserId: string,
    input: AddTeamMemberDto,
  ): Promise<TeamView> {
    const team = await this.findManageable(teamId, actorUserId);
    const user = (await this.users
      .findOne({ username: input.username.trim().toLowerCase() })
      .exec()) as UserDocument | null;
    if (!user) {
      throw new DomainException(
        "TEAM_MEMBER_USER_NOT_FOUND",
        "The requested username was not found",
        HttpStatus.NOT_FOUND,
      );
    }
    if (
      team.members.some(
        (member) => member.userId.toString() === user._id.toString(),
      )
    ) {
      throw new DomainException(
        "TEAM_MEMBER_EXISTS",
        "The user is already a team member",
        HttpStatus.CONFLICT,
      );
    }
    team.members.push({
      userId: user._id,
      username: user.username,
      role: input.role,
      joinedAt: new Date(),
    });
    await team.save();
    await this.accounts.addNotification(
      user._id.toString(),
      NotificationType.Team,
      "Added to team",
      `You joined ${team.name} as ${input.role}.`,
    );
    return this.toView(team, actorUserId);
  }

  async createProject(
    teamId: string,
    actorUserId: string,
    input: CreateProjectDto,
  ): Promise<TeamView> {
    const team = await this.findManageable(teamId, actorUserId);
    if (
      team.projects.some(
        (project) =>
          project.name.toLowerCase() === input.name.trim().toLowerCase(),
      )
    ) {
      throw new DomainException(
        "PROJECT_NAME_TAKEN",
        "The team already has a project with this name",
        HttpStatus.CONFLICT,
      );
    }
    const project: ProjectRecord = {
      id: randomUUID(),
      name: input.name.trim(),
      monthlyBudgetCents: input.monthlyBudgetCents,
      bookedCostCents: 0,
      createdAt: new Date(),
    };
    team.projects.push(project);
    await team.save();
    return this.toView(team, actorUserId);
  }

  async resolveProjectForUser(
    userId: string,
    projectId?: string,
  ): Promise<ProjectAttribution | null> {
    if (!projectId) return null;
    const team = (await this.teams
      .findOne({
        "members.userId": new Types.ObjectId(userId),
        "projects.id": projectId,
      })
      .exec()) as TeamDocument | null;
    const project = team?.projects.find(
      (candidate) => candidate.id === projectId,
    );
    if (!team || !project) {
      throw new DomainException(
        "PROJECT_NOT_FOUND",
        "The project was not found for this user",
        HttpStatus.NOT_FOUND,
      );
    }
    return {
      projectId: project.id,
      projectName: project.name,
      teamName: team.name,
    };
  }

  async recordBooking(
    projectId: string | null,
    amountCents: number,
  ): Promise<void> {
    if (!projectId) return;
    await this.teams
      .updateOne(
        { "projects.id": projectId },
        { $inc: { "projects.$.bookedCostCents": amountCents } },
      )
      .exec();
  }

  private async findManageable(
    teamId: string,
    userId: string,
  ): Promise<TeamDocument> {
    if (!Types.ObjectId.isValid(teamId)) this.throwNotFound();
    const team = (await this.teams
      .findOne({
        _id: new Types.ObjectId(teamId),
        members: {
          $elemMatch: {
            userId: new Types.ObjectId(userId),
            role: { $in: [TeamRole.Owner, TeamRole.Admin] },
          },
        },
      })
      .exec()) as TeamDocument | null;
    if (!team) this.throwNotFound();
    return team;
  }

  private toView(team: TeamDocument, userId: string): TeamView {
    const currentMember = team.members.find(
      (member) => member.userId.toString() === userId,
    );
    if (!currentMember) this.throwNotFound();
    return {
      id: team._id.toString(),
      name: team.name,
      currentUserRole: currentMember.role,
      members: team.members.map((member) => ({
        userId: member.userId.toString(),
        username: member.username,
        role: member.role,
        joinedAt: member.joinedAt.toISOString(),
      })),
      projects: team.projects.map((project) => ({
        id: project.id,
        name: project.name,
        monthlyBudgetCents: project.monthlyBudgetCents,
        bookedCostCents: project.bookedCostCents,
        createdAt: project.createdAt.toISOString(),
      })),
      createdAt: team.createdAt.toISOString(),
      updatedAt: team.updatedAt.toISOString(),
    };
  }

  private throwNotFound(): never {
    throw new DomainException(
      "TEAM_NOT_FOUND",
      "The team was not found or cannot be managed by this user",
      HttpStatus.NOT_FOUND,
    );
  }
}
