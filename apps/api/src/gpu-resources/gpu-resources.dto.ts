import {
  ApiProperty,
  ApiPropertyOptional,
  OmitType,
  PartialType,
} from "@nestjs/swagger";
import {
  GpuAvailability,
  GpuListingStatus,
  type CreateGpuResourceInput,
  type SetGpuListingStatusInput,
  type UpdateGpuResourceInput,
} from "@gpu-rental/contracts";
import { Type } from "class-transformer";
import {
  IsArray,
  IsNumber,
  IsEnum,
  IsInt,
  IsOptional,
  IsString,
  Length,
  Max,
  Min,
} from "class-validator";

import { PaginationDto } from "../common/pagination.dto";

export enum GpuSort {
  Newest = "newest",
  PriceAsc = "priceAsc",
  PriceDesc = "priceDesc",
}

export class GpuResourceQueryDto extends PaginationDto {
  @IsOptional()
  @IsString()
  model?: string;

  @IsOptional()
  @IsString()
  region?: string;

  @IsOptional()
  @Type(() => Number)
  @IsInt()
  @Min(1)
  memoryGb?: number;

  @IsOptional()
  @Type(() => Number)
  @IsInt()
  @Min(0)
  maxHourlyPriceCents?: number;

  @IsOptional()
  @IsEnum(GpuAvailability)
  availability?: GpuAvailability;

  @IsOptional()
  @IsEnum(GpuSort)
  sort: GpuSort = GpuSort.Newest;
}

export class AdminGpuResourceQueryDto extends GpuResourceQueryDto {
  @IsOptional()
  @IsEnum(GpuListingStatus)
  listingStatus?: GpuListingStatus;
}

export class CreateGpuResourceDto implements CreateGpuResourceInput {
  @ApiProperty({ example: "cn-east-a100-01" })
  @IsString()
  @Length(2, 80)
  name!: string;

  @ApiProperty({ example: "NVIDIA A100" })
  @IsString()
  @Length(2, 80)
  model!: string;

  @ApiProperty({ example: 80 })
  @IsInt()
  @Min(1)
  @Max(1024)
  memoryGb!: number;

  @ApiPropertyOptional({ example: 1 })
  @IsOptional()
  @IsInt()
  @Min(1)
  @Max(8)
  gpuCount?: number;

  @ApiPropertyOptional({ example: 16 })
  @IsOptional()
  @IsInt()
  @Min(1)
  @Max(512)
  cpuCores?: number;

  @ApiPropertyOptional({ example: 64 })
  @IsOptional()
  @IsInt()
  @Min(1)
  @Max(4096)
  systemMemoryGb?: number;

  @ApiPropertyOptional({ example: 100 })
  @IsOptional()
  @IsInt()
  @Min(1)
  @Max(16384)
  storageGb?: number;

  @ApiPropertyOptional({ example: "12.4" })
  @IsOptional()
  @IsString()
  @Length(1, 20)
  cudaVersion?: string;

  @ApiPropertyOptional({ example: "550.54" })
  @IsOptional()
  @IsString()
  @Length(1, 30)
  driverVersion?: string;

  @ApiPropertyOptional({ example: 1000 })
  @IsOptional()
  @IsInt()
  @Min(1)
  @Max(1_000_000)
  bandwidthMbps?: number;

  @ApiPropertyOptional({ example: 99.9 })
  @IsOptional()
  @Type(() => Number)
  @IsNumber({ maxDecimalPlaces: 3 })
  @Min(0)
  @Max(100)
  reliabilityPercent?: number;

  @ApiProperty({ example: "cn-east" })
  @IsString()
  @Length(2, 80)
  region!: string;

  @ApiProperty({ example: 2290, description: "Integer cents per hour" })
  @IsInt()
  @Min(0)
  hourlyPriceCents!: number;

  @ApiPropertyOptional({ type: [String] })
  @IsOptional()
  @IsArray()
  @IsString({ each: true })
  tags?: string[];

  @ApiPropertyOptional({ enum: GpuListingStatus })
  @IsOptional()
  @IsEnum(GpuListingStatus)
  listingStatus?: GpuListingStatus;
}

export class UpdateGpuResourceDto
  extends PartialType(
    OmitType(CreateGpuResourceDto, ["listingStatus"] as const),
  )
  implements UpdateGpuResourceInput {}

export class SetGpuListingStatusDto implements SetGpuListingStatusInput {
  @ApiProperty({ enum: GpuListingStatus })
  @IsEnum(GpuListingStatus)
  listingStatus!: GpuListingStatus;
}
