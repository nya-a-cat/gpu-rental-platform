import { Module } from "@nestjs/common";
import { MongooseModule } from "@nestjs/mongoose";

import { AuthModule } from "../auth/auth.module";
import { EnvironmentTemplatesModule } from "../environment-templates/environment-templates.module";
import { Order, OrderSchema } from "../orders/order.schema";
import { Instance, InstanceSchema } from "./instance.schema";
import { InstancesController } from "./instances.controller";
import { InstancesService } from "./instances.service";

@Module({
  imports: [
    AuthModule,
    EnvironmentTemplatesModule,
    MongooseModule.forFeature([
      { name: Instance.name, schema: InstanceSchema },
      { name: Order.name, schema: OrderSchema },
    ]),
  ],
  controllers: [InstancesController],
  providers: [InstancesService],
  exports: [InstancesService],
})
export class InstancesModule {}
