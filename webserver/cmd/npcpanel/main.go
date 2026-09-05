// Command npcpanel serves the moderator panel that maps every NPC in the world
// by city and, when a database is configured, lets a moderator turn one on or
// off, move it, and edit what it sells.
//
// It is a GM tool in the same shape as itembrowser: bind it to loopback, because
// it has no login of its own. Authorization is still real — every write goes
// through npcadmin, which re-checks that -moderator names an account whose role
// is 'moderator' or 'admin', so a wrong id fails closed.
//
// Without -dsn it reads only the content tree and the panel is read-only, which
// is enough to answer "which NPCs do we have, and where":
//
//	go run ./webserver/cmd/npcpanel -content Release
//	go run ./webserver/cmd/npcpanel -content Release -dsn "$W2PP_DB_DSN" -moderator 1
package main

import (
	"context"
	"errors"
	"flag"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/jeanluca/w2pp-openwyd/internal/store"
	"github.com/jeanluca/w2pp-openwyd/webserver/internal/itemcatalog"
	"github.com/jeanluca/w2pp-openwyd/webserver/internal/itemicons"
	"github.com/jeanluca/w2pp-openwyd/webserver/internal/npcadmin"
	"github.com/jeanluca/w2pp-openwyd/webserver/internal/npcpanel"
)

func main() {
	contentDir := flag.String("content", "Release", "content tree holding TMsrv/run/NPCGener.txt and Common/ItemList.csv")
	addr := flag.String("addr", "127.0.0.1:8089", "listen address (keep it on loopback: no auth)")
	iconDir := flag.String("icons", "", "generated item-icon pack directory; empty disables icons")
	dsn := flag.String("dsn", os.Getenv("W2PP_DB_DSN"), "PostgreSQL DSN; empty makes the panel read-only")
	moderatorID := flag.Int64("moderator", 0, "account id used for every write; must have role moderator or admin")
	all := flag.Bool("all", false, "list every NPC (quest givers, banks, mounts, event actors) instead of only shopkeepers")
	flag.Parse()

	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))

	data, err := npcpanel.Load(*contentDir)
	if err != nil {
		logger.Error("load npc inventory", "content", *contentDir, "err", err)
		os.Exit(1)
	}
	logger.Info("npc inventory loaded",
		"blocks", data.Stats.Blocks, "npcs", len(data.NPCs),
		"shops", data.Stats.Shops, "merchants", data.Stats.Merchants,
		"in_settlement", data.Stats.InSettlement,
		"missing_template", data.Stats.MissingTemplate,
		"undecodable", data.Stats.UndecodableMob)

	// Shopkeepers are the default scope: they are the NPCs with stock to edit,
	// and the Merchant byte's other values are quest givers, mounts and event
	// actors that read as noise in a shop list (npc-map.md).
	if !*all {
		data = data.OnlyShops()
		logger.Info("listing shopkeepers only", "shops", len(data.NPCs), "use", "-all para o mapa completo")
	}

	cfg := npcpanel.Config{Data: data, ModeratorID: *moderatorID, Logger: logger}

	if *dsn != "" {
		if *moderatorID <= 0 {
			logger.Error("-dsn requires -moderator: writes are authorized by account role")
			os.Exit(1)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		pool, err := store.Pool(ctx, *dsn)
		if err != nil {
			cancel()
			logger.Error("connect to database", "err", err)
			os.Exit(1)
		}
		defer pool.Close()
		if err := store.Migrate(ctx, pool); err != nil {
			cancel()
			logger.Error("apply migrations", "err", err)
			os.Exit(1)
		}
		cancel()

		admin := npcadmin.New(store.New(pool))
		admin.SetLogger(logger)
		// The item picker and the shop editor both name items from the same
		// catalog the inventory already read, so scan it once more here rather
		// than threading it out of npcpanel.Load.
		if catalog, err := itemcatalog.Scan(*contentDir); err != nil {
			logger.Warn("item catalog unavailable to the admin service", "err", err)
		} else {
			admin.SetItemCatalog(catalog)
		}
		cfg.Admin = admin
		logger.Info("database connected: panel is editable", "moderator", *moderatorID)
	} else {
		logger.Info("no -dsn: panel is read-only")
	}

	// Icons are optional: the pixels come from the classic client and are not
	// versioned in this repo, so a missing pack degrades to name-only rows.
	if *iconDir != "" {
		if manifest, err := itemicons.Load(filepath.Join(*iconDir, "manifest.json")); err != nil {
			logger.Warn("icon pack ignored", "dir", *iconDir, "err", err)
		} else {
			cfg.IconDir = filepath.Join(*iconDir, manifest.PackVersion)
			logger.Info("icon pack loaded", "pack", manifest.PackVersion, "mapped", manifest.MappedItems)
		}
	}

	srv := &http.Server{
		Addr:              *addr,
		Handler:           npcpanel.Handler(cfg),
		ReadHeaderTimeout: 5 * time.Second,
	}
	logger.Info("npc panel ready", "url", "http://"+*addr, "npcs", len(data.NPCs))
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		logger.Error("listen", "addr", *addr, "err", err)
		os.Exit(1)
	}
}
