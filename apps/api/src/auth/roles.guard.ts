import {
  CanActivate,
  ExecutionContext,
  HttpStatus,
  Injectable,
} from "@nestjs/common";
import { Reflector } from "@nestjs/core";
import type { UserRole } from "@gpu-rental/contracts";

import { DomainException } from "../common/domain-exception";
import type { AuthenticatedRequest } from "./authenticated-request";
import { ROLES_KEY } from "./roles.decorator";

@Injectable()
export class RolesGuard implements CanActivate {
  constructor(private readonly reflector: Reflector) {}

  canActivate(context: ExecutionContext): boolean {
    const roles = this.reflector.getAllAndOverride<UserRole[]>(ROLES_KEY, [
      context.getHandler(),
      context.getClass(),
    ]);
    if (!roles?.length) {
      return true;
    }
    const request = context.switchToHttp().getRequest<AuthenticatedRequest>();
    if (!roles.includes(request.auth.role)) {
      throw new DomainException(
        "ROLE_FORBIDDEN",
        "You do not have permission to access this resource",
        HttpStatus.FORBIDDEN,
      );
    }
    return true;
  }
}
