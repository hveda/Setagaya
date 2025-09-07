# Build stage
FROM golang:1.25.1-alpine3.22@sha256:546671046d6f9f786c24b83111ba1801b736ee4b01b23db33e4f3eb41d4f8883 AS builder

# Install security updates and required packages
RUN apk update && apk upgrade && apk add --no-cache git ca-certificates tzdata && \
    addgroup -g 1001 setagaya && \
    adduser -D -u 1001 -G setagaya setagaya

# Set working directory
WORKDIR /build

# Enable Go modules
ENV GO111MODULE=on

# Copy go mod files first for better caching
COPY go.mod go.sum ./
RUN go mod download && go mod verify

# Copy source code
COPY . .

# Build the binary with security flags
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags='-w -s -extldflags "-static"' \
    -a -installsuffix cgo \
    -o sic .

# Final stage
FROM scratch

# Import from builder
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder /usr/share/zoneinfo /usr/share/zoneinfo
COPY --from=builder /etc/passwd /etc/passwd

# Copy the binary
COPY --from=builder /build/sic /sic

# Use non-root user
USER setagaya

# Run the binary
ENTRYPOINT ["/sic"]
