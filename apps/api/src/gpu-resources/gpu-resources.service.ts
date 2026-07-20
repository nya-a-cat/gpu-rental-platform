import { HttpStatus, Injectable, type OnModuleInit } from "@nestjs/common";
import { InjectModel } from "@nestjs/mongoose";
import {
  GpuAvailability,
  GpuListingStatus,
  OrderStatus,
  ResourceMode,
  type GpuResourceFacets,
  type GpuResourceView,
  type PaginatedResponse,
} from "@gpu-rental/contracts";
import {
  Types,
  type QueryFilter,
  type Model,
  type PipelineStage,
} from "mongoose";

import { DomainException } from "../common/domain-exception";
import { isMongoDuplicateKeyError } from "../common/mongo-error";
import { Order } from "../orders/order.schema";
import { DistributedLockService } from "../redis/distributed-lock.service";
import {
  AdminGpuResourceQueryDto,
  type CreateGpuResourceDto,
  type GpuResourceQueryDto,
  GpuSort,
  type UpdateGpuResourceDto,
} from "./gpu-resources.dto";
import { GpuResource, type GpuResourceDocument } from "./gpu-resource.schema";

interface GpuAggregateRecord extends GpuResource {
  _id: Types.ObjectId;
  availability: GpuAvailability;
}

interface GpuFacetResult {
  items: GpuAggregateRecord[];
  metadata: Array<{ total: number }>;
}

const DEMO_RESOURCES: CreateGpuResourceDto[] = [
  {
    name: "cn-east-h100-01",
    model: "NVIDIA H100",
    memoryGb: 80,
    region: "cn-east",
    hourlyPriceCents: 3290,
    tags: ["Hopper", "80GB"],
    listingStatus: GpuListingStatus.Online,
  },
  {
    name: "cn-east-a100-01",
    model: "NVIDIA A100",
    memoryGb: 80,
    region: "cn-east",
    hourlyPriceCents: 2290,
    tags: ["Ampere", "80GB"],
    listingStatus: GpuListingStatus.Online,
  },
  {
    name: "cn-north-a100-01",
    model: "NVIDIA A100",
    memoryGb: 40,
    region: "cn-north",
    hourlyPriceCents: 1690,
    tags: ["Ampere", "40GB"],
    listingStatus: GpuListingStatus.Online,
  },
  {
    name: "cn-south-4090-01",
    model: "NVIDIA RTX 4090",
    memoryGb: 24,
    region: "cn-south",
    hourlyPriceCents: 690,
    tags: ["Ada", "24GB"],
    listingStatus: GpuListingStatus.Online,
  },
];

@Injectable()
export class GpuResourcesService implements OnModuleInit {
  constructor(
    @InjectModel(GpuResource.name)
    private readonly resources: Model<GpuResource>,
    @InjectModel(Order.name) private readonly orders: Model<Order>,
    private readonly locks: DistributedLockService,
  ) {}

  async onModuleInit(): Promise<void> {
    await this.resources.init();
  }

  listPublic(
    query: GpuResourceQueryDto,
  ): Promise<PaginatedResponse<GpuResourceView>> {
    return this.aggregateList(query, true);
  }

  listAdmin(
    query: AdminGpuResourceQueryDto,
  ): Promise<PaginatedResponse<GpuResourceView>> {
    return this.aggregateList(query, false);
  }

  async getPublicById(id: string): Promise<GpuResourceView> {
    return this.getById(id, true);
  }

  async getAdminById(id: string): Promise<GpuResourceView> {
    return this.getById(id, false);
  }

  async getFacets(): Promise<GpuResourceFacets> {
    const filter = { listingStatus: GpuListingStatus.Online };
    const [models, regions, memoryGbValues, maximum] = await Promise.all([
      this.resources.distinct("model", filter),
      this.resources.distinct("region", filter),
      this.resources.distinct("memoryGb", filter),
      this.resources
        .findOne(filter)
        .sort({ hourlyPriceCents: -1 })
        .select({ hourlyPriceCents: 1 })
        .lean()
        .exec(),
    ]);
    return {
      models: models.sort(),
      regions: regions.sort(),
      memoryGbValues: memoryGbValues.sort((left, right) => left - right),
      maxHourlyPriceCents: maximum?.hourlyPriceCents ?? 0,
    };
  }

  async create(input: CreateGpuResourceDto): Promise<GpuResourceView> {
    try {
      const resource = (await this.resources.create({
        ...input,
        name: input.name.trim(),
        model: input.model.trim(),
        region: input.region.trim(),
        resourceMode: ResourceMode.Simulated,
        listingStatus: input.listingStatus ?? GpuListingStatus.Offline,
      })) as GpuResourceDocument;
      return this.getAdminById(resource._id.toString());
    } catch (error) {
      this.throwNameConflict(error);
      throw error;
    }
  }

  async update(
    id: string,
    input: UpdateGpuResourceDto,
  ): Promise<GpuResourceView> {
    this.assertObjectId(id);
    try {
      const resource = await this.resources
        .findByIdAndUpdate(
          id,
          { $set: input },
          { new: true, runValidators: true },
        )
        .exec();
      if (!resource) {
        this.throwNotFound();
      }
      return this.getAdminById(id);
    } catch (error) {
      this.throwNameConflict(error);
      throw error;
    }
  }

  async setListingStatus(
    id: string,
    listingStatus: GpuListingStatus,
  ): Promise<GpuResourceView> {
    this.assertObjectId(id);
    return this.locks.withResourceLock(id, async () => {
      const resource = (await this.resources
        .findById(id)
        .exec()) as GpuResourceDocument | null;
      if (!resource) {
        this.throwNotFound();
      }
      if (resource.listingStatus === listingStatus) {
        return this.getAdminById(id);
      }

      if (listingStatus !== GpuListingStatus.Online) {
        const now = new Date();
        await this.orders.updateMany(
          {
            gpuResourceId: resource._id,
            status: OrderStatus.Active,
            endsAt: { $lte: now },
          },
          { $set: { status: OrderStatus.Expired } },
        );
        const activeOrder = await this.orders.exists({
          gpuResourceId: resource._id,
          status: OrderStatus.Active,
        });
        if (activeOrder) {
          throw new DomainException(
            "RESOURCE_IN_USE",
            "An active order must finish before the resource can be taken offline",
            HttpStatus.CONFLICT,
          );
        }
      }

      resource.listingStatus = listingStatus;
      await resource.save();
      return this.getAdminById(id);
    });
  }

  async seedDemoResources(): Promise<number> {
    let created = 0;
    for (const resource of DEMO_RESOURCES) {
      const result = await this.resources.updateOne(
        { name: resource.name },
        {
          $setOnInsert: {
            ...resource,
            resourceMode: ResourceMode.Simulated,
          },
        },
        { upsert: true },
      );
      created += result.upsertedCount;
    }
    return created;
  }

  private async aggregateList(
    query: AdminGpuResourceQueryDto,
    onlineOnly: boolean,
  ): Promise<PaginatedResponse<GpuResourceView>> {
    const match: QueryFilter<GpuResource> = {};
    if (onlineOnly) {
      match.listingStatus = GpuListingStatus.Online;
    } else if (query.listingStatus) {
      match.listingStatus = query.listingStatus;
    }
    if (query.model) match.model = query.model;
    if (query.region) match.region = query.region;
    if (query.memoryGb) match.memoryGb = query.memoryGb;
    if (query.maxHourlyPriceCents !== undefined) {
      match.hourlyPriceCents = { $lte: query.maxHourlyPriceCents };
    }

    const sort = this.resolveSort(query.sort);
    const pipeline: PipelineStage[] = [
      { $match: match },
      {
        $lookup: {
          from: "orders",
          let: { resourceId: "$_id" },
          pipeline: [
            {
              $match: {
                $expr: {
                  $and: [
                    { $eq: ["$gpuResourceId", "$$resourceId"] },
                    { $eq: ["$status", OrderStatus.Active] },
                  ],
                },
              },
            },
            { $limit: 1 },
          ],
          as: "activeOrders",
        },
      },
      {
        $addFields: {
          availability: {
            $cond: [
              { $gt: [{ $size: "$activeOrders" }, 0] },
              GpuAvailability.Rented,
              GpuAvailability.Available,
            ],
          },
        },
      },
    ];
    if (query.availability) {
      pipeline.push({ $match: { availability: query.availability } });
    }
    pipeline.push({
      $facet: {
        items: [
          { $sort: sort },
          { $skip: (query.page - 1) * query.pageSize },
          { $limit: query.pageSize },
        ],
        metadata: [{ $count: "total" }],
      },
    });

    const [result] = await this.resources
      .aggregate<GpuFacetResult>(pipeline)
      .exec();
    return {
      items: (result?.items ?? []).map((item) => this.toView(item)),
      page: query.page,
      pageSize: query.pageSize,
      total: result?.metadata[0]?.total ?? 0,
    };
  }

  private async getById(
    id: string,
    onlineOnly: boolean,
  ): Promise<GpuResourceView> {
    this.assertObjectId(id);
    const match: PipelineStage.Match = {
      $match: {
        _id: new Types.ObjectId(id),
        ...(onlineOnly ? { listingStatus: GpuListingStatus.Online } : {}),
      },
    };
    const [resource] = await this.resources
      .aggregate<GpuAggregateRecord>([
        match,
        {
          $lookup: {
            from: "orders",
            let: { resourceId: "$_id" },
            pipeline: [
              {
                $match: {
                  $expr: {
                    $and: [
                      { $eq: ["$gpuResourceId", "$$resourceId"] },
                      { $eq: ["$status", OrderStatus.Active] },
                    ],
                  },
                },
              },
              { $limit: 1 },
            ],
            as: "activeOrders",
          },
        },
        {
          $addFields: {
            availability: {
              $cond: [
                { $gt: [{ $size: "$activeOrders" }, 0] },
                GpuAvailability.Rented,
                GpuAvailability.Available,
              ],
            },
          },
        },
      ])
      .exec();
    if (!resource) {
      this.throwNotFound();
    }
    return this.toView(resource);
  }

  private toView(resource: GpuAggregateRecord): GpuResourceView {
    return {
      id: resource._id.toString(),
      name: resource.name,
      model: resource.model,
      memoryGb: resource.memoryGb,
      gpuCount: resource.gpuCount ?? 1,
      cpuCores: resource.cpuCores ?? 16,
      systemMemoryGb: resource.systemMemoryGb ?? 64,
      storageGb: resource.storageGb ?? 100,
      cudaVersion: resource.cudaVersion ?? "12.4",
      driverVersion: resource.driverVersion ?? "550",
      bandwidthMbps: resource.bandwidthMbps ?? 1000,
      reliabilityPercent: resource.reliabilityPercent ?? 99.9,
      region: resource.region,
      hourlyPriceCents: resource.hourlyPriceCents,
      tags: resource.tags,
      resourceMode: resource.resourceMode,
      listingStatus: resource.listingStatus,
      availability: resource.availability,
      createdAt: resource.createdAt.toISOString(),
      updatedAt: resource.updatedAt.toISOString(),
    };
  }

  private resolveSort(sort: GpuSort): Record<string, 1 | -1> {
    if (sort === GpuSort.PriceAsc) {
      return { hourlyPriceCents: 1, createdAt: -1 };
    }
    if (sort === GpuSort.PriceDesc) {
      return { hourlyPriceCents: -1, createdAt: -1 };
    }
    return { createdAt: -1 };
  }

  private assertObjectId(id: string): void {
    if (!Types.ObjectId.isValid(id)) {
      this.throwNotFound();
    }
  }

  private throwNotFound(): never {
    throw new DomainException(
      "GPU_RESOURCE_NOT_FOUND",
      "The GPU resource was not found",
      HttpStatus.NOT_FOUND,
    );
  }

  private throwNameConflict(error: unknown): void {
    if (isMongoDuplicateKeyError(error)) {
      throw new DomainException(
        "GPU_RESOURCE_NAME_TAKEN",
        "The resource name is already in use",
        HttpStatus.CONFLICT,
      );
    }
  }
}
