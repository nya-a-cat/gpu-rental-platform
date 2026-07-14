import { Module } from "@nestjs/common";
import { MongooseModule } from "@nestjs/mongoose";

import { AuthModule } from "../auth/auth.module";
import { Instance, InstanceSchema } from "../instances/instance.schema";
import { CloudAccount, CloudAccountSchema } from "./cloud-account.schema";
import { CloudAccountsController } from "./cloud-accounts.controller";
import { CloudAccountsService } from "./cloud-accounts.service";

@Module({
  imports: [
    AuthModule,
    MongooseModule.forFeature([
      { name: CloudAccount.name, schema: CloudAccountSchema },
      { name: Instance.name, schema: InstanceSchema },
    ]),
  ],
  controllers: [CloudAccountsController],
  providers: [CloudAccountsService],
  exports: [CloudAccountsService],
})
export class CloudAccountsModule {}
