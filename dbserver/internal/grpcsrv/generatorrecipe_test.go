package grpcsrv

import (
	"context"
	"errors"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	dbv1 "github.com/jeanluca/w2pp-openwyd/api/db/v1"
	"github.com/jeanluca/w2pp-openwyd/internal/domain"
)

type fakeRecipeStore struct {
	cfg domain.GeneratorRecipeConfig
	err error
}

func (f *fakeRecipeStore) GeneratorRecipeVersion(context.Context) (int64, error) {
	return f.cfg.Version, f.err
}
func (f *fakeRecipeStore) GeneratorRecipes(context.Context) (domain.GeneratorRecipeConfig, error) {
	return f.cfg, f.err
}

func TestNpcRecipeServerEntregaAReceitaInteira(t *testing.T) {
	rec := domain.GeneratorRecipe{
		Index: 20000, Leader: "Lobo", Follower: "Lobo_", MinuteGenerate: 2,
		MinGroup: 1, MaxGroup: 3, MaxNumMob: 8, RouteType: 1, Formation: 2,
		SegX: [5]int32{2100, 0, 0, 0, 2110}, SegY: [5]int32{2100, 0, 0, 0, 2090},
		SegRange: [5]int32{4, 0, 0, 0, 2}, SegWait: [5]int32{-1, 0, 0, 0, 3}, Renovar: 5,
	}
	s := NewNpcRecipe(&fakeRecipeStore{cfg: domain.GeneratorRecipeConfig{Version: 9, Recipes: []domain.GeneratorRecipe{rec}}})

	v, err := s.GeneratorRecipeVersion(context.Background(), &dbv1.GeneratorRecipeVersionRequest{})
	if err != nil || v.GetVersion() != 9 {
		t.Fatalf("versão = %v, %v; quero 9", v.GetVersion(), err)
	}
	resp, err := s.GetGeneratorRecipes(context.Background(), &dbv1.GetGeneratorRecipesRequest{})
	if err != nil {
		t.Fatalf("GetGeneratorRecipes: %v", err)
	}
	if resp.GetVersion() != 9 || len(resp.GetRecipes()) != 1 {
		t.Fatalf("resp = %+v", resp)
	}
	got := resp.GetRecipes()[0]
	// Every field crosses: a waypoint lost here would put the zone somewhere
	// else with nothing in any log.
	if got.GetIndex() != 20000 || got.GetLeader() != "Lobo" || got.GetFollower() != "Lobo_" ||
		got.GetMinuteGenerate() != 2 || got.GetMinGroup() != 1 || got.GetMaxGroup() != 3 ||
		got.GetMaxNumMob() != 8 || got.GetRouteType() != 1 || got.GetFormation() != 2 || got.GetRenovar() != 5 {
		t.Errorf("receita = %+v", got)
	}
	if len(got.GetSegX()) != 5 || got.GetSegX()[4] != 2110 || got.GetSegY()[4] != 2090 ||
		got.GetSegRange()[0] != 4 || got.GetSegWait()[0] != -1 || got.GetSegWait()[4] != 3 {
		t.Errorf("pontos = x%v y%v r%v w%v", got.GetSegX(), got.GetSegY(), got.GetSegRange(), got.GetSegWait())
	}
}

func TestNpcRecipeServerFalhaDoBancoEInternal(t *testing.T) {
	s := NewNpcRecipe(&fakeRecipeStore{err: errors.New("banco fora")})
	if _, err := s.GetGeneratorRecipes(context.Background(), &dbv1.GetGeneratorRecipesRequest{}); status.Code(err) != codes.Internal {
		t.Errorf("GetGeneratorRecipes: %v, quero Internal", err)
	}
	if _, err := s.GeneratorRecipeVersion(context.Background(), &dbv1.GeneratorRecipeVersionRequest{}); status.Code(err) != codes.Internal {
		t.Errorf("GeneratorRecipeVersion: %v, quero Internal", err)
	}
}
