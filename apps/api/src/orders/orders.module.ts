import { Module } from "@nestjs/common";
import { MongooseModule } from "@nestjs/mongoose";

import { AuthModule } from "../auth/auth.module";
import { EnvironmentTemplatesModule } from "../environment-templates/environment-templates.module";
import {
  GpuResource,
  GpuResourceSchema,
} from "../gpu-resources/gpu-resource.schema";
import { InstancesModule } from "../instances/instances.module";
import { Order, OrderSchema } from "./order.schema";
import { OrdersController } from "./orders.controller";
import { OrdersService } from "./orders.service";

@Module({
  imports: [
    AuthModule,
    EnvironmentTemplatesModule,
    InstancesModule,
    MongooseModule.forFeature([
      { name: Order.name, schema: OrderSchema },
      { name: GpuResource.name, schema: GpuResourceSchema },
    ]),
  ],
  controllers: [OrdersController],
  providers: [OrdersService],
  exports: [OrdersService],
})
export class OrdersModule {}
