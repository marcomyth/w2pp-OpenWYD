//go:build integration

package store

import (
	"context"
	"testing"

	"github.com/jeanluca/w2pp-openwyd/internal/domain"
)

// TestGetNPCDefinitionLePrecoEmPontos: a leitura de UM NPC traz o preço em pontos
// de cada vaga. É a leitura que o painel faz antes de gravar uma vaga, e ele grava
// a loja inteira de volta — um preço que não viesse aqui seria apagado de todas
// as outras vagas na primeira edição. Foi o que a Loja de Honra, com o estoque no
// banco desde 26/09/2026, teria sofrido.
func TestGetNPCDefinitionLePrecoEmPontos(t *testing.T) {
	ctx := context.Background()
	pool := testPool(t)
	if err := Migrate(ctx, pool); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	_, _ = pool.Exec(ctx, `DELETE FROM npc_audit; DELETE FROM npc_shop_item; DELETE FROM npc_definition`)
	s := New(pool)

	var modID int64
	if err := pool.QueryRow(ctx,
		`INSERT INTO account (name, pass_hash, role) VALUES ('mod_npc_pontos','x','moderator') RETURNING id`).
		Scan(&modID); err != nil {
		t.Fatalf("seed moderator: %v", err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(ctx, `DELETE FROM account WHERE id = $1`, modID) })

	id, err := s.UpsertNPCDefinition(ctx, domain.NPCDefinition{
		Slug: "honra-int-1", TemplateName: "God_of_War", DisplayName: "Honor Store",
		Enabled: true, PosX: 2130, PosY: 2088, Merchant: 104,
	}, modID)
	if err != nil {
		t.Fatalf("upsert: %v", err)
	}
	cem := int32(100)
	if err := s.SetNPCShop(ctx, id, []domain.NPCShopItem{
		{Slot: 0, ItemIndex: 413, Quantity: 1, PricePoints: &cem},
		{Slot: 1, ItemIndex: 1100, Quantity: 1}, // em ouro
	}, modID); err != nil {
		t.Fatalf("set shop: %v", err)
	}

	def, err := s.GetNPCDefinition(ctx, id)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if len(def.Shop) != 2 {
		t.Fatalf("loja com %d vagas, quer 2: %+v", len(def.Shop), def.Shop)
	}
	if p := def.Shop[0].PricePoints; p == nil || *p != 100 {
		t.Errorf("vaga 0 voltou com preço em pontos %v, quer 100", p)
	}
	if p := def.Shop[1].PricePoints; p != nil {
		t.Errorf("vaga 1, em ouro, voltou com preço em pontos %d", *p)
	}
}
