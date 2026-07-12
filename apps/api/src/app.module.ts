import { resolve } from "node:path";

import {
  type MiddlewareConsumer,
  Module,
  type NestModule,
  RequestMethod,
} from "@nestjs/common";
import { ConfigModule, ConfigService } from "@nestjs/config";
import { APP_FILTER } from "@nestjs/core";
import { MongooseModule } from "@nestjs/mongoose";
import { ScheduleModule } from "@nestjs/schedule";

import { AdminModule } from "./admin/admin.module";
import { AuthModule } from "./auth/auth.module";
import { validateEnvironment } from "./common/environment";
import { HttpExceptionFilter } from "./common/http-exception.filter";
import { RequestIdMiddleware } from "./common/request-id.middleware";
import { GpuResourcesModule } from "./gpu-resources/gpu-resources.module";
import { HealthModule } from "./health/health.module";
import { OrdersModule } from "./orders/orders.module";
import { RedisModule } from "./redis/redis.module";
import { UsersModule } from "./users/users.module";

@Module({
  imports: [
    ConfigModule.forRoot({
      cache: true,
      envFilePath: resolve(__dirname, "../../../.env"),
      isGlobal: true,
      validate: validateEnvironment,
    }),
    MongooseModule.forRootAsync({
      inject: [ConfigService],
      useFactory: (config: ConfigService) => ({
        uri: config.getOrThrow<string>("MONGO_URI"),
        autoIndex: true,
        maxPoolSize: 20,
        retryAttempts: 3,
        retryDelay: 1_000,
      }),
    }),
    ScheduleModule.forRoot(),
    RedisModule,
    AuthModule,
    UsersModule,
    GpuResourcesModule,
    OrdersModule,
    AdminModule,
    HealthModule,
  ],
  providers: [{ provide: APP_FILTER, useClass: HttpExceptionFilter }],
})
export class AppModule implements NestModule {
  configure(consumer: MiddlewareConsumer): void {
    consumer
      .apply(RequestIdMiddleware)
      .forRoutes({ path: "{*path}", method: RequestMethod.ALL });
  }
}
