// Command itembrowser serves a local, searchable view of the item catalog.
//
// It is a developer/GM tool: it reads the read-only content tree and never
// touches the database or live game state. Bind it to loopback (the default) —
// there is no authentication, and the catalog is server content.
//
//	go run ./webserver/cmd/itembrowser -content Release
//	go run ./webserver/cmd/itembrowser -content Release -icons dist/item-icons
package main

import (
	"errors"
	"flag"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/jeanluca/w2pp-openwyd/webserver/internal/itembrowser"
	"github.com/jeanluca/w2pp-openwyd/webserver/internal/itemicons"
)

func main() {
	contentDir := flag.String("content", "Release", "content tree holding Common/ItemList.csv")
	addr := flag.String("addr", "127.0.0.1:8088", "listen address (keep it on loopback: no auth)")
	iconDir := flag.String("icons", "", "generated item-icon pack directory (manifest.json + <pack_version>/*.png); empty disables icons")
	flag.Parse()

	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))

	data, err := itembrowser.Load(*contentDir)
	if err != nil {
		logger.Error("load catalog", "content", *contentDir, "err", err)
		os.Exit(1)
	}

	// Icons are optional: the pixels come from the classic client and are not
	// versioned in this repo, so a missing pack degrades to name-only browsing
	// rather than failing the tool.
	served := ""
	if *iconDir != "" {
		manifest, err := itemicons.Load(filepath.Join(*iconDir, "manifest.json"))
		switch {
		case err != nil:
			logger.Warn("icon pack ignored", "dir", *iconDir, "err", err)
		default:
			itembrowser.ApplyIcons(&data, manifest)
			served = *iconDir
			logger.Info("icon pack loaded", "pack", manifest.PackVersion, "mapped", manifest.MappedItems)
		}
	}

	srv := &http.Server{
		Addr:              *addr,
		Handler:           itembrowser.Handler(data, served),
		ReadHeaderTimeout: 5 * time.Second,
	}
	logger.Info("item browser ready", "url", "http://"+*addr, "items", len(data.Items))
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		logger.Error("listen", "addr", *addr, "err", err)
		os.Exit(1)
	}
}
