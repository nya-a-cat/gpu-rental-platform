import { ApiProperty } from "@nestjs/swagger";
import { OrderStatus, type CreateOrderInput } from "@gpu-rental/contracts";
import { Type } from "class-transformer";
import {
  IsEnum,
  IsInt,
  IsMongoId,
  IsOptional,
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
