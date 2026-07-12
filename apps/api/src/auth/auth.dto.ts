import { ApiProperty } from "@nestjs/swagger";
import type { LoginInput, RegisterInput } from "@gpu-rental/contracts";
import { IsString, Length, Matches } from "class-validator";

const USERNAME_PATTERN = /^[A-Za-z0-9_-]+$/;

export class RegisterDto implements RegisterInput {
  @ApiProperty({ example: "gpu_user" })
  @IsString()
  @Length(3, 32)
  @Matches(USERNAME_PATTERN)
  username!: string;

  @ApiProperty({ minLength: 8, maxLength: 72, writeOnly: true })
  @IsString()
  @Length(8, 72)
  password!: string;
}

export class LoginDto implements LoginInput {
  @ApiProperty({ example: "gpu_user" })
  @IsString()
  @Length(3, 32)
  username!: string;

  @ApiProperty({ writeOnly: true })
  @IsString()
  @Length(1, 72)
  password!: string;
}
