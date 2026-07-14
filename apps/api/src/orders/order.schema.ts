import { Prop, Schema, SchemaFactory } from "@nestjs/mongoose";
import { OrderStatus } from "@gpu-rental/contracts";
import { Types, type HydratedDocument } from "mongoose";

@Schema({ collection: "orders", timestamps: true, versionKey: false })
export class Order {
  @Prop({ type: Types.ObjectId, ref: "User", required: true, index: true })
  userId!: Types.ObjectId;

  @Prop({ type: Types.ObjectId, ref: "GpuResource", required: true })
  gpuResourceId!: Types.ObjectId;

  @Prop({ type: String, required: true })
  gpuName!: string;

  @Prop({ type: String, required: true })
  gpuModel!: string;

  @Prop({ type: Number, required: true, min: 1 })
  gpuMemoryGb!: number;

  @Prop({ type: Number, required: true, min: 1, default: 1 })
  gpuCount!: number;

  @Prop({ type: Number, required: true, min: 1, default: 100 })
  temporaryStorageGb!: number;

  @Prop({ type: String, required: true, default: "pytorch-jupyter" })
  environmentTemplateId!: string;

  @Prop({ type: String, required: true, default: "PyTorch + JupyterLab" })
  environmentTemplateName!: string;

  @Prop({ type: String, required: true, trim: true, default: "GPU workload" })
  instanceName!: string;

  @Prop({ type: String, default: null })
  projectId!: string | null;

  @Prop({ type: String, default: null })
  projectName!: string | null;

  @Prop({ type: String, default: null })
  teamName!: string | null;

  @Prop({ type: String, required: true })
  region!: string;

  @Prop({ type: Number, required: true, min: 0 })
  hourlyPriceCents!: number;

  @Prop({ type: Number, required: true, min: 1 })
  durationHours!: number;

  @Prop({ type: Number, required: true, min: 0 })
  totalPriceCents!: number;

  @Prop({
    type: String,
    enum: OrderStatus,
    default: OrderStatus.Active,
    required: true,
  })
  status!: OrderStatus;

  @Prop({ type: Date, required: true })
  startsAt!: Date;

  @Prop({ type: Date, required: true, index: true })
  endsAt!: Date;

  @Prop({ type: Date, default: null })
  returnedAt!: Date | null;

  @Prop({ type: Date, default: null })
  cancelledAt!: Date | null;

  createdAt!: Date;
  updatedAt!: Date;
}

export type OrderDocument = HydratedDocument<Order>;
export const OrderSchema = SchemaFactory.createForClass(Order);

// 部分唯一索引是并发预订的最终防线，仅限制同一资源的有效订单。
OrderSchema.index(
  { gpuResourceId: 1 },
  {
    unique: true,
    partialFilterExpression: { status: OrderStatus.Active },
    name: "one_active_order_per_gpu",
  },
);
OrderSchema.index({ userId: 1, createdAt: -1 });
OrderSchema.index({ status: 1, createdAt: -1 });
