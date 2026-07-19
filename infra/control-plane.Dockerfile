# syntax=docker/dockerfile:1.7

FROM golang:1.26.5-bookworm AS build
WORKDIR /workspace/apps/control-plane

COPY apps/control-plane/go.mod apps/control-plane/go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

COPY apps/control-plane ./
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/control-plane ./cmd/control-plane
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/control-plane-migrate ./cmd/migrate
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/control-plane-healthcheck ./cmd/healthcheck
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/control-plane-audit-archive ./cmd/audit-archive

FROM gcr.io/distroless/static-debian12:nonroot AS runtime
COPY --from=build /out/control-plane /usr/local/bin/control-plane
COPY --from=build /out/control-plane-migrate /usr/local/bin/control-plane-migrate
COPY --from=build /out/control-plane-healthcheck /usr/local/bin/control-plane-healthcheck
COPY --from=build /out/control-plane-audit-archive /usr/local/bin/control-plane-audit-archive
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/control-plane"]
