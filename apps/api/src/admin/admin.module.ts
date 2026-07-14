import { Module } from "@nestjs/common";
import { MongooseModule } from "@nestjs/mongoose";

import { AuthModule } from "../auth/auth.module";
import {
  GpuResource,
  GpuResourceSchema,
} from "../gpu-resources/gpu-resource.schema";
import { GpuResourcesModule } from "../gpu-resources/gpu-resources.module";
import { InstancesModule } from "../instances/instances.module";
import { Order, OrderSchema } from "../orders/order.schema";
import { OrdersModule } from "../orders/orders.module";
import { User, UserSchema } from "../users/user.schema";
import { AdminController } from "./admin.controller";
import { AdminService } from "./admin.service";

@Module({
  imports: [
    AuthModule,
    GpuResourcesModule,
    InstancesModule,
    OrdersModule,
    MongooseModule.forFeature([
      { name: User.name, schema: UserSchema },
      { name: GpuResource.name, schema: GpuResourceSchema },
      { name: Order.name, schema: OrderSchema },
    ]),
  ],
  controllers: [AdminController],
  providers: [AdminService],
})
export class AdminModule {}
