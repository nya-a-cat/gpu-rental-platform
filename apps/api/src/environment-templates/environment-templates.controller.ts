import { Controller, Get, Param } from "@nestjs/common";
import { ApiOperation, ApiTags } from "@nestjs/swagger";
import type { EnvironmentTemplateView } from "@gpu-rental/contracts";

import { EnvironmentTemplatesService } from "./environment-templates.service";

@ApiTags("environment-templates")
@Controller("environment-templates")
export class EnvironmentTemplatesController {
  constructor(private readonly templates: EnvironmentTemplatesService) {}

  @Get()
  @ApiOperation({ summary: "List curated workload environment templates" })
  list(): EnvironmentTemplateView[] {
    return this.templates.list();
  }

  @Get(":id")
  getById(@Param("id") id: string): EnvironmentTemplateView {
    return this.templates.getById(id);
  }
}
