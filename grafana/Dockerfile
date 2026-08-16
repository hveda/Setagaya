# Use latest stable Grafana with security updates (pinned by digest)
FROM grafana/grafana:13.1.3@sha256:ab5cb380e3ff3172d6c8bd2e7cfd31cce977d2881b260e1f5bc089bf0b759b43

# Security environment variables
ENV GF_SECURITY_ADMIN_USER=setagaya
ENV GF_SECURITY_ADMIN_PASSWORD=setagaya
ENV GF_AUTH_ANONYMOUS_ENABLED=true
ENV GF_AUTH_ANONYMOUS_ORG_NAME="Main Org."
ENV GF_AUTH_ANONYMOUS_ORG_ROLE=Viewer

# Additional security settings
ENV GF_SECURITY_DISABLE_GRAVATAR=true
ENV GF_SECURITY_COOKIE_SECURE=true
ENV GF_SECURITY_COOKIE_SAMESITE=strict
ENV GF_SECURITY_STRICT_TRANSPORT_SECURITY=true
ENV GF_SECURITY_CONTENT_TYPE_PROTECTION=true
ENV GF_SECURITY_X_CONTENT_TYPE_OPTIONS=nosniff
ENV GF_SECURITY_X_XSS_PROTECTION=true
ENV GF_LOG_MODE=console
ENV GF_LOG_LEVEL=info

# Copy configuration files with proper permissions
COPY --chmod=755 ./provisioning /etc/grafana/provisioning
COPY --chmod=644 ./datasources /etc/grafana/provisioning/datasources
COPY --chmod=644 config.ini /etc/grafana/config.ini
COPY --chmod=644 ./dashboards /var/lib/grafana/dashboards
COPY --chmod=755 ./plugins /var/lib/grafana/plugins

# Ensure proper ownership and permissions after copying
USER root
RUN chown -R 472:0 /etc/grafana/provisioning /var/lib/grafana/dashboards /var/lib/grafana/plugins && \
    find /etc/grafana/provisioning -type d -exec chmod 755 {} \; && \
    find /etc/grafana/provisioning -type f -exec chmod 644 {} \;

# Use non-root grafana user
USER grafana

# Container health check against Grafana's health endpoint (DS-0026)
HEALTHCHECK --interval=30s --timeout=5s --start-period=20s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:3000/api/health || exit 1

# Expose Grafana port
EXPOSE 3000
