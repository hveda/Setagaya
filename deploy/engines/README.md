# Engine images

Honryu runs each load-test engine inside `bzt` (Taurus) in a pod. The control
plane never runs an engine itself: it compiles a Taurus config, and the pod runs
it.

There is **one image per engine**, not one image for all of them, because
engines do not share a runtime. This is not a preference — it is forced:

> bzt's bundled Gatling 3.9.5 compiles its generated Scala simulation at run
> time with Scala 2.13.10, which **cannot read JDK 21 class files**. A Gatling
> image therefore needs a JDK that Scala 2.13.10 understands, while JMeter is
> happy on 21. One image cannot satisfy both.

Verified during the Phase 0 spike: Gatling failed with
`ClassfileParser … errorBadIndex` on JDK 21, while JMeter and k6 ran clean.

## Configuration

Two environment variables select engines at run time:

| Variable | Meaning |
|---|---|
| `HONRYU_ENGINE_IMAGES` | `engine=image:tag` pairs, comma-separated |
| `HONRYU_DEFAULT_ENGINE` | engine used when an execution names none |

```sh
HONRYU_ENGINE_IMAGES="jmeter=registry.example/honryu/engine-jmeter:5.6.3,k6=registry.example/honryu/engine-k6:1.0.0"
HONRYU_DEFAULT_ENGINE=jmeter
```

Both are validated at startup, not at deploy time. A default engine with no
image, an unknown engine name, or an image without a tag or digest all stop the
server from starting — the alternative is a blank image reference surfacing in a
pod spec minutes into someone's scheduled run.

An **untagged image is rejected**: it follows whatever the registry currently
calls `latest`, so the engine a past run used could not be reproduced.

An execution may name an engine this deployment has no image for. That is an
error naming the engine, never a silent fall back to the default: running a
different engine than the one asked for would produce results for a workload
nobody requested.

## Building an image

Each image needs `bzt` plus that engine's toolchain, and nothing else:

| Engine | Toolchain | Notes |
|---|---|---|
| `jmeter` | JDK 21, JMeter 5.6.x | bzt downloads JMeter on first use unless it is baked in; bake it in so pods need no network |
| `k6` | k6 binary | script-only — a scenario must carry a `.js`, since bzt's k6 executor rejects the declarative form. Image: `deploy/engines/k6/` (unlike JMeter, bzt never provisions k6 — `installable=False` — so the image bakes the pinned binary in itself) |
| `gatling` | **JDK ≤ 17**, Gatling 3.9.5 | see the Scala constraint above; unsupported until the pairing is settled (spec open question 1) |

An engine is only supported once its image has run the checks in
`test/engine/` (`make engine`) against that image.

## Status

`jmeter` and `k6` are exercised end to end by `make engine`. `gatling` is
deferred: the domain models it as a declarative-capable engine, but no image
pairing has been validated, so it should not appear in `HONRYU_ENGINE_IMAGES`
until one has.
