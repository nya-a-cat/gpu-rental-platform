import { Prop, Schema, SchemaFactory } from "@nestjs/mongoose";
import { ConnectionMode, InstanceStatus } from "@gpu-rental/contracts";
import { Types, type HydratedDocument } from "mongoose";

@Schema({ collection: "instances", timestamps: true, versionKey: false })
export class Instance {
  @Prop({ type: Types.ObjectId, ref: "Order", required: true, unique: true })
  orderId!: Types.ObjectId;

  @Prop({ type: Types.ObjectId, ref: "User", required: true, index: true })
  userId!: Types.ObjectId;

  @Prop({ type: Types.ObjectId, ref: "GpuResource", required: true })
  gpuResourceId!: Types.ObjectId;

  @Prop({ type: String, required: true, trim: true })
  name!: string;

  @Prop({ type: String, required: true })
  gpuName!: string;

  @Prop({ type: String, required: true })
  gpuModel!: string;

  @Prop({ type: Number, required: true, min: 1 })
  gpuCount!: number;

  @Prop({ type: Number, required: true, min: 1 })
  gpuMemoryGb!: number;

  @Prop({ type: String, required: true })
  environmentTemplateId!: string;

  @Prop({ type: String, required: true })
  environmentTemplateName!: string;

  @Prop({ type: String, required: true })
  environmentImage!: string;

  @Prop({ type: [String], enum: ConnectionMode, default: [] })
  connectionModes!: ConnectionMode[];

  @Prop({ type: Number, required: true, min: 0 })
  hourlyPriceCents!: number;

  @Prop({ type: Number, required: true, min: 0 })
  maximumCostCents!: number;

  @Prop({ type: String, enum: InstanceStatus, required: true, index: true })
  status!: InstanceStatus;

  @Prop({ type: Boolean, required: true, default: true, immutable: true })
  simulated!: boolean;

  @Prop({ type: Date, required: true })
  startsAt!: Date;

  @Prop({ type: Date, required: true, index: true })
  endsAt!: Date;

  @Prop({ type: Date, default: null })
  runningSince!: Date | null;

  @Prop({ type: Number, required: true, min: 0, default: 0 })
  accumulatedRunSeconds!: number;

  @Prop({ type: Date, default: null })
  stoppedAt!: Date | null;

  @Prop({ type: Date, default: null })
  terminatedAt!: Date | null;

  createdAt!: Date;
  updatedAt!: Date;
}

export type InstanceDocument = HydratedDocument<Instance>;
export const InstanceSchema = SchemaFactory.createForClass(Instance);
InstanceSchema.index({ userId: 1, createdAt: -1 });
InstanceSchema.index({ status: 1, createdAt: -1 });
