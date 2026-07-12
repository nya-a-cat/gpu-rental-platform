import "reflect-metadata";

import { Logger, ValidationPipe } from "@nestjs/common";
import { ConfigService } from "@nestjs/config";
import { NestFactory } from "@nestjs/core";
import { DocumentBuilder, SwaggerModule } from "@nestjs/swagger";
import cookieParser from "cookie-parser";

import { AppModule } from "./app.module";
import { SESSION_COOKIE_NAME } from "./auth/session-cookie";

async function bootstrap(): Promise<void> {
  const app = await NestFactory.create(AppModule);
  const config = app.get(ConfigService);
  app.use(cookieParser());
  app.useGlobalPipes(
    new ValidationPipe({
      forbidNonWhitelisted: true,
      transform: true,
      whitelist: true,
    }),
  );
  app.enableCors({
    credentials: true,
    origin: config.getOrThrow<string>("WEB_ORIGIN"),
  });
  app.setGlobalPrefix("api");
  app.enableShutdownHooks();

  const swaggerConfig = new DocumentBuilder()
    .setTitle("GPU Rental Platform API")
    .setDescription("Resource marketplace, session and order management API")
    .setVersion("0.1.0")
    .addCookieAuth(SESSION_COOKIE_NAME)
    .build();
  SwaggerModule.setup(
    "api/docs",
    app,
    SwaggerModule.createDocument(app, swaggerConfig),
  );

  const port = config.get<number>("API_PORT", 4000);
  await app.listen(port, "0.0.0.0");
  Logger.log(`API listening on port ${port}`, "Bootstrap");
}

void bootstrap();
