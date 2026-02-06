# Stage 1: Build the operator binary
FROM golang:1.21-alpine AS builder

RUN apk add --no-cache git ca-certificates

WORKDIR /workspace

# Copy dependency manifests first for better layer caching.
COPY go.mod go.sum ./
RUN go mod download

# Copy source code.
COPY . .

# Build a statically-linked binary with CGO disabled.
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags="-w -s" \
    -o /workspace/manager \
    main.go

# Stage 2: Minimal runtime image
FROM gcr.io/distroless/static:nonroot

WORKDIR /

COPY --from=builder /workspace/manager /manager

# Run as nonroot user (65532 is the nonroot user in distroless images).
USER 65532:65532

ENTRYPOINT ["/manager"]
