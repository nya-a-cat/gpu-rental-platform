import { Injectable } from "@nestjs/common";
import { InjectModel } from "@nestjs/mongoose";
import {
  GpuListingStatus,
  OrderStatus,
  type AdminOverview,
} from "@gpu-rental/contracts";
import type { Model } from "mongoose";

import { GpuResource } from "../gpu-resources/gpu-resource.schema";
import { Order } from "../orders/order.schema";
import { User } from "../users/user.schema";

@Injectable()
export class AdminService {
  constructor(
    @InjectModel(User.name) private readonly users: Model<User>,
    @InjectModel(GpuResource.name)
    private readonly resources: Model<GpuResource>,
    @InjectModel(Order.name) private readonly orders: Model<Order>,
  ) {}

  async getOverview(): Promise<AdminOverview> {
    const [
      usersTotal,
      resourcesTotal,
      resourcesOnline,
      activeOrders,
      terminalOrders,
      revenue,
    ] = await Promise.all([
      this.users.countDocuments().exec(),
      this.resources.countDocuments().exec(),
      this.resources
        .countDocuments({ listingStatus: GpuListingStatus.Online })
        .exec(),
      this.orders.countDocuments({ status: OrderStatus.Active }).exec(),
      this.orders
        .countDocuments({ status: { $ne: OrderStatus.Active } })
        .exec(),
      this.orders
        .aggregate<{
          total: number;
        }>([
          { $match: { status: { $ne: OrderStatus.Cancelled } } },
          { $group: { _id: null, total: { $sum: "$totalPriceCents" } } },
        ])
        .exec(),
    ]);
    return {
      usersTotal,
      resourcesTotal,
      resourcesOnline,
      activeOrders,
      terminalOrders,
      bookedRevenueCents: revenue[0]?.total ?? 0,
    };
  }
}
