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
import type { SessionIdentity } from "../redis/session.service";
import { CreateOrderDto, OrderQueryDto } from "./orders.dto";
import { OrdersService } from "./orders.service";

@ApiTags("orders")
@ApiCookieAuth(SESSION_COOKIE_NAME)
@UseGuards(SessionAuthGuard)
@Controller("orders")
export class OrdersController {
  constructor(private readonly orders: OrdersService) {}

  @Post()
  @ApiOperation({ summary: "Reserve an online GPU resource" })
  create(
    @CurrentUser() user: SessionIdentity,
    @Body() input: CreateOrderDto,
  ): Promise<OrderView> {
    return this.orders.create(user.userId, input);
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
  returnOrder(
    @CurrentUser() user: SessionIdentity,
    @Param("id") orderId: string,
  ): Promise<OrderView> {
    return this.orders.returnOrder(orderId, user.userId);
  }
}
