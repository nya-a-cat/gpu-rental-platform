import { Prop, Schema, SchemaFactory } from "@nestjs/mongoose";
import { TeamRole } from "@gpu-rental/contracts";
import { Types, type HydratedDocument } from "mongoose";

@Schema({ _id: false })
export class TeamMemberRecord {
  @Prop({ type: Types.ObjectId, ref: "User", required: true })
  userId!: Types.ObjectId;

  @Prop({ type: String, required: true })
  username!: string;

  @Prop({ type: String, enum: TeamRole, required: true })
  role!: TeamRole;

  @Prop({ type: Date, required: true })
  joinedAt!: Date;
}

const TeamMemberRecordSchema = SchemaFactory.createForClass(TeamMemberRecord);

@Schema({ _id: false })
export class ProjectRecord {
  @Prop({ type: String, required: true })
  id!: string;

  @Prop({ type: String, required: true })
  name!: string;

  @Prop({ type: Number, required: true, min: 0 })
  monthlyBudgetCents!: number;

  @Prop({ type: Number, required: true, min: 0, default: 0 })
  bookedCostCents!: number;

  @Prop({ type: Date, required: true })
  createdAt!: Date;
}

const ProjectRecordSchema = SchemaFactory.createForClass(ProjectRecord);

@Schema({ collection: "teams", timestamps: true, versionKey: false })
export class Team {
  @Prop({ type: String, required: true, trim: true })
  name!: string;

  @Prop({ type: Types.ObjectId, ref: "User", required: true, index: true })
  ownerId!: Types.ObjectId;

  @Prop({ type: [TeamMemberRecordSchema], default: [] })
  members!: TeamMemberRecord[];

  @Prop({ type: [ProjectRecordSchema], default: [] })
  projects!: ProjectRecord[];

  createdAt!: Date;
  updatedAt!: Date;
}

export type TeamDocument = HydratedDocument<Team>;
export const TeamSchema = SchemaFactory.createForClass(Team);
TeamSchema.index({ "members.userId": 1, createdAt: -1 });
TeamSchema.index({ ownerId: 1, name: 1 }, { unique: true });
