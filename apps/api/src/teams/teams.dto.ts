import { ApiProperty } from "@nestjs/swagger";
import {
  TeamRole,
  type AddTeamMemberInput,
  type CreateProjectInput,
  type CreateTeamInput,
} from "@gpu-rental/contracts";
import { Type } from "class-transformer";
import { IsIn, IsInt, IsString, Length, Max, Min } from "class-validator";

export class CreateTeamDto implements CreateTeamInput {
  @ApiProperty({ example: "Research lab" })
  @IsString()
  @Length(2, 80)
  name!: string;
}

export class AddTeamMemberDto implements AddTeamMemberInput {
  @ApiProperty({ example: "operator" })
  @IsString()
  @Length(3, 32)
  username!: string;

  @ApiProperty({ enum: [TeamRole.Admin, TeamRole.Member] })
  @IsIn([TeamRole.Admin, TeamRole.Member])
  role!: TeamRole.Admin | TeamRole.Member;
}

export class CreateProjectDto implements CreateProjectInput {
  @ApiProperty({ example: "LLM training" })
  @IsString()
  @Length(2, 80)
  name!: string;

  @ApiProperty({ example: 200_000 })
  @Type(() => Number)
  @IsInt()
  @Min(0)
  @Max(100_000_000)
  monthlyBudgetCents!: number;
}
