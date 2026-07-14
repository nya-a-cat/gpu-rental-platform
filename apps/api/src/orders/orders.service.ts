import { HttpStatus, Injectable, type OnModuleInit } from "@nestjs/common";
import { InjectModel } from "@nestjs/mongoose";
import { Interval } from "@nestjs/schedule";
import {
  GpuListingStatus,
  OrderStatus,
  type OrderView,
  type PaginatedResponse,
} from "@gpu-rental/contracts";
import { Types, type FilterQuery, type Model } from "mongoose";

import { DomainException } from "../common/domain-exception";
import { EnvironmentTemplatesService } from "../environment-templates/environment-templates.service";
import { isMongoDuplicateKeyError } from "../common/mongo-error";
import {
  GpuResource,
  type GpuResourceDocument,
} from "../gpu-resources/gpu-resource.schema";
import { DistributedLockService } from "../redis/distributed-lock.service";
import { TeamsService } from "../teams/teams.service";
import {
  type AdminOrderQueryDto,
  type CreateOrderDto,
  type OrderQueryDto,
} from "./orders.dto";
import { Order, type OrderDocument } from "./order.schema";

@Injectable()
export class OrdersService implements OnModuleInit {
  constructor(
    @InjectModel(Order.name) private readonly orders: Model<Order>,
    @InjectModel(GpuResource.name)
    private readonly resources: Model<GpuResource>,
    private readonly locks: DistributedLockService,
    private readonly templates: EnvironmentTemplatesService,
    private readonly teams: TeamsService,
  ) {}

  async onModuleInit(): Promise<void> {
    await this.orders.init();
  }

  async create(userId: string, input: CreateOrderDto): Promise<OrderView> {
    const template = this.templates.getById(input.environmentTemplateId);
    const project = await this.teams.resolveProjectForUser(
      userId,
      input.projectId,
    );
    try {
      return await this.locks.withResourceLock(
        input.gpuResourceId,
        async () => {
          const resource = (await this.resources
            .findOne({
              _id: new Types.ObjectId(input.gpuResourceId),
              listingStatus: GpuListingStatus.Online,
            })
            .exec()) as GpuResourceDocument | null;
          if (!resource) {
            throw new DomainException(
              "GPU_RESOURCE_UNAVAILABLE",
              "The GPU resource is not online",
              HttpStatus.CONFLICT,
            );
          }

          const now = new Date();
          await this.orders.updateMany(
            {
              gpuResourceId: resource._id,
              status: OrderStatus.Active,
              endsAt: { $lte: now },
            },
            { $set: { status: OrderStatus.Expired } },
          );
          if (
            await this.orders.exists({
              gpuResourceId: resource._id,
              status: OrderStatus.Active,
            })
          ) {
            throw new DomainException(
              "GPU_RESOURCE_RENTED",
              "The GPU resource already has an active order",
              HttpStatus.CONFLICT,
            );
          }

          const order = (await this.orders.create({
            userId: new Types.ObjectId(userId),
            gpuResourceId: resource._id,
            gpuName: resource.name,
            gpuModel: resource.model,
            gpuMemoryGb: resource.memoryGb,
            gpuCount: resource.gpuCount ?? 1,
            temporaryStorageGb: resource.storageGb ?? 100,
            environmentTemplateId: template.id,
            environmentTemplateName: template.name,
            instanceName:
              input.instanceName?.trim() || `${resource.name} workload`,
            projectId: project?.projectId ?? null,
            projectName: project?.projectName ?? null,
            teamName: project?.teamName ?? null,
            region: resource.region,
            hourlyPriceCents: resource.hourlyPriceCents,
            durationHours: input.durationHours,
            totalPriceCents: resource.hourlyPriceCents * input.durationHours,
            status: OrderStatus.Active,
            startsAt: now,
            endsAt: new Date(now.getTime() + input.durationHours * 3_600_000),
          })) as OrderDocument;
          return this.toView(order);
        },
      );
    } catch (error) {
      if (isMongoDuplicateKeyError(error)) {
        throw new DomainException(
          "GPU_RESOURCE_RENTED",
          "The GPU resource already has an active order",
          HttpStatus.CONFLICT,
        );
      }
      throw error;
    }
  }

  listMine(
    userId: string,
    query: OrderQueryDto,
  ): Promise<PaginatedResponse<OrderView>> {
    return this.list({ ...query, userId });
  }

  listAdmin(query: AdminOrderQueryDto): Promise<PaginatedResponse<OrderView>> {
    return this.list(query);
  }

  async returnOrder(orderId: string, userId: string): Promise<OrderView> {
    this.assertObjectId(orderId);
    const orderObjectId = new Types.ObjectId(orderId);
    const userObjectId = new Types.ObjectId(userId);
    const transitioned = (await this.orders
      .findOneAndUpdate(
        {
          _id: orderObjectId,
          userId: userObjectId,
          status: OrderStatus.Active,
        },
        {
          $set: {
            status: OrderStatus.Returned,
            returnedAt: new Date(),
          },
        },
        { new: true },
      )
      .exec()) as OrderDocument | null;
    if (transitioned) {
      return this.toView(transitioned);
    }

    const existing = (await this.orders
      .findOne({ _id: orderObjectId, userId: userObjectId })
      .exec()) as OrderDocument | null;
    if (!existing) {
      this.throwNotFound();
    }
    if (existing.status === OrderStatus.Returned) {
      return this.toView(existing);
    }
    this.throwTerminalConflict();
  }

  async cancelOrder(orderId: string): Promise<OrderView> {
    this.assertObjectId(orderId);
    const orderObjectId = new Types.ObjectId(orderId);
    const transitioned = (await this.orders
      .findOneAndUpdate(
        { _id: orderObjectId, status: OrderStatus.Active },
        {
          $set: {
            status: OrderStatus.Cancelled,
            cancelledAt: new Date(),
          },
        },
        { new: true },
      )
      .exec()) as OrderDocument | null;
    if (transitioned) {
      return this.toView(transitioned);
    }

    const existing = (await this.orders
      .findById(orderObjectId)
      .exec()) as OrderDocument | null;
    if (!existing) {
      this.throwNotFound();
    }
    if (existing.status === OrderStatus.Cancelled) {
      return this.toView(existing);
    }
    this.throwTerminalConflict();
  }

  @Interval(60_000)
  async expireDueOrders(): Promise<number> {
    const result = await this.orders.updateMany(
      { status: OrderStatus.Active, endsAt: { $lte: new Date() } },
      { $set: { status: OrderStatus.Expired } },
    );
    return result.modifiedCount;
  }

  private async list(
    query: AdminOrderQueryDto,
  ): Promise<PaginatedResponse<OrderView>> {
    const filter: FilterQuery<Order> = {};
    if (query.userId) filter.userId = new Types.ObjectId(query.userId);
    if (query.gpuResourceId) {
      filter.gpuResourceId = new Types.ObjectId(query.gpuResourceId);
    }
    if (query.status) filter.status = query.status;
    const [items, total] = await Promise.all([
      this.orders
        .find(filter)
        .sort({ createdAt: -1 })
        .skip((query.page - 1) * query.pageSize)
        .limit(query.pageSize)
        .exec(),
      this.orders.countDocuments(filter).exec(),
    ]);
    return {
      items: (items as OrderDocument[]).map((item) => this.toView(item)),
      page: query.page,
      pageSize: query.pageSize,
      total,
    };
  }

  private toView(order: OrderDocument): OrderView {
    return {
      id: order._id.toString(),
      userId: order.userId.toString(),
      gpuResourceId: order.gpuResourceId.toString(),
      gpuName: order.gpuName,
      gpuModel: order.gpuModel,
      gpuMemoryGb: order.gpuMemoryGb,
      gpuCount: order.gpuCount ?? 1,
      temporaryStorageGb: order.temporaryStorageGb ?? 100,
      environmentTemplateId: order.environmentTemplateId ?? "pytorch-jupyter",
      environmentTemplateName:
        order.environmentTemplateName ?? "PyTorch + JupyterLab",
      instanceName: order.instanceName ?? `${order.gpuName} workload`,
      projectId: order.projectId ?? null,
      projectName: order.projectName ?? null,
      teamName: order.teamName ?? null,
      region: order.region,
      hourlyPriceCents: order.hourlyPriceCents,
      durationHours: order.durationHours,
      totalPriceCents: order.totalPriceCents,
      status: order.status,
      startsAt: order.startsAt.toISOString(),
      endsAt: order.endsAt.toISOString(),
      returnedAt: order.returnedAt?.toISOString() ?? null,
      cancelledAt: order.cancelledAt?.toISOString() ?? null,
      createdAt: order.createdAt.toISOString(),
      updatedAt: order.updatedAt.toISOString(),
    };
  }

  private assertObjectId(id: string): void {
    if (!Types.ObjectId.isValid(id)) {
      this.throwNotFound();
    }
  }

  private throwNotFound(): never {
    throw new DomainException(
      "ORDER_NOT_FOUND",
      "The order was not found",
      HttpStatus.NOT_FOUND,
    );
  }

  private throwTerminalConflict(): never {
    throw new DomainException(
      "ORDER_ALREADY_TERMINAL",
      "A terminal order cannot change status",
      HttpStatus.CONFLICT,
    );
  }
}
