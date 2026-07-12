import {
  CanActivate,
  ExecutionContext,
  HttpStatus,
  Injectable,
} from "@nestjs/common";

import { DomainException } from "../common/domain-exception";
import { SessionService } from "../redis/session.service";
import type { AuthenticatedRequest } from "./authenticated-request";

@Injectable()
export class SessionAuthGuard implements CanActivate {
  constructor(private readonly sessions: SessionService) {}

  async canActivate(context: ExecutionContext): Promise<boolean> {
    const request = context.switchToHttp().getRequest<AuthenticatedRequest>();
    const identity = await this.sessions.resolve(
      request.cookies?.sid as string | undefined,
    );
    if (!identity) {
      throw new DomainException(
        "SESSION_INVALID",
        "Authentication is required",
        HttpStatus.UNAUTHORIZED,
      );
    }
    request.auth = identity;
    return true;
  }
}
