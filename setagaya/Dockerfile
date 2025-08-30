# Multi-stage build for better security and smaller image size
FROM gcr.io/distroless/base-debian11:nonroot

# Copy ca-certificates are already included in distroless/base
ARG binary_name=setagaya
COPY ./build/${binary_name} /usr/local/bin/${binary_name}

ENV GOOGLE_APPLICATION_CREDENTIALS /auth/setagaya-gcp.json

ARG env=local
ENV env ${env}

COPY ./ui/ /
ENV binary=${binary_name}

# Use nonroot user for better security (UID 65532)
USER nonroot:nonroot

# Use exec form for better signal handling
CMD ["/usr/local/bin/setagaya"]
