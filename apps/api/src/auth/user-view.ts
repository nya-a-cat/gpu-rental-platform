import type { UserView } from "@gpu-rental/contracts";

import type { UserDocument } from "../users/user.schema";

export function toUserView(user: UserDocument): UserView {
  return {
    id: user._id.toString(),
    username: user.username,
    role: user.role,
    createdAt: user.createdAt.toISOString(),
    updatedAt: user.updatedAt.toISOString(),
  };
}
