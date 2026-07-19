# syntax=docker/dockerfile:1.7

FROM golang:1.26.5-bookworm AS build
WORKDIR /workspace/apps/gpu-platform-addon

COPY apps/gpu-platform-addon/go.mod apps/gpu-platform-addon/go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

COPY apps/gpu-platform-addon ./
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/gpu-platform-addon ./cmd/gpu-platform-addon

FROM gcr.io/distroless/static-debian12:nonroot AS runtime
COPY --from=build /out/gpu-platform-addon /usr/local/bin/gpu-platform-addon
USER 65532:65532
ENTRYPOINT ["/usr/local/bin/gpu-platform-addon"]
