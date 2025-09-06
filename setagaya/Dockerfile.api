# Build stage
FROM golang:1.25.1-alpine AS builder

# Install build dependencies and security updates
RUN apk update && apk upgrade && \
    apk add --no-cache git ca-certificates tzdata && \
    addgroup -g 1001 setagaya && \
    adduser -D -u 1001 -G setagaya setagaya

# Set working directory
WORKDIR /build

# Copy go mod files first for better layer caching
COPY go.mod go.sum ./
RUN go mod download && go mod verify

# Copy source code
COPY . .

# Build the API server binary with security flags and optimizations
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags='-w -s -extldflags "-static"' \
    -a -installsuffix cgo \
    -o setagaya \
    ./main.go

# Runtime stage
FROM alpine:3.20

# Install security updates and create non-root user
RUN apk update && apk upgrade && \
    apk add --no-cache ca-certificates tzdata && \
    addgroup -g 1001 setagaya && \
    adduser -D -u 1001 -G setagaya setagaya

# Environment variables
ENV GOOGLE_APPLICATION_CREDENTIALS=/auth/setagaya-gcp.json
ENV env=local

# Copy the binary from builder stage
COPY --from=builder --chmod=755 /build/setagaya /usr/local/bin/setagaya

# Copy UI files with proper ownership
COPY --chmod=644 --chown=setagaya:setagaya ./ui/ /

# Switch to non-root user
USER setagaya:setagaya

# Set working directory
WORKDIR /home/setagaya

# Expose application port
EXPOSE 8080

# Run the application
ENTRYPOINT ["/usr/local/bin/setagaya"]
