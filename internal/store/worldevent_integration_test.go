//go:build integration

package store

import (
	"context"
	"testing"

	"github.com/jeanluca/w2pp-openwyd/internal/domain"
)

func TestWorldEventConfigCRUDAndProgress(t *testing.T) {
	ctx := context.Background()
	pool := testPool(t)
	resetTestSchema(ctx, pool)
	if err := Migrate(ctx, pool); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	var modID int64
	if err := pool.QueryRow(ctx,
		`INSERT INTO account (name, pass_hash, role) VALUES ('mod_worldevent_test','x','moderator') RETURNING id`).
		Scan(&modID); err != nil {
		t.Fatalf("seed moderator: %v", err)
	}

	st := New(pool)
	// A linha que a 0015 cria já nasce com a guerra de torres da 0051: ligada, às
	// 20h. Sem isso um servidor recém-migrado ficaria sem a guerra diária até
	// alguém abrir o painel.
	if inicial, err := st.WorldEventConfig(ctx); err != nil ||
		!inicial.TowerWarEnabled || inicial.TowerWarHour != domain.DefaultTowerWarHour {
		t.Fatalf("config inicial = %+v/%v, want guerra de torres ligada às %dh", inicial, err, domain.DefaultTowerWarHour)
	}
	v, err := st.WorldEventConfigVersion(ctx)
	if err != nil || v != 0 {
		t.Fatalf("initial version = %d/%v, want 0/nil", v, err)
	}
	cfg := domain.WorldEventConfig{
		Enabled: true, ItemIndex: 777, Rate: 2,
		StartIndex: 100, CurrentIndex: 100, EndIndex: 200,
		Indexed: true, NoticeEnabled: true, DoubleExpEnabled: true, KefraLiveEnabled: true,
		TowerWarEnabled: false, TowerWarHour: 18,
		BossRespawnHours: domain.DefaultBossRespawnHours,
	}
	if err := st.UpsertWorldEventConfig(ctx, cfg, modID); err != nil {
		t.Fatalf("UpsertWorldEventConfig: %v", err)
	}
	v, err = st.WorldEventConfigVersion(ctx)
	if err != nil || v != 1 {
		t.Fatalf("version after upsert = %d/%v, want 1/nil", v, err)
	}
	got, err := st.WorldEventConfig(ctx)
	if err != nil {
		t.Fatalf("WorldEventConfig: %v", err)
	}
	// O formulário não grava o estado do Kefra (0067): ele fica como estava, vivo.
	if got.KefraLiveEnabled {
		t.Error("o formulário dos eventos gravou o estado do Kefra")
	}
	if got.ItemIndex != 777 || got.CurrentIndex != 100 || !got.DoubleExpEnabled {
		t.Fatalf("config = %+v, want saved values", got)
	}
	if got.TowerWarEnabled || got.TowerWarHour != 18 {
		t.Errorf("guerra de torres voltou como %v às %dh, want desligada às 18h", got.TowerWarEnabled, got.TowerWarHour)
	}
	var auditCount int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM world_event_audit`).Scan(&auditCount); err != nil {
		t.Fatal(err)
	}
	if auditCount != 1 {
		t.Fatalf("audit count = %d, want 1", auditCount)
	}

	applied, err := st.UpdateWorldEventProgress(ctx, 1, 101)
	if err != nil || !applied {
		t.Fatalf("UpdateWorldEventProgress = %v/%v, want applied", applied, err)
	}
	v, err = st.WorldEventConfigVersion(ctx)
	if err != nil || v != 1 {
		t.Fatalf("version after progress = %d/%v, want unchanged 1", v, err)
	}
	got, _ = st.WorldEventConfig(ctx)
	if got.CurrentIndex != 101 {
		t.Fatalf("current after progress = %d, want 101", got.CurrentIndex)
	}

	applied, err = st.UpdateWorldEventProgress(ctx, 0, 150)
	if err != nil || applied {
		t.Fatalf("stale progress = %v/%v, want not applied", applied, err)
	}
	got, _ = st.WorldEventConfig(ctx)
	if got.CurrentIndex != 101 {
		t.Fatalf("current after stale progress = %d, want unchanged 101", got.CurrentIndex)
	}

	applied, err = st.UpdateWorldEventProgress(ctx, 1, 100)
	if err != nil || !applied {
		t.Fatalf("lower progress = %v/%v, want applied no-op", applied, err)
	}
	got, _ = st.WorldEventConfig(ctx)
	if got.CurrentIndex != 101 {
		t.Fatalf("current after lower progress = %d, want monotonic 101", got.CurrentIndex)
	}
}

// TestKefraStateGravaSoOEstadoEAudita: o estado do Kefra tem o caminho próprio
// (0067). Grava as duas colunas, audita com a fonte, sobe a versão, e o formulário
// salvo depois não desfaz o estado.
func TestKefraStateGravaSoOEstadoEAudita(t *testing.T) {
	ctx := context.Background()
	pool := testPool(t)
	resetTestSchema(ctx, pool)
	if err := Migrate(ctx, pool); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	st := New(pool)

	v, err := st.SetKefraState(ctx, true, 7, FonteEventoJogo, 0)
	if err != nil || v != 1 {
		t.Fatalf("SetKefraState = %d/%v, want 1/nil", v, err)
	}
	got, err := st.WorldEventConfig(ctx)
	if err != nil || !got.KefraLiveEnabled || got.KefraGuildID != 7 {
		t.Fatalf("config = %+v/%v, want Kefra derrotado pela guilda 7", got, err)
	}
	var conta *int64
	var fonte string
	if err := pool.QueryRow(ctx,
		`SELECT account_id, fonte FROM world_event_audit ORDER BY id DESC LIMIT 1`).Scan(&conta, &fonte); err != nil {
		t.Fatal(err)
	}
	if conta != nil || fonte != FonteEventoJogo {
		t.Errorf("auditoria = conta %v fonte %q, want sem conta e fonte jogo", conta, fonte)
	}

	cfg := domain.DefaultWorldEventConfig()
	cfg.DoubleExpEnabled = true
	if err := st.UpsertWorldEventConfig(ctx, cfg, 0); err != nil {
		t.Fatalf("UpsertWorldEventConfig: %v", err)
	}
	if got, _ = st.WorldEventConfig(ctx); !got.KefraLiveEnabled || got.KefraGuildID != 7 || !got.DoubleExpEnabled {
		t.Errorf("depois do formulário = %+v, want Kefra ainda derrotado pela 7 e XP em dobro", got)
	}

	if _, err := st.SetKefraState(ctx, false, 9, FonteEventoPainel, 0); err != nil {
		t.Fatalf("SetKefraState vivo: %v", err)
	}
	if got, _ = st.WorldEventConfig(ctx); got.KefraLiveEnabled || got.KefraGuildID != 0 {
		t.Errorf("Kefra vivo = %+v, want sem guilda", got)
	}
}

// TestHoraDaGuerraDeTorresForaDaFaixaERecusada: the CHECK is the last line of
// defence after the panel and the web service. An hour of 24 would never match
// the clock, and the war would silently never start.
func TestHoraDaGuerraDeTorresForaDaFaixaERecusada(t *testing.T) {
	ctx := context.Background()
	pool := testPool(t)
	resetTestSchema(ctx, pool)
	if err := Migrate(ctx, pool); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	st := New(pool)
	for _, hora := range []int32{-1, 24} {
		cfg := domain.DefaultWorldEventConfig()
		cfg.TowerWarHour = hora
		if err := st.UpsertWorldEventConfig(ctx, cfg, 0); err == nil {
			t.Errorf("o banco aceitou a guerra de torres às %dh", hora)
		}
	}
}
