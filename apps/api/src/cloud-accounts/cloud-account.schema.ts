import { Prop, Schema, SchemaFactory } from "@nestjs/mongoose";
import {
  BillingEntryType,
  NetworkProtocol,
  NotificationType,
  VolumeStatus,
} from "@gpu-rental/contracts";
import { Types, type HydratedDocument } from "mongoose";

@Schema({ _id: false })
export class BillingEntryRecord {
  @Prop({ type: String, required: true })
  id!: string;

  @Prop({ type: String, enum: BillingEntryType, required: true })
  type!: BillingEntryType;

  @Prop({ type: Number, required: true, min: 0 })
  amountCents!: number;

  @Prop({ type: String, required: true })
  reference!: string;

  @Prop({ type: String, required: true })
  description!: string;

  @Prop({ type: Date, required: true })
  createdAt!: Date;
}

const BillingEntryRecordSchema =
  SchemaFactory.createForClass(BillingEntryRecord);

@Schema({ _id: false })
export class SshKeyRecord {
  @Prop({ type: String, required: true })
  id!: string;

  @Prop({ type: String, required: true })
  name!: string;

  @Prop({ type: String, required: true })
  fingerprint!: string;

  @Prop({ type: String, required: true })
  publicKey!: string;

  @Prop({ type: Date, required: true })
  createdAt!: Date;
}

const SshKeyRecordSchema = SchemaFactory.createForClass(SshKeyRecord);

@Schema({ _id: false })
export class ApiKeyRecord {
  @Prop({ type: String, required: true })
  id!: string;

  @Prop({ type: String, required: true })
  name!: string;

  @Prop({ type: String, required: true })
  prefix!: string;

  @Prop({ type: String, required: true })
  tokenHash!: string;

  @Prop({ type: Date, required: true })
  createdAt!: Date;

  @Prop({ type: Date, default: null })
  lastUsedAt!: Date | null;
}

const ApiKeyRecordSchema = SchemaFactory.createForClass(ApiKeyRecord);

@Schema({ _id: false })
export class NetworkRuleRecord {
  @Prop({ type: String, required: true })
  id!: string;

  @Prop({ type: Types.ObjectId, ref: "Instance", required: true })
  instanceId!: Types.ObjectId;

  @Prop({ type: String, required: true })
  name!: string;

  @Prop({ type: String, enum: NetworkProtocol, required: true })
  protocol!: NetworkProtocol;

  @Prop({ type: Number, required: true, min: 1, max: 65535 })
  port!: number;

  @Prop({ type: String, required: true })
  sourceCidr!: string;

  @Prop({ type: Date, required: true })
  createdAt!: Date;
}

const NetworkRuleRecordSchema = SchemaFactory.createForClass(NetworkRuleRecord);

@Schema({ _id: false })
export class SnapshotRecord {
  @Prop({ type: String, required: true })
  id!: string;

  @Prop({ type: String, required: true })
  name!: string;

  @Prop({ type: Number, required: true, min: 1 })
  sizeGb!: number;

  @Prop({ type: Date, required: true })
  createdAt!: Date;
}

const SnapshotRecordSchema = SchemaFactory.createForClass(SnapshotRecord);

@Schema({ _id: false })
export class VolumeRecord {
  @Prop({ type: String, required: true })
  id!: string;

  @Prop({ type: String, required: true })
  name!: string;

  @Prop({ type: Number, required: true, min: 10, max: 10240 })
  sizeGb!: number;

  @Prop({ type: String, enum: VolumeStatus, required: true })
  status!: VolumeStatus;

  @Prop({ type: Types.ObjectId, ref: "Instance", default: null })
  attachedInstanceId!: Types.ObjectId | null;

  @Prop({ type: Number, required: true, min: 0 })
  monthlyPriceCents!: number;

  @Prop({ type: [SnapshotRecordSchema], default: [] })
  snapshots!: SnapshotRecord[];

  @Prop({ type: Date, required: true })
  createdAt!: Date;

  @Prop({ type: Date, required: true })
  updatedAt!: Date;
}

const VolumeRecordSchema = SchemaFactory.createForClass(VolumeRecord);

@Schema({ _id: false })
export class NotificationRecord {
  @Prop({ type: String, required: true })
  id!: string;

  @Prop({ type: String, enum: NotificationType, required: true })
  type!: NotificationType;

  @Prop({ type: String, required: true })
  title!: string;

  @Prop({ type: String, required: true })
  message!: string;

  @Prop({ type: Date, default: null })
  readAt!: Date | null;

  @Prop({ type: Date, required: true })
  createdAt!: Date;
}

const NotificationRecordSchema =
  SchemaFactory.createForClass(NotificationRecord);

@Schema({ collection: "cloud_accounts", timestamps: true, versionKey: false })
export class CloudAccount {
  @Prop({ type: Types.ObjectId, ref: "User", required: true, unique: true })
  userId!: Types.ObjectId;

  @Prop({ type: Number, required: true, min: 0, default: 100_000 })
  balanceCents!: number;

  @Prop({ type: [BillingEntryRecordSchema], default: [] })
  billingEntries!: BillingEntryRecord[];

  @Prop({ type: [SshKeyRecordSchema], default: [] })
  sshKeys!: SshKeyRecord[];

  @Prop({ type: [ApiKeyRecordSchema], default: [] })
  apiKeys!: ApiKeyRecord[];

  @Prop({ type: [NetworkRuleRecordSchema], default: [] })
  networkRules!: NetworkRuleRecord[];

  @Prop({ type: [VolumeRecordSchema], default: [] })
  volumes!: VolumeRecord[];

  @Prop({ type: [NotificationRecordSchema], default: [] })
  notifications!: NotificationRecord[];

  createdAt!: Date;
  updatedAt!: Date;
}

export type CloudAccountDocument = HydratedDocument<CloudAccount>;
export const CloudAccountSchema = SchemaFactory.createForClass(CloudAccount);
