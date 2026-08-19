# Builds any one of Honryu's own control-plane binaries (api, calibrator,
# scheduler, sidecar) from a single Dockerfile, selected via
# --build-arg CMD=<name>. They share one build stage because they share one
# module and one dependency set -- four near-identical Dockerfiles would
# only drift.
#
# Runtime base is alpine, not distroless: the k8s scheduler adapter's own
# sidecar container Command is ["/bin/sh", "-c", "/honryu-sidecar ..."]
# (internal/adapters/scheduler/k8s/k8s.go's sidecarScript) -- a shell-less
# distroless image would fail to even start that container. The binary is
# named /honryu-${CMD} to match that same hardcoded /honryu-sidecar path.
# Pinned by digest as well as tag: a tag is a moving pointer, so "golang:1.26.6"
# alone does not make this build reproducible, and a compromised or simply
# re-pushed tag would silently change the compiler that produces the binaries.
# Digest resolved with `docker buildx imagetools inspect` at pin time.
FROM golang:1.26.6@sha256:0d1d3a794be25f809dd2cb3160d8c73276c4056a9f8242a138e908ddeee7b6b6 AS build
ARG CMD=api
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -o /out/honryu-${CMD} ./cmd/${CMD}

FROM alpine:3.20@sha256:d9e853e87e55526f6b2917df91a2115c36dd7c696a35be12163d44e6e2a4b6bc
ARG CMD=api
# ARGs are build-time only; ENTRYPOINT's shell form expands $CMD at
# container *runtime*, so it must be re-exposed as an ENV to survive into
# the running container (exec-form ENTRYPOINT's JSON array, tried first,
# does not expand ARG/ENV at all -- verified, it shipped the literal
# "${CMD}" string).
ENV CMD=${CMD}
COPY --from=build /out/honryu-${CMD} /honryu-${CMD}
ENTRYPOINT /honryu-${CMD}
