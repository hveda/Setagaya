# Multi-stage build for secure and minimal storage service
FROM golang:1.21-alpine AS builder

WORKDIR /go/src/storage
COPY go.mod go.sum ./
RUN go mod download

COPY main.go ./
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-w -s -extldflags=-static" -o storage main.go

# Use distroless for production
FROM gcr.io/distroless/static-debian11:nonroot

# Copy the statically compiled binary
COPY --from=builder /go/src/storage/storage /usr/local/bin/storage

# Use nonroot user for security
USER nonroot:nonroot

# Use exec form for better signal handling
CMD ["/usr/local/bin/storage"]
