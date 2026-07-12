import {
  Body,
  Controller,
  Get,
  HttpCode,
  HttpStatus,
  Param,
  Patch,
  Post,
  Query,
  UseGuards,
} from "@nestjs/common";
import { ApiCookieAuth, ApiOperation, ApiTags } from "@nestjs/swagger";
import {
  UserRole,
  type AdminOverview,
  type GpuResourceView,
  type OrderView,
  type PaginatedResponse,
} from "@gpu-rental/contracts";

import { Roles } from "../auth/roles.decorator";
import { RolesGuard } from "../auth/roles.guard";
import { SESSION_COOKIE_NAME } from "../auth/session-cookie";
import { SessionAuthGuard } from "../auth/session-auth.guard";
import {
  AdminGpuResourceQueryDto,
  CreateGpuResourceDto,
  SetGpuListingStatusDto,
  UpdateGpuResourceDto,
} from "../gpu-resources/gpu-resources.dto";
import { GpuResourcesService } from "../gpu-resources/gpu-resources.service";
import { AdminOrderQueryDto } from "../orders/orders.dto";
import { OrdersService } from "../orders/orders.service";
import { AdminService } from "./admin.service";

@ApiTags("admin")
@ApiCookieAuth(SESSION_COOKIE_NAME)
@Roles(UserRole.Admin)
@UseGuards(SessionAuthGuard, RolesGuard)
@Controller("admin")
export class AdminController {
  constructor(
    private readonly admin: AdminService,
    private readonly resources: GpuResourcesService,
    private readonly orders: OrdersService,
  ) {}

  @Get("overview")
  getOverview(): Promise<AdminOverview> {
    return this.admin.getOverview();
  }

  @Get("gpu-resources")
  listResources(
    @Query() query: AdminGpuResourceQueryDto,
  ): Promise<PaginatedResponse<GpuResourceView>> {
    return this.resources.listAdmin(query);
  }

  @Post("gpu-resources")
  createResource(
    @Body() input: CreateGpuResourceDto,
  ): Promise<GpuResourceView> {
    return this.resources.create(input);
  }

  @Patch("gpu-resources/:id")
  updateResource(
    @Param("id") id: string,
    @Body() input: UpdateGpuResourceDto,
  ): Promise<GpuResourceView> {
    return this.resources.update(id, input);
  }

  @Patch("gpu-resources/:id/listing-status")
  @ApiOperation({ summary: "Change listing status under the resource lock" })
  setListingStatus(
    @Param("id") id: string,
    @Body() input: SetGpuListingStatusDto,
  ): Promise<GpuResourceView> {
    return this.resources.setListingStatus(id, input.listingStatus);
  }

  @Get("orders")
  listOrders(
    @Query() query: AdminOrderQueryDto,
  ): Promise<PaginatedResponse<OrderView>> {
    return this.orders.listAdmin(query);
  }

  @Post("orders/:id/cancel")
  @HttpCode(HttpStatus.OK)
  cancelOrder(@Param("id") id: string): Promise<OrderView> {
    return this.orders.cancelOrder(id);
  }
}
