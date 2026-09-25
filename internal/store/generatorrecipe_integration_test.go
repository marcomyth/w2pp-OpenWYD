//go:build integration

// Integration tests for the block recipes (0165_receita_de_bloco). They require
// a real database and are excluded from the default build. Run with:
//
//	W2PP_TEST_DSN=postgres://postgres:dev@localhost:5432/postgres go test -tags=integration ./internal/store/
package store

import (
	"context"
	"testing"

	"github.com/jeanluca/w2pp-openwyd/internal/domain"
)

func limparReceitas(t *testing.T, ctx context.Context) *Store {
	t.Helper()
	pool := testPool(t)
	if err := Migrate(ctx, pool); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	_, _ = pool.Exec(ctx, `DELETE FROM npc_generator_recipe; UPDATE npc_generator_recipe_meta SET version = 0 WHERE id = TRUE`)
	return New(pool)
}

func receitaDeTeste(idx int32) domain.GeneratorRecipe {
	return domain.GeneratorRecipe{
		Index: idx, Leader: "Urso", Follower: "Filhote", MinuteGenerate: -1,
		MinGroup: 1, MaxGroup: 49, MaxNumMob: 8, RouteType: 6, Formation: 4,
		SegX: [5]int32{2100, 2105, 0, 0, 2110}, SegY: [5]int32{2100, 2095, 0, 0, 2090},
		SegRange: [5]int32{12, 0, 0, 0, 40}, SegWait: [5]int32{-1, 0, 0, 0, 3},
		Nota: "zona do norte",
	}
}

func TestReceitaDeBlocoCRUD(t *testing.T) {
	ctx := context.Background()
	s := limparReceitas(t, ctx)

	cfg, err := s.GeneratorRecipes(ctx)
	if err != nil {
		t.Fatalf("GeneratorRecipes: %v", err)
	}
	if len(cfg.Recipes) != 0 || cfg.Version != 0 {
		t.Fatalf("a tabela nasce vazia, li %+v", cfg)
	}

	rec := receitaDeTeste(42)
	antes, tinha, err := s.SetGeneratorRecipe(ctx, rec, false, 0)
	if err != nil {
		t.Fatalf("SetGeneratorRecipe: %v", err)
	}
	if tinha || antes.Leader != "" {
		t.Errorf("disse que havia linha numa tabela vazia: %+v", antes)
	}

	cfg, _ = s.GeneratorRecipes(ctx)
	if cfg.Version != 1 || len(cfg.Recipes) != 1 {
		t.Fatalf("depois de gravar: %+v", cfg)
	}
	// Every column round-trips, the arrays included: a waypoint lost on the way
	// would put the zone somewhere else with nothing in any log.
	if got := cfg.Recipes[0]; got != rec {
		t.Errorf("li de volta\n %+v\nquero\n %+v", got, rec)
	}

	// Saving again updates, and the "renew now" counter only moves when asked.
	rec.MaxNumMob = 3
	if _, tinha, err = s.SetGeneratorRecipe(ctx, rec, true, 0); err != nil || !tinha {
		t.Fatalf("regravar: tinha=%v err=%v", tinha, err)
	}
	if _, _, err = s.SetGeneratorRecipe(ctx, rec, false, 0); err != nil {
		t.Fatal(err)
	}
	cfg, _ = s.GeneratorRecipes(ctx)
	if len(cfg.Recipes) != 1 || cfg.Recipes[0].MaxNumMob != 3 || cfg.Recipes[0].Renovar != 1 {
		t.Fatalf("depois de regravar: %+v", cfg.Recipes)
	}
	if cfg.Version != 3 {
		t.Errorf("versão = %d, quero 3 (uma por escrita)", cfg.Version)
	}

	antes, tinha, err = s.DeleteGeneratorRecipe(ctx, 42, 0)
	if err != nil || !tinha || antes.MaxNumMob != 3 {
		t.Fatalf("apagar: %+v tinha=%v err=%v", antes, tinha, err)
	}
	// A second delete finds nothing and still moves the version.
	if _, tinha, err = s.DeleteGeneratorRecipe(ctx, 42, 0); err != nil || tinha {
		t.Fatalf("apagar de novo: tinha=%v err=%v", tinha, err)
	}
	if v, _ := s.GeneratorRecipeVersion(ctx); v != 5 {
		t.Errorf("versão = %d, quero 5", v)
	}
}

// The CHECKs refuse what the game could not honor, whatever wrote the row.
func TestReceitaDeBlocoRecusaValorAbsurdo(t *testing.T) {
	ctx := context.Background()
	s := limparReceitas(t, ctx)
	ruins := map[string]func(*domain.GeneratorRecipe){
		"líder vazio":      func(r *domain.GeneratorRecipe) { r.Leader = "" },
		"formação 5":       func(r *domain.GeneratorRecipe) { r.Formation = 5 },
		"teto abaixo de -1": func(r *domain.GeneratorRecipe) { r.MaxNumMob = -2 },
		"grupo de 101":     func(r *domain.GeneratorRecipe) { r.MaxGroup = 101 },
	}
	for nome, estraga := range ruins {
		rec := receitaDeTeste(7)
		estraga(&rec)
		if _, _, err := s.SetGeneratorRecipe(ctx, rec, false, 0); err == nil {
			t.Errorf("%s: o banco aceitou", nome)
		}
	}
	if _, _, err := s.SetGeneratorRecipe(ctx, receitaDeTeste(domain.MaxGeneratorIndex+1), false, 0); err == nil {
		t.Error("um índice acima do int16 foi aceito")
	}
	if cfg, _ := s.GeneratorRecipes(ctx); len(cfg.Recipes) != 0 {
		t.Errorf("gravou %d linhas apesar das recusas", len(cfg.Recipes))
	}
}
