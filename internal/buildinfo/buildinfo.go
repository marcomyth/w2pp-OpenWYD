// Package buildinfo reports which build of the server is running.
//
// It exists to settle one recurring, expensive question: "is the fix already
// deployed?" Without it, a report from the game is ambiguous — a bug that looks
// unfixed may simply be an older binary still serving. Every service logs this
// on boot, so the answer is one line in the log instead of a guess.
package buildinfo

import (
	"os"
	"runtime/debug"
	"strings"
)

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
		return doAmbiente()
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
		return doAmbiente()
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

// doAmbiente é a última tentativa: a revisão que a PLATAFORMA injeta como variável.
//
// POR QUE ELA EXISTE: em 24/09/2026 medi o boot do tmserver em produção e ele dizia
// `revision=unknown built=unknown`. Ou seja, o pacote inteiro — que existe para
// responder "o conserto já está no ar?" — estava mudo justamente no lugar onde a
// pergunta é feita. O construtor da plataforma compila fora de um checkout git, então
// o carimbo do toolchain não existe, e ninguém passa o -ldflags.
//
// A plataforma, porém, injeta a revisão como variável de ambiente em todo serviço. Ler
// dali não é tão bom quanto o carimbo do compilador — é a palavra de quem construiu
// sobre o que construiu, e não do binário sobre si mesmo — mas é infinitamente melhor
// que "unknown", e é o que transforma um palpite numa linha de log.
func doAmbiente() string {
	for _, nome := range []string{"GIT_COMMIT", "RAILWAY_GIT_COMMIT_SHA"} {
		if v := strings.TrimSpace(os.Getenv(nome)); v != "" {
			if len(v) > 8 {
				v = v[:8]
			}
			return v + " (do ambiente)"
		}
	}
	return "unknown"
}
