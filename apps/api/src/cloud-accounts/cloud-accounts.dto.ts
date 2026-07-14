import { ApiProperty } from "@nestjs/swagger";
import {
  NetworkProtocol,
  type AttachVolumeInput,
  type CreateApiKeyInput,
  type CreateNetworkRuleInput,
  type CreateSnapshotInput,
  type CreateSshKeyInput,
  type CreateVolumeInput,
  type TopUpInput,
} from "@gpu-rental/contracts";
import { Type } from "class-transformer";
import {
  IsEnum,
  IsInt,
  IsMongoId,
  IsString,
  Length,
  Matches,
  Max,
  Min,
} from "class-validator";

export class TopUpDto implements TopUpInput {
  @ApiProperty({ example: 10_000, description: "Integer cents" })
  @Type(() => Number)
  @IsInt()
  @Min(100)
  @Max(10_000_000)
  amountCents!: number;
}

export class CreateSshKeyDto implements CreateSshKeyInput {
  @ApiProperty({ example: "laptop" })
  @IsString()
  @Length(2, 80)
  name!: string;

  @ApiProperty({ example: "ssh-ed25519 AAAA... operator@example" })
  @IsString()
  @Length(20, 4096)
  @Matches(/^ssh-(ed25519|rsa|ecdsa)\s+\S+(?:\s+.*)?$/)
  publicKey!: string;
}

export class CreateApiKeyDto implements CreateApiKeyInput {
  @ApiProperty({ example: "training automation" })
  @IsString()
  @Length(2, 80)
  name!: string;
}

export class CreateNetworkRuleDto implements CreateNetworkRuleInput {
  @ApiProperty()
  @IsMongoId()
  instanceId!: string;

  @ApiProperty({ example: "tensorboard" })
  @IsString()
  @Length(2, 80)
  name!: string;

  @ApiProperty({ enum: NetworkProtocol })
  @IsEnum(NetworkProtocol)
  protocol!: NetworkProtocol;

  @ApiProperty({ example: 6006 })
  @Type(() => Number)
  @IsInt()
  @Min(1)
  @Max(65535)
  port!: number;

  @ApiProperty({ example: "0.0.0.0/0" })
  @IsString()
  @Matches(/^(?:\d{1,3}\.){3}\d{1,3}\/(?:[0-9]|[12][0-9]|3[0-2])$/)
  sourceCidr!: string;
}

export class CreateVolumeDto implements CreateVolumeInput {
  @ApiProperty({ example: "model-cache" })
  @IsString()
  @Length(2, 80)
  name!: string;

  @ApiProperty({ example: 100 })
  @Type(() => Number)
  @IsInt()
  @Min(10)
  @Max(10_240)
  sizeGb!: number;
}

export class AttachVolumeDto implements AttachVolumeInput {
  @ApiProperty()
  @IsMongoId()
  instanceId!: string;
}

export class CreateSnapshotDto implements CreateSnapshotInput {
  @ApiProperty({ example: "checkpoint-01" })
  @IsString()
  @Length(2, 80)
  name!: string;
}
