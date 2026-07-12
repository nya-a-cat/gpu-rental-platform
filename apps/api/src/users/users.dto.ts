import { ApiProperty } from "@nestjs/swagger";
import type { ChangePasswordInput } from "@gpu-rental/contracts";
import { IsString, Length } from "class-validator";

export class ChangePasswordDto implements ChangePasswordInput {
  @ApiProperty({ writeOnly: true })
  @IsString()
  @Length(1, 72)
  currentPassword!: string;

  @ApiProperty({ minLength: 8, maxLength: 72, writeOnly: true })
  @IsString()
  @Length(8, 72)
  newPassword!: string;
}
