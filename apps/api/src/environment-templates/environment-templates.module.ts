import { Module } from "@nestjs/common";

import { EnvironmentTemplatesController } from "./environment-templates.controller";
import { EnvironmentTemplatesService } from "./environment-templates.service";

@Module({
  controllers: [EnvironmentTemplatesController],
  providers: [EnvironmentTemplatesService],
  exports: [EnvironmentTemplatesService],
})
export class EnvironmentTemplatesModule {}
