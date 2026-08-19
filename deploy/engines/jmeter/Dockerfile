# JMeter engine image: bzt (Taurus) + JDK 21 + JMeter 5.6.x, per
# deploy/engines/README.md's pairing table -- JMeter (plus its plugin
# manager) baked in via a real bzt provisioning run *at build time*, so a
# pod needs no network to start generating load ("pods need no network",
# deploy/engines/README.md). Verified live: without this, a fresh pod's
# first run spent its entire steady-state hold downloading JMeter and never
# sent a single request (Phase 7 task 85's live verification).
FROM eclipse-temurin:21-jdk-jammy@sha256:dbfd085220ae632a0830166e443747d1ee89e9038d92e3b48c3e5e9d8292b9a7

RUN apt-get update && apt-get install -y --no-install-recommends \
        python3 python3-pip \
    && rm -rf /var/lib/apt/lists/*

# bzt 1.16.51 -- the version this image already shipped (read off the running
# engine-jmeter:5.6.3-2 with `pip3 show bzt`), so this changes nothing about
# behaviour and only removes drift. Unpinned, this installed whatever bzt was
# newest at build time, with all ~70 transitive dependencies floating too --
# which sat badly beside a Dockerfile whose whole point is that "pods need no
# network" because the engine is baked in at a known version.
# See requirements.txt for how to regenerate the hashed set.
COPY engines/jmeter/requirements.txt /tmp/requirements.txt
# setuptools override -- see the equivalent block in engines/k6/Dockerfile for
# the full reasoning. Short version: bzt pins setuptools==79.0.1, which has
# CVE-2026-59890 (medium, fixed in 83.0.0), and no bzt release avoids it.
# Installed first, with the resolved set following via --no-deps, so the
# vulnerable version never appears in requirements.txt.
COPY engines/requirements-setuptools.txt /tmp/requirements-setuptools.txt
RUN pip3 install --no-cache-dir --no-deps --require-hashes -r /tmp/requirements-setuptools.txt \
    && pip3 install --no-cache-dir --no-deps --require-hashes -r /tmp/requirements.txt \
    && rm /tmp/requirements.txt /tmp/requirements-setuptools.txt

# The Honryu KPI reporter (streams bzt's per-second aggregated results to
# the path the sidecar tails) and a default bzt config that always loads
# it -- bzt merges ~/.bzt-rc into every run, so no per-execution compiled
# config needs to know this module exists. Pins the JMeter version to match
# this image's own tag, rather than trusting bzt's own shifting default.
# Built with deploy/ as context (see deploy/engines/README.md), so
# deploy/engine/honryu_kpi.py sits alongside this Dockerfile's own dir.
COPY engine/honryu_kpi.py /root/honryu_kpi.py
COPY engines/jmeter/bzt-rc.yml /root/.bzt-rc
ENV PYTHONPATH=/root

# Trigger bzt's own provisioning (JMeter binary + plugin manager + plugins)
# against a config that never reaches the "make a request" stage -- baking
# its ~/.bzt cache into this image layer. The run itself is expected to
# fail (nothing is listening on the bogus target); only the provisioning
# side effect, which happens first, is what this step is for.
COPY engines/jmeter/warmup.yml /root/warmup.yml
RUN bzt /root/warmup.yml -o settings.check-updates=false; true
