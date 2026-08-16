package config

import (
	"fmt"
	"sort"
	"strings"

	"github.com/heridotlife/honryu/internal/domain/taurus"
)

// ParseEngineImages reads the engine catalogue from its environment form,
// "jmeter=repo/image:tag,k6=repo/image:tag".
//
// Images are per engine rather than one image for everything, because engines
// do not share a runtime: bzt's bundled Gatling 3.9.5 compiles Scala 2.13.10 at
// run time and cannot read JDK 21 class files, so a single image that satisfies
// JMeter and Gatling at once does not exist.
func ParseEngineImages(raw string) (map[taurus.Executor]string, error) {
	images := map[taurus.Executor]string{}
	if strings.TrimSpace(raw) == "" {
		return images, nil
	}

	for _, pair := range strings.Split(raw, ",") {
		pair = strings.TrimSpace(pair)
		if pair == "" {
			continue
		}
		name, image, ok := strings.Cut(pair, "=")
		name = strings.TrimSpace(name)
		image = strings.TrimSpace(image)
		if !ok || image == "" {
			return nil, fmt.Errorf("config: engine image %q must be written engine=image:tag", pair)
		}
		engine := taurus.Executor(name)
		if !engine.Known() {
			return nil, fmt.Errorf("config: unknown engine %q in engine images", name)
		}
		// An untagged image follows whatever the registry calls latest, so the
		// engine a run used could not be reproduced afterwards.
		if !strings.Contains(image, ":") && !strings.Contains(image, "@") {
			return nil, fmt.Errorf("config: engine image %q for %s has no tag or digest", image, name)
		}
		images[engine] = image
	}
	return images, nil
}

// ValidateEngines checks the catalogue is usable before the server accepts
// traffic. Without it a missing image surfaces as a blank image reference in a
// pod spec, minutes into someone's scheduled run.
func (c ClusterConfig) ValidateEngines() error {
	if len(c.EngineImages) == 0 {
		return fmt.Errorf("config: no engine images configured; set %sENGINE_IMAGES", envPrefix)
	}
	if !c.DefaultEngine.Known() {
		return fmt.Errorf("config: unknown default engine %q", c.DefaultEngine)
	}
	if _, ok := c.EngineImages[c.DefaultEngine]; !ok {
		return fmt.Errorf("config: default engine %q has no image in %sENGINE_IMAGES",
			c.DefaultEngine, envPrefix)
	}
	return nil
}

// ImageFor returns the image for an engine. An empty engine means the caller
// expressed no preference and takes the configured default.
//
// An engine with no image is an error rather than a fallback: silently running
// the default engine would produce results for a workload nobody asked for.
func (c ClusterConfig) ImageFor(engine taurus.Executor) (string, error) {
	if engine == "" {
		engine = c.DefaultEngine
	}
	if image, ok := c.EngineImages[engine]; ok {
		return image, nil
	}
	return "", fmt.Errorf("config: no image configured for engine %q (have: %s)",
		engine, strings.Join(c.configuredEngines(), ", "))
}

func (c ClusterConfig) configuredEngines() []string {
	out := make([]string, 0, len(c.EngineImages))
	for e := range c.EngineImages {
		out = append(out, string(e))
	}
	sort.Strings(out)
	return out
}
