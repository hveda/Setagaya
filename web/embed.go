// Package web embeds the built SPA (web/dist, produced by `bun run build`)
// so cmd/api can serve it as static assets without a separate deployable.
//
// web/dist is gitignored -- its real contents are a build artifact, not
// source -- except for dist/.gitkeep, committed solely so this directive has
// at least one file to find in a checkout that has never run the JS build
// (go:embed fails to compile against a directory with zero files). Running
// `bun run build` locally empties and repopulates dist, which removes that
// placeholder; that is expected, not a mistake to fix.
package web

import "embed"

// Dist is the SPA's build output. Paths are rooted at "dist" (e.g.
// "dist/index.html", "dist/assets/..."); callers that want "index.html"
// directly should wrap it with fs.Sub(Dist, "dist").
//
//go:embed all:dist
var Dist embed.FS
