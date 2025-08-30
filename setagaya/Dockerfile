# Multi-stage build for better security and smaller image size
# Stage 1: UI Build Environment
FROM node:18-alpine AS ui-builder

# Set working directory for UI build
WORKDIR /app

# Copy package files for dependency installation
COPY package.json ./
COPY src/ ./src/
COPY tailwind.config.js ./
COPY webpack.config.js ./

# Install npm dependencies (including devDependencies for build tools)
RUN npm install

# Copy UI source files
COPY ui/ ./ui/

# Build production UI assets
RUN npm run build-css-prod
RUN npm run build-js 2>/dev/null || echo "Webpack build skipped - no JS entry point"

# Stage 2: Runtime Environment
FROM gcr.io/distroless/base-debian11:nonroot

# Copy ca-certificates are already included in distroless/base
ARG binary_name=setagaya
COPY ./build/${binary_name} /usr/local/bin/${binary_name}

ENV GOOGLE_APPLICATION_CREDENTIALS /auth/setagaya-gcp.json

ARG env=local
ENV env ${env}

# Copy UI templates and static files from build stage
COPY --from=ui-builder /app/ui/templates/ /templates/
COPY --from=ui-builder /app/ui/static/ /static/
# Copy built CSS assets
COPY --from=ui-builder /app/ui/static/css/output.css /static/css/output.css

ENV binary=${binary_name}

# Use nonroot user for better security (UID 65532)
USER nonroot:nonroot

# Use exec form for better signal handling
CMD ["/usr/local/bin/setagaya"]
