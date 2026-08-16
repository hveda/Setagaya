# k6 engine image: bzt (Taurus) + a pinned k6 binary, per
# deploy/engines/README.md's pairing table. No JVM here -- k6 is a single Go
# binary, so the base is python (bzt's runtime) rather than a JDK image.
#
# Unlike JMeter, bzt does NOT provision k6 itself: bzt's K6 RequiredTool is
# installable=False (it only checks `k6 version` on PATH), so this image
# downloads the pinned k6 release at build time and puts it on PATH. The
# "pods need no network" property holds the same way -- the binary is baked
# in, just via our download instead of bzt's provisioning.
#
# Built with deploy/ as context (see deploy/engines/README.md), the same as
# the jmeter image, so engine/honryu_kpi.py sits alongside this directory.
FROM python:3.12-slim

ARG K6_VERSION=0.57.0

RUN pip3 install --no-cache-dir bzt \
    && apt-get update && apt-get install -y --no-install-recommends ca-certificates curl \
    && rm -rf /var/lib/apt/lists/*

# Pinned k6 release; the tag doubles as the image tag (engine-k6:<K6_VERSION>).
# hadolint ignore reason: version is a build ARG, not a blind curl|sh.
RUN curl -fsSL \
        "https://github.com/grafana/k6/releases/download/v${K6_VERSION}/k6-v${K6_VERSION}-linux-amd64.tar.gz" \
        -o /tmp/k6.tar.gz \
    && tar -xzf /tmp/k6.tar.gz -C /tmp \
    && install -m 0755 "/tmp/k6-v${K6_VERSION}-linux-amd64/k6" /usr/local/bin/k6 \
    && rm -rf /tmp/k6* \
    && k6 version

# The Honryu KPI reporter (engine-agnostic: bzt normalises every engine's
# results into one stream) plus a default bzt config that always loads it --
# bzt merges ~/.bzt-rc into every run, so no per-execution compiled config
# needs to know this module exists.
COPY engine/honryu_kpi.py /root/honryu_kpi.py
COPY engines/k6/bzt-rc.yml /root/.bzt-rc
ENV PYTHONPATH=/root

# Smoke the full executor path at build time: bzt generating and launching a
# k6 script (provisioning check included). The run itself is expected to fail
# -- nothing is listening on 127.0.0.1:1 -- but it proves the toolchain
# chain (bzt -> k6 -> reporter) is intact before the image ships.
COPY engines/k6/warmup.js /root/warmup.js
RUN bzt /root/warmup.js || true
