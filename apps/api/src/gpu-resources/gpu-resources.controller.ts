import { Controller, Get, Param, Query } from "@nestjs/common";
import { ApiOperation, ApiTags } from "@nestjs/swagger";
import type {
  GpuResourceFacets,
  GpuResourceView,
  PaginatedResponse,
} from "@gpu-rental/contracts";

import { GpuResourceQueryDto } from "./gpu-resources.dto";
import { GpuResourcesService } from "./gpu-resources.service";

@ApiTags("gpu-resources")
@Controller("gpu-resources")
export class GpuResourcesController {
  constructor(private readonly resources: GpuResourcesService) {}

  @Get()
  @ApiOperation({ summary: "List online GPU resources" })
  list(
    @Query() query: GpuResourceQueryDto,
  ): Promise<PaginatedResponse<GpuResourceView>> {
    return this.resources.listPublic(query);
  }

  @Get("facets")
  getFacets(): Promise<GpuResourceFacets> {
    return this.resources.getFacets();
  }

  @Get(":id")
  getById(@Param("id") id: string): Promise<GpuResourceView> {
    return this.resources.getPublicById(id);
  }
}
