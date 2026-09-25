package dbclient

import (
	"context"
	"fmt"

	"google.golang.org/grpc"

	dbv1 "github.com/jeanluca/w2pp-openwyd/api/db/v1"
	"github.com/jeanluca/w2pp-openwyd/internal/domain"
)

// GeneratorRecipeSource reads the block recipes on dbServer
// (0165_receita_de_bloco). Polled like the block switches.
type GeneratorRecipeSource struct {
	api dbv1.NpcRecipeServiceClient
}

// NewGeneratorRecipeSource wraps a gRPC connection.
func NewGeneratorRecipeSource(conn grpc.ClientConnInterface) *GeneratorRecipeSource {
	return &GeneratorRecipeSource{api: dbv1.NewNpcRecipeServiceClient(conn)}
}

// Version is the poll.
func (c *GeneratorRecipeSource) Version(ctx context.Context) (int64, error) {
	resp, err := c.api.GeneratorRecipeVersion(ctx, &dbv1.GeneratorRecipeVersionRequest{})
	if err != nil {
		return 0, fmt.Errorf("dbclient: generator recipe version: %w", err)
	}
	return resp.GetVersion(), nil
}

// Snapshot fetches every block with a recipe in the database.
func (c *GeneratorRecipeSource) Snapshot(ctx context.Context) (domain.GeneratorRecipeConfig, error) {
	resp, err := c.api.GetGeneratorRecipes(ctx, &dbv1.GetGeneratorRecipesRequest{})
	if err != nil {
		return domain.GeneratorRecipeConfig{}, fmt.Errorf("dbclient: get generator recipes: %w", err)
	}
	cfg := domain.GeneratorRecipeConfig{Version: resp.GetVersion()}
	for _, r := range resp.GetRecipes() {
		rec := domain.GeneratorRecipe{
			Index: r.GetIndex(), Leader: r.GetLeader(), Follower: r.GetFollower(),
			MinuteGenerate: r.GetMinuteGenerate(), MinGroup: r.GetMinGroup(), MaxGroup: r.GetMaxGroup(),
			MaxNumMob: r.GetMaxNumMob(), RouteType: r.GetRouteType(), Formation: r.GetFormation(),
			Renovar: r.GetRenovar(),
		}
		copy(rec.SegX[:], r.GetSegX())
		copy(rec.SegY[:], r.GetSegY())
		copy(rec.SegRange[:], r.GetSegRange())
		copy(rec.SegWait[:], r.GetSegWait())
		cfg.Recipes = append(cfg.Recipes, rec)
	}
	return cfg, nil
}
