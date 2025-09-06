# JMeter Engine Docker Build Options

This directory contains two Dockerfile options for building JMeter engines with different JMeter versions and build approaches.

## Modern Approach (Recommended)
**File:** `Dockerfile.engines.jmeter`
- **JMeter Version:** 5.6.3 (latest)
- **Build Method:** Compiles setagaya-agent from source during Docker build
- **Agent Compatibility:** Automatically detects JMeter paths via `JMETER_BIN` environment variable
- **Advantages:** No pre-build step required, uses latest Go security features, version-agnostic agent
- **Usage:** `docker build -f Dockerfile.engines.jmeter .`

## Legacy Approach (Backward Compatibility)
**File:** `Dockerfile.engines.jmeter.legacy`
- **JMeter Version:** 3.3 (legacy)
- **Build Method:** Uses pre-built setagaya-agent binary
- **Agent Compatibility:** Falls back to hardcoded JMeter 3.3 paths when `JMETER_BIN` not available
- **Prerequisites:** Run `./build.sh jmeter` before building Docker image
- **Usage:** 
  ```bash
  ./build.sh jmeter
  docker build -f Dockerfile.engines.jmeter.legacy .
  ```

## Version Compatibility

The setagaya-agent has been updated to support both JMeter versions:

### Automatic Version Detection
- **With `JMETER_BIN` environment variable:** Agent uses dynamic paths (works with any JMeter version)
- **Without `JMETER_BIN` environment variable:** Agent falls back to hardcoded JMeter 3.3 paths

### Environment Variables Set by Dockerfiles
Both Dockerfiles set these environment variables:
- `JMETER_HOME=/opt/apache-jmeter-${JMETER_VERSION}`
- `JMETER_BIN=${JMETER_HOME}/bin`
- `PATH=${JMETER_BIN}:${PATH}`

The agent's `init()` function reads `JMETER_BIN` to determine the correct paths for:
- `JMETER_EXECUTABLE=${JMETER_BIN}/jmeter`
- `JMETER_SHUTDOWN=${JMETER_BIN}/stoptest.sh`

## Migration Notes
- The legacy approach maintains compatibility with existing build processes
- The modern approach is recommended for new deployments
- Both approaches use the same setagaya user (UID 1001) and security practices
- **Version-agnostic:** The same setagaya-agent binary works with both JMeter 3.3 and 5.6.3
