import { InstanceStatus } from "@gpu-rental/contracts";
import { IsEnum, IsOptional } from "class-validator";

import { PaginationDto } from "../common/pagination.dto";

export class InstanceQueryDto extends PaginationDto {
  @IsOptional()
  @IsEnum(InstanceStatus)
  status?: InstanceStatus;
}
