import { ApiProperty } from "@nestjs/swagger";
import { OrderStatus, type CreateOrderInput } from "@gpu-rental/contracts";
import { Type } from "class-transformer";
import {
  IsEnum,
  IsInt,
  IsMongoId,
  IsOptional,
  IsString,
  Length,
  Max,
  Min,
} from "class-validator";

import { PaginationDto } from "../common/pagination.dto";

export class CreateOrderDto implements CreateOrderInput {
  @ApiProperty()
  @IsMongoId()
  gpuResourceId!: string;

  @ApiProperty({ minimum: 1, maximum: 720, example: 8 })
  @Type(() => Number)
  @IsInt()
  @Min(1)
  @Max(720)
  durationHours!: number;

  @ApiProperty({ required: false, example: "pytorch-jupyter" })
  @IsOptional()
  @IsString()
  @Length(2, 80)
  environmentTemplateId?: string;

  @ApiProperty({ required: false, example: "training-run-01" })
  @IsOptional()
  @IsString()
  @Length(2, 80)
  instanceName?: string;
}

export class OrderQueryDto extends PaginationDto {
  @IsOptional()
  @IsEnum(OrderStatus)
  status?: OrderStatus;
}

export class AdminOrderQueryDto extends OrderQueryDto {
  @IsOptional()
  @IsMongoId()
  userId?: string;

  @IsOptional()
  @IsMongoId()
  gpuResourceId?: string;
}
