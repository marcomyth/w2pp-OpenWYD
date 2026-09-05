// Package buildinfo reports which build of the server is running.
//
// It exists to settle one recurring, expensive question: "is the fix already
// deployed?" Without it, a report from the game is ambiguous — a bug that looks
// unfixed may simply be an older binary still serving. Every service logs this
// on boot, so the answer is one line in the log instead of a guess.
package buildinfo

import "runtime/debug"

// Commit is the git revision, injected at link time:
//
//	go build -ldflags "-X github.com/jeanluca/w2pp-openwyd/internal/buildinfo.Commit=$(git rev-parse --short=8 HEAD)"
//
// Left empty it falls back to the VCS stamp the Go toolchain embeds
// automatically, which covers a plain `go build` from a clean checkout.
var Commit string

// BuiltAt is the build timestamp (RFC 3339), injected the same way. Empty when
// the linker flag was not passed.
var BuiltAt string

// Revision returns the running build's revision, preferring the injected value
// and falling back to the toolchain's VCS stamp. It returns "unknown" when
// neither is available — a plain `go run`, or a build from a dirty tree with
// stamping disabled.
func Revision() string {
	if Commit != "" {
		return Commit
	}
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "unknown"
	}
	var rev, modified string
	for _, s := range info.Settings {
		switch s.Key {
		case "vcs.revision":
			rev = s.Value
		case "vcs.modified":
			modified = s.Value
		}
	}
	if rev == "" {
		return "unknown"
	}
	if len(rev) > 8 {
		rev = rev[:8]
	}
	if modified == "true" {
		return rev + "-dirty"
	}
	return rev
}

// Built returns the build timestamp, or the toolchain's VCS time, or "unknown".
func Built() string {
	if BuiltAt != "" {
		return BuiltAt
	}
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "unknown"
	}
	for _, s := range info.Settings {
		if s.Key == "vcs.time" {
			return s.Value
		}
	}
	return "unknown"
}
