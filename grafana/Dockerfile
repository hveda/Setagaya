# Use latest stable Grafana with security updates (pinned by digest)
FROM grafana/grafana:13.1.3@sha256:ab5cb380e3ff3172d6c8bd2e7cfd31cce977d2881b260e1f5bc089bf0b759b43

# Security environment variables
ENV GF_SECURITY_ADMIN_USER=honryu
ENV GF_SECURITY_ADMIN_PASSWORD=honryu
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

# Grafana 13's plugin "preinstall" feature reaches out to grafana.com for a
# default catalog (grafana-lokiexplore-app, elasticsearch, zipkin, ...) on
# every cold start. Found live (2026-08-18, phase 16 cutover): this cluster
# has no route to the public internet, so each of the ~6 default plugins
# times out (~10-15s apiece) before Grafana's HTTP listener binds --
# 60-90s+ added to every pod start/restart, and the readiness probe reports
# "connection refused" the whole time. None of these plugins are used by
# this deployment's dashboards.
ENV GF_PLUGINS_PREINSTALL_DISABLED=true

# Copy configuration files with proper permissions
#
# No ./plugins copy: the only vendored plugin was grafana-piechart-panel
# (AngularJS), used by honryu.json's three "Response Status" panels. Angular
# was defaulted off in Grafana 11 and removed in 12 -- unrenderable on this
# pinned 13.1.3 regardless -- so those panels were migrated to Grafana's
# built-in piechart type instead of carrying a dead plugin forward.
COPY --chmod=755 ./provisioning /etc/grafana/provisioning
COPY --chmod=644 ./datasources /etc/grafana/provisioning/datasources
COPY --chmod=644 config.ini /etc/grafana/config.ini
COPY --chmod=644 ./dashboards /var/lib/grafana/dashboards

# Ensure proper ownership and permissions after copying.
#
# /var/lib/grafana/dashboards is included here for the same reason
# /etc/grafana/provisioning is: COPY --chmod applies its one mode to both
# files AND the directory it creates, and 644 on a directory has no execute
# bit -- non-traversable, even by its owner. Found live (2026-08-18): every
# dashboard failed to provision with "lstat .../honryu.json: permission
# denied" until this find pass covered dashboards/ too. Predates this
# phase's panel migration; it is why the dashboards had never actually
# loaded (confirmed via task 14's finding that Grafana had never run here).
USER root
RUN chown -R 472:0 /etc/grafana/provisioning /var/lib/grafana/dashboards && \
    find /etc/grafana/provisioning /var/lib/grafana/dashboards -type d -exec chmod 755 {} \; && \
    find /etc/grafana/provisioning /var/lib/grafana/dashboards -type f -exec chmod 644 {} \;

# Use non-root grafana user
USER grafana

# Container health check against Grafana's health endpoint (DS-0026)
HEALTHCHECK --interval=30s --timeout=5s --start-period=20s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:3000/api/health || exit 1

# Expose Grafana port
EXPOSE 3000
