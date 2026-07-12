import { Module } from "@nestjs/common";
import { MongooseModule } from "@nestjs/mongoose";

import { Order, OrderSchema } from "../orders/order.schema";
import { GpuResource, GpuResourceSchema } from "./gpu-resource.schema";
import { GpuResourcesController } from "./gpu-resources.controller";
import { GpuResourcesService } from "./gpu-resources.service";

@Module({
  imports: [
    MongooseModule.forFeature([
      { name: GpuResource.name, schema: GpuResourceSchema },
      { name: Order.name, schema: OrderSchema },
    ]),
  ],
  controllers: [GpuResourcesController],
  providers: [GpuResourcesService],
  exports: [GpuResourcesService],
})
export class GpuResourcesModule {}
