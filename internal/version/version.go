// Package version holds the release version shared by the server and agent.
package version

// Version is overridden at build time with -ldflags "-X onsite-ai/internal/version.Version=...".
var Version = "0.1.0"
