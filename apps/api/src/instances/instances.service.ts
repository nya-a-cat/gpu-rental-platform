import { HttpStatus, Injectable, type OnModuleInit } from "@nestjs/common";
import { InjectModel } from "@nestjs/mongoose";
import { Interval } from "@nestjs/schedule";
import {
  ConnectionMode,
  InstanceStatus,
  OrderStatus,
  type EnvironmentTemplateView,
  type InstanceView,
  type OrderView,
  type PaginatedResponse,
} from "@gpu-rental/contracts";
import { Types, type FilterQuery, type Model } from "mongoose";

import { DomainException } from "../common/domain-exception";
import { EnvironmentTemplatesService } from "../environment-templates/environment-templates.service";
import { Order } from "../orders/order.schema";
import type { InstanceQueryDto } from "./instances.dto";
import { Instance, type InstanceDocument } from "./instance.schema";

@Injectable()
export class InstancesService implements OnModuleInit {
  constructor(
    @InjectModel(Instance.name) private readonly instances: Model<Instance>,
    @InjectModel(Order.name) private readonly orders: Model<Order>,
    private readonly templates: EnvironmentTemplatesService,
  ) {}

  async onModuleInit(): Promise<void> {
    await this.instances.init();
  }

  resolveTemplate(id?: string): EnvironmentTemplateView {
    return this.templates.getById(id);
  }

  async createForOrder(order: OrderView): Promise<InstanceView> {
    const template = this.templates.getById(order.environmentTemplateId);
    const now = new Date();
    const instance = (await this.instances.create({
      orderId: new Types.ObjectId(order.id),
      userId: new Types.ObjectId(order.userId),
      gpuResourceId: new Types.ObjectId(order.gpuResourceId),
      name: order.instanceName,
      gpuName: order.gpuName,
      gpuModel: order.gpuModel,
      gpuCount: order.gpuCount,
      gpuMemoryGb: order.gpuMemoryGb,
      environmentTemplateId: template.id,
      environmentTemplateName: template.name,
      environmentImage: template.image,
      connectionModes: template.connectionModes,
      hourlyPriceCents: order.hourlyPriceCents,
      maximumCostCents: order.totalPriceCents,
      status: InstanceStatus.Running,
      simulated: true,
      startsAt: new Date(order.startsAt),
      endsAt: new Date(order.endsAt),
      runningSince: now,
      accumulatedRunSeconds: 0,
    })) as InstanceDocument;
    return this.toView(instance, now);
  }

  async listMine(
    userId: string,
    query: InstanceQueryDto,
  ): Promise<PaginatedResponse<InstanceView>> {
    const filter: FilterQuery<Instance> = {
      userId: new Types.ObjectId(userId),
    };
    if (query.status) filter.status = query.status;
    const [items, total] = await Promise.all([
      this.instances
        .find(filter)
        .sort({ createdAt: -1 })
        .skip((query.page - 1) * query.pageSize)
        .limit(query.pageSize)
        .exec(),
      this.instances.countDocuments(filter).exec(),
    ]);
    const now = new Date();
    return {
      items: (items as InstanceDocument[]).map((item) =>
        this.toView(item, now),
      ),
      page: query.page,
      pageSize: query.pageSize,
      total,
    };
  }

  async getMine(id: string, userId: string): Promise<InstanceView> {
    return this.toView(await this.findMine(id, userId), new Date());
  }

  async start(id: string, userId: string): Promise<InstanceView> {
    const instance = await this.findMine(id, userId);
    if (instance.status === InstanceStatus.Running) {
      return this.toView(instance, new Date());
    }
    this.assertCanTransition(instance);
    const now = new Date();
    if (now >= instance.endsAt) {
      await this.terminateDocument(instance, now, false);
      throw new DomainException(
        "INSTANCE_LEASE_EXPIRED",
        "The instance lease has expired",
        HttpStatus.CONFLICT,
      );
    }
    const activeOrder = await this.orders.exists({
      _id: instance.orderId,
      userId: instance.userId,
      status: OrderStatus.Active,
    });
    if (!activeOrder) {
      throw new DomainException(
        "INSTANCE_ORDER_INACTIVE",
        "The instance order is no longer active",
        HttpStatus.CONFLICT,
      );
    }
    instance.status = InstanceStatus.Running;
    instance.runningSince = now;
    instance.stoppedAt = null;
    await instance.save();
    return this.toView(instance, now);
  }

  async stop(id: string, userId: string): Promise<InstanceView> {
    const instance = await this.findMine(id, userId);
    if (instance.status === InstanceStatus.Stopped) {
      return this.toView(instance, new Date());
    }
    this.assertCanTransition(instance);
    const now = new Date();
    this.accumulateRunningTime(instance, now);
    instance.status = InstanceStatus.Stopped;
    instance.stoppedAt = now;
    await instance.save();
    return this.toView(instance, now);
  }

  async terminate(id: string, userId: string): Promise<InstanceView> {
    const instance = await this.findMine(id, userId);
    const now = new Date();
    await this.terminateDocument(instance, now, true);
    return this.toView(instance, now);
  }

  async terminateByOrderId(orderId: string): Promise<void> {
    if (!Types.ObjectId.isValid(orderId)) return;
    const instance = (await this.instances
      .findOne({ orderId: new Types.ObjectId(orderId) })
      .exec()) as InstanceDocument | null;
    if (!instance) return;
    await this.terminateDocument(instance, new Date(), false);
  }

  @Interval(60_000)
  async terminateExpiredInstances(): Promise<number> {
    const now = new Date();
    const expired = (await this.instances
      .find({
        status: { $in: [InstanceStatus.Running, InstanceStatus.Stopped] },
        endsAt: { $lte: now },
      })
      .exec()) as InstanceDocument[];
    for (const instance of expired) {
      await this.terminateDocument(instance, now, false);
    }
    return expired.length;
  }

  private async terminateDocument(
    instance: InstanceDocument,
    now: Date,
    returnOrder: boolean,
  ): Promise<void> {
    if (instance.status !== InstanceStatus.Terminated) {
      this.accumulateRunningTime(instance, now);
      instance.status = InstanceStatus.Terminated;
      instance.terminatedAt = now;
      await instance.save();
    }
    if (returnOrder) {
      await this.orders.updateOne(
        {
          _id: instance.orderId,
          userId: instance.userId,
          status: OrderStatus.Active,
        },
        {
          $set: { status: OrderStatus.Returned, returnedAt: now },
        },
      );
    }
  }

  private accumulateRunningTime(instance: InstanceDocument, now: Date): void {
    if (instance.runningSince) {
      instance.accumulatedRunSeconds += Math.max(
        0,
        Math.ceil((now.getTime() - instance.runningSince.getTime()) / 1000),
      );
      instance.runningSince = null;
    }
  }

  private assertCanTransition(instance: InstanceDocument): void {
    if (
      instance.status === InstanceStatus.Terminated ||
      instance.status === InstanceStatus.Failed
    ) {
      throw new DomainException(
        "INSTANCE_TERMINAL",
        "A terminal instance cannot change status",
        HttpStatus.CONFLICT,
      );
    }
  }

  private async findMine(
    id: string,
    userId: string,
  ): Promise<InstanceDocument> {
    if (!Types.ObjectId.isValid(id)) this.throwNotFound();
    const instance = (await this.instances
      .findOne({
        _id: new Types.ObjectId(id),
        userId: new Types.ObjectId(userId),
      })
      .exec()) as InstanceDocument | null;
    if (!instance) this.throwNotFound();
    return instance;
  }

  private toView(instance: InstanceDocument, now: Date): InstanceView {
    const billableSeconds = Math.min(
      Math.ceil(
        (instance.endsAt.getTime() - instance.startsAt.getTime()) / 1000,
      ),
      instance.accumulatedRunSeconds +
        (instance.runningSince
          ? Math.max(
              0,
              Math.ceil(
                (now.getTime() - instance.runningSince.getTime()) / 1000,
              ),
            )
          : 0),
    );
    const accruedCostCents = Math.min(
      instance.maximumCostCents,
      Math.ceil((instance.hourlyPriceCents * billableSeconds) / 3600),
    );
    const slug = instance._id.toString().slice(-12);
    const host = `${slug}.simulated.invalid`;
    const modes = new Set(instance.connectionModes);
    return {
      id: instance._id.toString(),
      orderId: instance.orderId.toString(),
      userId: instance.userId.toString(),
      gpuResourceId: instance.gpuResourceId.toString(),
      name: instance.name,
      gpuName: instance.gpuName,
      gpuModel: instance.gpuModel,
      gpuCount: instance.gpuCount,
      gpuMemoryGb: instance.gpuMemoryGb,
      environmentTemplateId: instance.environmentTemplateId,
      environmentTemplateName: instance.environmentTemplateName,
      environmentImage: instance.environmentImage,
      status: instance.status,
      simulated: instance.simulated,
      startsAt: instance.startsAt.toISOString(),
      endsAt: instance.endsAt.toISOString(),
      runningSince: instance.runningSince?.toISOString() ?? null,
      stoppedAt: instance.stoppedAt?.toISOString() ?? null,
      terminatedAt: instance.terminatedAt?.toISOString() ?? null,
      billableSeconds,
      accruedCostCents,
      maximumCostCents: instance.maximumCostCents,
      access: {
        sshCommand: modes.has(ConnectionMode.Ssh)
          ? `ssh operator@${host}`
          : null,
        jupyterUrl: modes.has(ConnectionMode.Jupyter)
          ? `https://${host}/jupyter`
          : null,
        webTerminalUrl: modes.has(ConnectionMode.WebTerminal)
          ? `https://${host}/terminal`
          : null,
        notice:
          "Simulation only. These .invalid endpoints cannot connect to physical infrastructure.",
      },
      createdAt: instance.createdAt.toISOString(),
      updatedAt: instance.updatedAt.toISOString(),
    };
  }

  private throwNotFound(): never {
    throw new DomainException(
      "INSTANCE_NOT_FOUND",
      "The instance was not found",
      HttpStatus.NOT_FOUND,
    );
  }
}
