import {
  ArgumentsHost,
  Catch,
  HttpException,
  HttpStatus,
  Logger,
  type ExceptionFilter,
} from "@nestjs/common";
import type { ApiErrorResponse } from "@gpu-rental/contracts";
import type { Request, Response } from "express";

import type { RequestWithId } from "./request-id.middleware";

interface ErrorBody {
  code?: unknown;
  message?: unknown;
}

const DEFAULT_CODES: Record<number, string> = {
  [HttpStatus.BAD_REQUEST]: "BAD_REQUEST",
  [HttpStatus.UNAUTHORIZED]: "UNAUTHORIZED",
  [HttpStatus.FORBIDDEN]: "FORBIDDEN",
  [HttpStatus.NOT_FOUND]: "NOT_FOUND",
  [HttpStatus.CONFLICT]: "CONFLICT",
  [HttpStatus.SERVICE_UNAVAILABLE]: "SERVICE_UNAVAILABLE",
};

@Catch()
export class HttpExceptionFilter implements ExceptionFilter {
  private readonly logger = new Logger(HttpExceptionFilter.name);

  catch(exception: unknown, host: ArgumentsHost): void {
    const context = host.switchToHttp();
    const request = context.getRequest<RequestWithId>();
    const response = context.getResponse<Response>();
    const status =
      exception instanceof HttpException
        ? exception.getStatus()
        : HttpStatus.INTERNAL_SERVER_ERROR;
    const body =
      exception instanceof HttpException
        ? (exception.getResponse() as string | ErrorBody)
        : undefined;

    if (!(exception instanceof HttpException)) {
      this.logger.error(
        `Unhandled request error ${request.method} ${request.originalUrl}`,
        exception instanceof Error ? exception.stack : String(exception),
      );
    }

    const payload: ApiErrorResponse = {
      code: this.resolveCode(body, status),
      message: this.resolveMessage(body, status),
      requestId: request.requestId,
    };
    response.status(status).json(payload);
  }

  private resolveCode(
    body: string | ErrorBody | undefined,
    status: number,
  ): string {
    if (typeof body === "object" && typeof body.code === "string") {
      return body.code;
    }
    return DEFAULT_CODES[status] ?? "INTERNAL_ERROR";
  }

  private resolveMessage(
    body: string | ErrorBody | undefined,
    status: number,
  ): string {
    if (typeof body === "string") {
      return body;
    }
    if (body && Array.isArray(body.message)) {
      return body.message
        .filter((item): item is string => typeof item === "string")
        .join("; ");
    }
    if (body && typeof body.message === "string") {
      return body.message;
    }
    return status === HttpStatus.INTERNAL_SERVER_ERROR
      ? "Internal server error"
      : "Request failed";
  }
}
