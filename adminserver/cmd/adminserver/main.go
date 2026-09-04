// Command adminserver is the staff admin panel: account search, role, VIP and
// audit, served as plain HTTP with an embedded UI.
//
// It is deliberately a SEPARATE service, not a surface bolted onto webServer.
// The game services run in production with players connected; a panel that lives
// inside one of them cannot be rolled back without rolling back the game too.
// Standalone, the whole feature is deletable: nothing in tmServer, dbServer,
// binServer or webServer imports it or knows it exists, and the only shared
// resource is the database — where every change this feature needs is additive.
//
// Plain HTTP rather than gRPC for the same reason. gRPC is right for the internal
// links, but this endpoint is opened in a browser, and browsers do not speak it —
// gRPC here would mean grpc-web plus a proxy, or a second service whose only job
// is translation. Both cost more than they return for a staff-only panel.
//
// This first cut serves only /healthz. That is the point: it puts the deploy
// pipeline — build arg, port binding, domain, health probe — under test while
// there is still nothing else that could be blamed for a failure.
//
// Usage:
//
//	adminserver [-addr :8080]
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

// Shutdown budget. Long enough to finish an in-flight request, short enough that
// the platform's own stop timeout never fires first and turns a clean stop into a
// kill.
const shutdownTimeout = 10 * time.Second

// HTTP server timeouts. Go's defaults are unlimited, which on a public endpoint
// lets a slow or abandoned client hold a connection open indefinitely — and this
// service, unlike the internal ones, is reachable from the internet.
const (
	readHeaderTimeout = 5 * time.Second
	readTimeout       = 15 * time.Second
	writeTimeout      = 30 * time.Second
	idleTimeout       = 60 * time.Second
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	if err := run(logger); err != nil {
		logger.Error("adminserver failed", "err", err)
		os.Exit(1)
	}
}

// run serves HTTP until the process receives SIGINT/SIGTERM, then drains.
func run(logger *slog.Logger) error {
	addr := flag.String("addr", defaultAddr(), "HTTP listen address")
	flag.Parse()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	srv := &http.Server{
		Addr:              *addr,
		Handler:           routes(logger),
		ReadHeaderTimeout: readHeaderTimeout,
		ReadTimeout:       readTimeout,
		WriteTimeout:      writeTimeout,
		IdleTimeout:       idleTimeout,
	}

	errCh := make(chan error, 1)
	go func() {
		logger.Info("adminserver listening", "addr", *addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- fmt.Errorf("serve: %w", err)
			return
		}
		errCh <- nil
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		logger.Info("shutting down")
		shutCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()
		if err := srv.Shutdown(shutCtx); err != nil {
			return fmt.Errorf("shutdown: %w", err)
		}
		return <-errCh
	}
}

// routes builds the mux. Everything the panel will grow lands here behind a
// session gate; /healthz is the one route that must never have one.
func routes(logger *slog.Logger) http.Handler {
	mux := http.NewServeMux()

	// Liveness. It answers from memory and touches nothing — no database, no
	// disk, no outbound call. A probe that depends on Postgres turns a slow
	// query into a restart, and a restart loop into an outage that looks like a
	// database problem while actually being caused by the probe. It also must
	// not be logged per request: the platform calls it on a schedule forever,
	// and that noise buries the events worth reading.
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		_, _ = w.Write([]byte("ok\n"))
	})

	// Placeholder root. Replaced by the embedded UI once there is one; until
	// then it answers so a browser hitting the domain gets something honest
	// instead of a 404 that reads as a broken deploy.
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		logger.Info("root requested", "ip", clientIP(r))
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte("adminserver up; painel ainda nao implantado\n"))
	})

	return mux
}

// defaultAddr honours the port the platform assigns. Railway (and most PaaS)
// inject PORT and route to it; binding a hardcoded port there means the health
// probe hits a closed socket and the deploy is marked failed with the process
// running fine.
func defaultAddr() string {
	if p := os.Getenv("PORT"); p != "" {
		return ":" + p
	}
	return ":8080"
}

// clientIP prefers the forwarded address, since the service always runs behind
// the platform's proxy and RemoteAddr is that proxy for every request. The value
// is caller-controlled, so it is fit for logs and never for authorization.
func clientIP(r *http.Request) string {
	if f := r.Header.Get("X-Forwarded-For"); f != "" {
		return f
	}
	return r.RemoteAddr
}
