import { HttpStatus, Injectable, type OnModuleInit } from "@nestjs/common";
import { InjectModel } from "@nestjs/mongoose";
import { Interval } from "@nestjs/schedule";
import {
  ConnectionMode,
  InstanceStatus,
  NotificationType,
  OrderStatus,
  type EnvironmentTemplateView,
  type InstanceView,
  type OrderView,
  type PaginatedResponse,
} from "@gpu-rental/contracts";
import { Types, type QueryFilter, type Model } from "mongoose";

import { DomainException } from "../common/domain-exception";
import { CloudAccountsService } from "../cloud-accounts/cloud-accounts.service";
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
    private readonly accounts: CloudAccountsService,
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
      temporaryStorageGb: order.temporaryStorageGb,
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
    const view = this.toView(instance, now);
    await this.accounts.addNotification(
      order.userId,
      NotificationType.Instance,
      "Instance running",
      `${order.instanceName} is ready with simulated access details.`,
    );
    return view;
  }

  async listMine(
    userId: string,
    query: InstanceQueryDto,
  ): Promise<PaginatedResponse<InstanceView>> {
    const filter: QueryFilter<Instance> = {
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
    await this.accounts.addNotification(
      userId,
      NotificationType.Instance,
      "Instance started",
      `${instance.name} resumed billable runtime.`,
    );
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
    await this.accounts.addNotification(
      userId,
      NotificationType.Instance,
      "Instance stopped",
      `${instance.name} stopped accruing runtime charges.`,
    );
    return this.toView(instance, now);
  }

  async terminate(id: string, userId: string): Promise<InstanceView> {
    const instance = await this.findMine(id, userId);
    const now = new Date();
    await this.terminateDocument(instance, now, true);
    return this.toView(instance, now);
  }

  async terminateByOrderId(orderId: string): Promise<InstanceView | null> {
    if (!Types.ObjectId.isValid(orderId)) return null;
    const instance = (await this.instances
      .findOne({ orderId: new Types.ObjectId(orderId) })
      .exec()) as InstanceDocument | null;
    if (!instance) return null;
    const now = new Date();
    await this.terminateDocument(instance, now, false);
    return this.toView(instance, now);
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
    let transitioned = false;
    if (instance.status !== InstanceStatus.Terminated) {
      this.accumulateRunningTime(instance, now);
      instance.status = InstanceStatus.Terminated;
      instance.terminatedAt = now;
      await instance.save();
      transitioned = true;
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
    const view = this.toView(instance, now);
    await this.accounts.refundUnused(
      view.userId,
      view.orderId,
      view.maximumCostCents,
      view.accruedCostCents,
    );
    await this.accounts.releaseInstanceResources(view.userId, view.id);
    if (transitioned) {
      await this.accounts.addNotification(
        view.userId,
        NotificationType.Instance,
        "Instance terminated",
        `${view.name} was terminated and unused booked value was reconciled.`,
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
      temporaryStorageGb: instance.temporaryStorageGb ?? 100,
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
