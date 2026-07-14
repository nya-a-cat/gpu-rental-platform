import {
  Body,
  Controller,
  Get,
  HttpCode,
  HttpStatus,
  Param,
  Post,
  Query,
  UseGuards,
} from "@nestjs/common";
import { ApiCookieAuth, ApiOperation, ApiTags } from "@nestjs/swagger";
import type { OrderView, PaginatedResponse } from "@gpu-rental/contracts";

import { CurrentUser } from "../auth/current-user.decorator";
import { SESSION_COOKIE_NAME } from "../auth/session-cookie";
import { SessionAuthGuard } from "../auth/session-auth.guard";
import { CloudAccountsService } from "../cloud-accounts/cloud-accounts.service";
import type { SessionIdentity } from "../redis/session.service";
import { InstancesService } from "../instances/instances.service";
import { TeamsService } from "../teams/teams.service";
import { CreateOrderDto, OrderQueryDto } from "./orders.dto";
import { OrdersService } from "./orders.service";

@ApiTags("orders")
@ApiCookieAuth(SESSION_COOKIE_NAME)
@UseGuards(SessionAuthGuard)
@Controller("orders")
export class OrdersController {
  constructor(
    private readonly orders: OrdersService,
    private readonly instances: InstancesService,
    private readonly accounts: CloudAccountsService,
    private readonly teams: TeamsService,
  ) {}

  @Post()
  @ApiOperation({ summary: "Reserve an online GPU resource" })
  async create(
    @CurrentUser() user: SessionIdentity,
    @Body() input: CreateOrderDto,
  ): Promise<OrderView> {
    const order = await this.orders.create(user.userId, input);
    let charged = false;
    let instanceCreated = false;
    try {
      await this.accounts.chargeOrder(order);
      charged = true;
      await this.instances.createForOrder(order);
      instanceCreated = true;
      await this.teams.recordBooking(order.projectId, order.totalPriceCents);
      return order;
    } catch (error) {
      await this.orders.cancelOrder(order.id);
      if (instanceCreated) {
        await this.instances.terminateByOrderId(order.id);
      } else if (charged) {
        await this.accounts.refundUnused(
          order.userId,
          order.id,
          order.totalPriceCents,
          0,
        );
      }
      throw error;
    }
  }

  @Get("me")
  listMine(
    @CurrentUser() user: SessionIdentity,
    @Query() query: OrderQueryDto,
  ): Promise<PaginatedResponse<OrderView>> {
    return this.orders.listMine(user.userId, query);
  }

  @Post(":id/return")
  @HttpCode(HttpStatus.OK)
  @ApiOperation({ summary: "Return an active order" })
  async returnOrder(
    @CurrentUser() user: SessionIdentity,
    @Param("id") orderId: string,
  ): Promise<OrderView> {
    const order = await this.orders.returnOrder(orderId, user.userId);
    await this.instances.terminateByOrderId(orderId);
    return order;
  }
}
