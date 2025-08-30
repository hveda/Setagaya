# Multi-stage build for secure ingress controller
FROM golang:1.21-alpine AS builder

WORKDIR /go/src/setagaya-ingress-controller

ENV GO111MODULE=on
COPY go.mod go.sum ./
RUN go mod download

COPY . /go/src/setagaya-ingress-controller

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-w -s -extldflags=-static" -o /go/bin/sic

# Use distroless for production
FROM gcr.io/distroless/static-debian11:nonroot

# Copy the statically compiled binary
COPY --from=builder /go/bin/sic /usr/local/bin/sic

# Use nonroot user for security
USER nonroot:nonroot

# Use exec form for better signal handling
CMD ["/usr/local/bin/sic"]
