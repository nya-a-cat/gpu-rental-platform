import type { ConfigService } from "@nestjs/config";
import type { CookieOptions } from "express";

export const SESSION_COOKIE_NAME = "sid";

export function sessionCookieOptions(
  config: ConfigService,
  includeMaxAge = true,
): CookieOptions {
  const options: CookieOptions = {
    httpOnly: true,
    path: "/",
    sameSite: "lax",
    secure: config.get<boolean>("COOKIE_SECURE", false),
  };
  if (includeMaxAge) {
    options.maxAge = config.get<number>("SESSION_TTL_SECONDS", 86_400) * 1_000;
  }
  return options;
}
