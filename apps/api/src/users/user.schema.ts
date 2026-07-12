import { Prop, Schema, SchemaFactory } from "@nestjs/mongoose";
import { UserRole } from "@gpu-rental/contracts";
import type { HydratedDocument } from "mongoose";

@Schema({ collection: "users", timestamps: true, versionKey: false })
export class User {
  @Prop({
    type: String,
    required: true,
    trim: true,
    lowercase: true,
    unique: true,
  })
  username!: string;

  @Prop({ type: String, required: true, select: false })
  passwordHash!: string;

  @Prop({
    type: String,
    enum: UserRole,
    default: UserRole.User,
    required: true,
  })
  role!: UserRole;

  createdAt!: Date;
  updatedAt!: Date;
}

export type UserDocument = HydratedDocument<User>;
export const UserSchema = SchemaFactory.createForClass(User);
