package grpcsrv

import (
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	dbv1 "github.com/jeanluca/w2pp-openwyd/api/db/v1"
	"github.com/jeanluca/w2pp-openwyd/internal/domain"
)

// GeneratorRecipeStore is what the block recipes need (satisfied by
// *store.Store). Read-only: the panel writes the table directly.
type GeneratorRecipeStore interface {
	GeneratorRecipeVersion(ctx context.Context) (int64, error)
	GeneratorRecipes(ctx context.Context) (domain.GeneratorRecipeConfig, error)
}

// NpcRecipeServer implements dbv1.NpcRecipeServiceServer.
type NpcRecipeServer struct {
	dbv1.UnimplementedNpcRecipeServiceServer
	store GeneratorRecipeStore
}

// NewNpcRecipe builds the service over the given store.
func NewNpcRecipe(s GeneratorRecipeStore) *NpcRecipeServer { return &NpcRecipeServer{store: s} }

// GeneratorRecipeVersion is the poll tmServer runs every few seconds.
func (s *NpcRecipeServer) GeneratorRecipeVersion(ctx context.Context, _ *dbv1.GeneratorRecipeVersionRequest) (*dbv1.GeneratorRecipeVersionResponse, error) {
	v, err := s.store.GeneratorRecipeVersion(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "generator recipe version: %v", err)
	}
	return &dbv1.GeneratorRecipeVersionResponse{Version: v}, nil
}

// GetGeneratorRecipes returns every block with a recipe and the version.
func (s *NpcRecipeServer) GetGeneratorRecipes(ctx context.Context, _ *dbv1.GetGeneratorRecipesRequest) (*dbv1.GetGeneratorRecipesResponse, error) {
	cfg, err := s.store.GeneratorRecipes(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "generator recipes: %v", err)
	}
	resp := &dbv1.GetGeneratorRecipesResponse{
		Version: cfg.Version, Recipes: make([]*dbv1.GeneratorRecipe, 0, len(cfg.Recipes)),
	}
	for _, r := range cfg.Recipes {
		resp.Recipes = append(resp.Recipes, &dbv1.GeneratorRecipe{
			Index: r.Index, Leader: r.Leader, Follower: r.Follower,
			MinuteGenerate: r.MinuteGenerate, MinGroup: r.MinGroup, MaxGroup: r.MaxGroup,
			MaxNumMob: r.MaxNumMob, RouteType: r.RouteType, Formation: r.Formation,
			SegX: r.SegX[:], SegY: r.SegY[:], SegRange: r.SegRange[:], SegWait: r.SegWait[:],
			Renovar: r.Renovar,
		})
	}
	return resp, nil
}
