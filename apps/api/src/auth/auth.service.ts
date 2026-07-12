import { HttpStatus, Injectable } from "@nestjs/common";
import { InjectModel } from "@nestjs/mongoose";
import type { AuthResponse } from "@gpu-rental/contracts";
import { UserRole } from "@gpu-rental/contracts";
import * as argon2 from "argon2";
import type { Model } from "mongoose";

import { DomainException } from "../common/domain-exception";
import { isMongoDuplicateKeyError } from "../common/mongo-error";
import { SessionService } from "../redis/session.service";
import { User, type UserDocument } from "../users/user.schema";
import type { LoginDto, RegisterDto } from "./auth.dto";
import { toUserView } from "./user-view";

export interface AuthenticatedResult extends AuthResponse {
  sessionToken: string;
}

@Injectable()
export class AuthService {
  constructor(
    @InjectModel(User.name) private readonly users: Model<User>,
    private readonly sessions: SessionService,
  ) {}

  async register(input: RegisterDto): Promise<AuthenticatedResult> {
    const username = input.username.trim().toLowerCase();
    try {
      const user = (await this.users.create({
        username,
        passwordHash: await argon2.hash(input.password, {
          type: argon2.argon2id,
        }),
        role: UserRole.User,
      })) as UserDocument;
      return this.createAuthenticatedResult(user);
    } catch (error) {
      if (isMongoDuplicateKeyError(error)) {
        throw new DomainException(
          "USERNAME_TAKEN",
          "The username is already in use",
          HttpStatus.CONFLICT,
        );
      }
      throw error;
    }
  }

  async login(input: LoginDto): Promise<AuthenticatedResult> {
    const user = (await this.users
      .findOne({ username: input.username.trim().toLowerCase() })
      .select("+passwordHash")
      .exec()) as UserDocument | null;
    const valid = user
      ? await argon2.verify(user.passwordHash, input.password)
      : false;
    if (!user || !valid) {
      throw new DomainException(
        "INVALID_CREDENTIALS",
        "The username or password is incorrect",
        HttpStatus.UNAUTHORIZED,
      );
    }
    return this.createAuthenticatedResult(user);
  }

  async getCurrentUser(userId: string): Promise<AuthResponse> {
    const user = (await this.users
      .findById(userId)
      .exec()) as UserDocument | null;
    if (!user) {
      throw new DomainException(
        "USER_NOT_FOUND",
        "The user no longer exists",
        HttpStatus.UNAUTHORIZED,
      );
    }
    return { user: toUserView(user) };
  }

  private async createAuthenticatedResult(
    user: UserDocument,
  ): Promise<AuthenticatedResult> {
    const userView = toUserView(user);
    const sessionToken = await this.sessions.create({
      userId: userView.id,
      username: userView.username,
      role: userView.role,
    });
    return { user: userView, sessionToken };
  }
}
