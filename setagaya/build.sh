#!/bin/bash

target=$1
mkdir -p build
export GO111MODULE=on
go mod download

# Build flags for static compilation and security
BUILD_FLAGS="-ldflags=-w -s -extldflags=-static"
export CGO_ENABLED=0

case "$target" in
    "jmeter") GOOS=linux GOARCH=amd64 go build -ldflags="-w -s -extldflags=-static"  -a -installsuffix cgo -o build/setagaya-agent $(pwd)/engines/jmeter
    ;;
    "controller") GOOS=linux GOARCH=amd64 go build -ldflags="-w -s -extldflags=-static" -a -installsuffix cgo -o build/setagaya-controller $(pwd)/controller/cmd
    ;;
    *)
    GOOS=linux GOARCH=amd64 go build -ldflags="-w -s -extldflags=-static" -a -installsuffix cgo -o build/setagaya
esac