# Multi-stage build. The builder compiles a static binary; the runtime stage is
# distroless, holding only the binary, a non-root user, and CA roots. Base images
# are pinned by digest for reproducible builds.

# golang:1.24-bookworm
FROM golang@sha256:1a6d4452c65dea36aac2e2d606b01b4a029ec90cc1ae53890540ce6173ea77ac AS builder

ARG VERSION=dev
ARG COMMIT=none
ARG TARGETOS
ARG TARGETARCH

WORKDIR /src

# Resolve dependencies first so the layer caches across source-only changes.
COPY go.mod go.sum ./
RUN go mod download

COPY . .

# CGO is off so the binary is fully static and runs on scratch/distroless.
# -trimpath drops local paths; the ldflags strip symbols and stamp the build.
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -trimpath \
    -ldflags "-s -w -X main.version=${VERSION} -X main.commit=${COMMIT}" \
    -o /out/memcored ./cmd/memcored

# gcr.io/distroless/static-debian12:nonroot
FROM gcr.io/distroless/static-debian12@sha256:d093aa3e30dbadd3efe1310db061a14da60299baff8450a17fe0ccc514a16639

COPY --from=builder /out/memcored /memcored

# Data directory for the snapshot and append log. Declared a volume and owned by
# the distroless nonroot user (uid 65532) so the container runs read-only except
# for this path and tmp.
COPY --from=builder --chown=65532:65532 /tmp /data
VOLUME ["/data"]

EXPOSE 6380

USER nonroot:nonroot

# A real PING over the socket proves the server is serving RESP, not merely that
# the process is up.
HEALTHCHECK --interval=10s --timeout=3s --start-period=2s --retries=3 \
    CMD ["/memcored", "-healthcheck"]

# exec form so SIGTERM reaches the process directly and graceful shutdown fires.
ENTRYPOINT ["/memcored", "-host", "0.0.0.0", "-persistence", "-data-dir", "/data"]
