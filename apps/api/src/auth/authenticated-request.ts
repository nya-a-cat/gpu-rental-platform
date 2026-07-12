import type { Request } from "express";

import type { SessionIdentity } from "../redis/session.service";

export type AuthenticatedRequest = Request & {
  auth: SessionIdentity;
};
