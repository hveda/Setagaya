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
FROM python:3.12-slim@sha256:2c941e860699f878900b0edc2403613c234d4b32eda3cc9fa7036991a2a63c4a

ARG K6_VERSION=0.57.0

# bzt 1.16.51 -- the version this image already shipped -- installed from a
# fully pinned, hash-verified set. Pinning bzt alone still let its ~60
# transitive dependencies float, so two builds of this file produced
# different images; --require-hashes also makes a substituted artifact fail
# the build rather than ship. See requirements.txt for how to regenerate.
COPY engines/k6/requirements.txt /tmp/requirements.txt
# The setuptools bump is a deliberate override of an upstream pin. It is
# installed FIRST and the resolved set follows with --no-deps, so the
# vulnerable version is never written into requirements.txt at all -- the
# file then describes what the image actually contains, which is the whole
# point of pinning it. --no-deps is safe because pip-compile's output is a
# complete closure: every transitive dependency is already enumerated.
#
# bzt declares setuptools==79.0.1 exactly, which carries CVE-2026-59890
# (medium, fixed in 83.0.0). There is no bzt release that avoids it -- 1.17.0
# merely moves the pin to 80.10.2, still below the fix -- so the constraint
# cannot be satisfied by upgrading bzt, and pip-compile fails outright on a
# setuptools>=83 requirement. Note this vulnerable setuptools was already in
# the published engine images (verified with pip3 show against
# engine-k6:0.57.0-2); hash-pinning did not introduce it, it made it visible.
#
# Verified before overriding: with setuptools 84.0.0 in place, bzt --help
# runs, bzt/bzt.engine import, and bzt.modules.aggregator still exports the
# AggregatorListener/DataPoint/KPISet API that engine/honryu_kpi.py builds on.
# setuptools is a build-time tool here; nothing in the load path needs the
# exact version bzt names. Installing it before the rest also covers any
# package that has to build from an sdist.
COPY engines/requirements-setuptools.txt /tmp/requirements-setuptools.txt
RUN pip3 install --no-cache-dir --no-deps --require-hashes -r /tmp/requirements-setuptools.txt \
    && pip3 install --no-cache-dir --no-deps --require-hashes -r /tmp/requirements.txt \
    && rm /tmp/requirements.txt /tmp/requirements-setuptools.txt \
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
