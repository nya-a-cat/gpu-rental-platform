import "reflect-metadata";

import { emitKeypressEvents } from "node:readline";

import { NestFactory } from "@nestjs/core";

import { AppModule } from "../app.module";
import { GpuResourcesService } from "../gpu-resources/gpu-resources.service";
import { UsersService } from "../users/users.service";

function readOption(name: string): string | undefined {
  const index = process.argv.indexOf(`--${name}`);
  return index >= 0 ? process.argv[index + 1] : undefined;
}

async function readHiddenPassword(prompt: string): Promise<string> {
  const fromEnvironment = process.env.ADMIN_PASSWORD;
  if (fromEnvironment) {
    return fromEnvironment;
  }
  if (!process.stdin.isTTY || !process.stdin.setRawMode) {
    throw new Error("ADMIN_PASSWORD is required when the CLI has no TTY");
  }

  process.stdout.write(prompt);
  emitKeypressEvents(process.stdin);
  process.stdin.setRawMode(true);
  process.stdin.resume();
  return new Promise((resolve, reject) => {
    let value = "";
    const cleanup = (): void => {
      process.stdin.off("keypress", onKeypress);
      process.stdin.setRawMode(false);
      process.stdin.pause();
      process.stdout.write("\n");
    };
    const onKeypress = (
      character: string | undefined,
      key: { name?: string; ctrl?: boolean },
    ): void => {
      if (key.ctrl && key.name === "c") {
        cleanup();
        reject(new Error("Command cancelled"));
        return;
      }
      if (key.name === "return" || key.name === "enter") {
        cleanup();
        resolve(value);
        return;
      }
      if (key.name === "backspace") {
        value = value.slice(0, -1);
        return;
      }
      if (character && !key.ctrl) {
        value += character;
      }
    };
    process.stdin.on("keypress", onKeypress);
  });
}

async function run(): Promise<void> {
  const command = process.argv[2];
  if (!command || !["demo:seed", "admin:create"].includes(command)) {
    throw new Error(
      "Usage: pnpm cli demo:seed | pnpm cli admin:create --username <name>",
    );
  }

  const app = await NestFactory.createApplicationContext(AppModule, {
    logger: ["error", "warn"],
  });
  try {
    if (command === "demo:seed") {
      const created = await app.get(GpuResourcesService).seedDemoResources();
      process.stdout.write(`Created ${created} GPU resources\n`);
      return;
    }

    const username = readOption("username")?.trim();
    if (!username || !/^[A-Za-z0-9_-]{3,32}$/.test(username)) {
      throw new Error("--username must contain 3-32 letters, digits, _ or -");
    }
    const password = await readHiddenPassword("Admin password: ");
    if (password.length < 8 || password.length > 72) {
      throw new Error("Admin password must contain 8-72 characters");
    }
    const id = await app.get(UsersService).createAdmin(username, password);
    process.stdout.write(`Created admin ${username.toLowerCase()} (${id})\n`);
  } finally {
    await app.close();
  }
}

run().catch((error: unknown) => {
  process.stderr.write(
    `${error instanceof Error ? error.message : String(error)}\n`,
  );
  process.exitCode = 1;
});
