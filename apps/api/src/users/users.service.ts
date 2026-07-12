import { HttpStatus, Injectable, type OnModuleInit } from "@nestjs/common";
import { InjectModel } from "@nestjs/mongoose";
import { UserRole } from "@gpu-rental/contracts";
import * as argon2 from "argon2";
import type { Model } from "mongoose";

import { DomainException } from "../common/domain-exception";
import { isMongoDuplicateKeyError } from "../common/mongo-error";
import { SessionService } from "../redis/session.service";
import type { ChangePasswordDto } from "./users.dto";
import { User, type UserDocument } from "./user.schema";

@Injectable()
export class UsersService implements OnModuleInit {
  constructor(
    @InjectModel(User.name) private readonly users: Model<User>,
    private readonly sessions: SessionService,
  ) {}

  async onModuleInit(): Promise<void> {
    await this.users.init();
  }

  async changePassword(
    userId: string,
    input: ChangePasswordDto,
  ): Promise<void> {
    const user = (await this.users
      .findById(userId)
      .select("+passwordHash")
      .exec()) as UserDocument | null;
    if (!user) {
      throw new DomainException(
        "USER_NOT_FOUND",
        "The user no longer exists",
        HttpStatus.NOT_FOUND,
      );
    }
    if (!(await argon2.verify(user.passwordHash, input.currentPassword))) {
      throw new DomainException(
        "CURRENT_PASSWORD_INVALID",
        "The current password is incorrect",
        HttpStatus.BAD_REQUEST,
      );
    }
    const passwordHash = await argon2.hash(input.newPassword, {
      type: argon2.argon2id,
    });
    await this.sessions.revokeAll(userId);
    user.passwordHash = passwordHash;
    await user.save();
  }

  async createAdmin(usernameInput: string, password: string): Promise<string> {
    const username = usernameInput.trim().toLowerCase();
    try {
      const user = (await this.users.create({
        username,
        passwordHash: await argon2.hash(password, { type: argon2.argon2id }),
        role: UserRole.Admin,
      })) as UserDocument;
      return user._id.toString();
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
}
