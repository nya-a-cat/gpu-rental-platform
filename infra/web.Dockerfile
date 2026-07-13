# syntax=docker/dockerfile:1.7

FROM node:24-bookworm-slim AS base
ENV PNPM_HOME=/pnpm
ENV PATH=$PNPM_HOME:$PATH
WORKDIR /workspace
RUN corepack enable

FROM base AS dependencies
COPY package.json pnpm-lock.yaml pnpm-workspace.yaml .npmrc ./
COPY apps/api/package.json apps/api/package.json
COPY apps/web/package.json apps/web/package.json
COPY packages/contracts/package.json packages/contracts/package.json
RUN --mount=type=cache,id=pnpm,target=/pnpm/store \
    pnpm install --frozen-lockfile

FROM dependencies AS build
ARG VITE_RUNTIME_MODE=api
ARG VITE_API_BASE_URL=/api
ARG VITE_BASE_PATH=/
ENV VITE_RUNTIME_MODE=$VITE_RUNTIME_MODE
ENV VITE_API_BASE_URL=$VITE_API_BASE_URL
ENV VITE_BASE_PATH=$VITE_BASE_PATH
COPY tsconfig.base.json ./
COPY apps/web apps/web
COPY packages/contracts packages/contracts
RUN pnpm --filter @gpu-rental/web... run build

FROM nginx:1.31-alpine AS runtime
COPY infra/nginx.conf /etc/nginx/conf.d/default.conf
COPY --from=build /workspace/apps/web/dist /usr/share/nginx/html
EXPOSE 80
