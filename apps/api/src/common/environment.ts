export interface AppEnvironment {
  API_PORT: number;
  COOKIE_SECURE: boolean;
  MONGO_URI: string;
  REDIS_URL: string;
  SESSION_TTL_SECONDS: number;
  WEB_ORIGIN: string;
}

function readRequired(config: Record<string, unknown>, key: string): string {
  const value = config[key];
  if (typeof value !== "string" || value.trim() === "") {
    throw new Error(`Missing required environment variable: ${key}`);
  }
  return value.trim();
}

function readPositiveInteger(
  config: Record<string, unknown>,
  key: string,
  fallback: number,
): number {
  const raw = config[key];
  const value = raw === undefined ? fallback : Number(raw);
  if (!Number.isSafeInteger(value) || value <= 0) {
    throw new Error(`${key} must be a positive integer`);
  }
  return value;
}

export function validateEnvironment(
  config: Record<string, unknown>,
): Record<string, unknown> & AppEnvironment {
  return {
    ...config,
    API_PORT: readPositiveInteger(config, "API_PORT", 4000),
    COOKIE_SECURE: String(config.COOKIE_SECURE ?? "false") === "true",
    MONGO_URI: readRequired(config, "MONGO_URI"),
    REDIS_URL: readRequired(config, "REDIS_URL"),
    SESSION_TTL_SECONDS: readPositiveInteger(
      config,
      "SESSION_TTL_SECONDS",
      86_400,
    ),
    WEB_ORIGIN: readRequired(config, "WEB_ORIGIN"),
  };
}
