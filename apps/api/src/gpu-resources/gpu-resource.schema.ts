import { Prop, Schema, SchemaFactory } from "@nestjs/mongoose";
import { GpuListingStatus, ResourceMode } from "@gpu-rental/contracts";
import type { HydratedDocument } from "mongoose";

@Schema({ collection: "gpu_resources", timestamps: true, versionKey: false })
export class GpuResource {
  @Prop({ type: String, required: true, trim: true, unique: true })
  name!: string;

  @Prop({ type: String, required: true, trim: true, index: true })
  model!: string;

  @Prop({ type: Number, required: true, min: 1, index: true })
  memoryGb!: number;

  @Prop({ type: Number, required: true, min: 1, default: 1 })
  gpuCount!: number;

  @Prop({ type: Number, required: true, min: 1, default: 16 })
  cpuCores!: number;

  @Prop({ type: Number, required: true, min: 1, default: 64 })
  systemMemoryGb!: number;

  @Prop({ type: Number, required: true, min: 1, default: 100 })
  storageGb!: number;

  @Prop({ type: String, required: true, trim: true, default: "12.4" })
  cudaVersion!: string;

  @Prop({ type: String, required: true, trim: true, default: "550" })
  driverVersion!: string;

  @Prop({ type: Number, required: true, min: 1, default: 1000 })
  bandwidthMbps!: number;

  @Prop({ type: Number, required: true, min: 0, max: 100, default: 99.9 })
  reliabilityPercent!: number;

  @Prop({ type: String, required: true, trim: true, index: true })
  region!: string;

  @Prop({ type: Number, required: true, min: 0, index: true })
  hourlyPriceCents!: number;

  @Prop({ type: [String], default: [] })
  tags!: string[];

  @Prop({
    type: String,
    enum: ResourceMode,
    default: ResourceMode.Simulated,
    immutable: true,
    required: true,
  })
  resourceMode!: ResourceMode;

  @Prop({
    type: String,
    enum: GpuListingStatus,
    default: GpuListingStatus.Offline,
    required: true,
    index: true,
  })
  listingStatus!: GpuListingStatus;

  createdAt!: Date;
  updatedAt!: Date;
}

export type GpuResourceDocument = HydratedDocument<GpuResource>;
export const GpuResourceSchema = SchemaFactory.createForClass(GpuResource);
