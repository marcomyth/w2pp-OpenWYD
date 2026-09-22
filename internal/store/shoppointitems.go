package store

import (
	"context"
	"fmt"
)

// ItemEmPontos é um item que alguma loja de NPC vende por pontos de lojinha
// (0078_npc_shop_price_points), com o que se precisa saber para escrever o
// aviso no cliente.
type ItemEmPontos struct {
	ItemIndex int32
	// Precos são os preços distintos em pontos, em ordem crescente. Mais de um
	// quer dizer que duas lojas cobram valores diferentes pelo mesmo item — o
	// aviso no cliente é por ITEM, então ele não pode citar número nenhum.
	Precos []int32
	// TambemEmOuro é verdadeiro quando o mesmo item também está à venda por
	// OURO em alguma prateleira. O texto do itemhelp vale para o item em todo
	// lugar do jogo, então marcar um item destes diz "vendido por pontos"
	// inclusive para quem o está comprando com moeda.
	TambemEmOuro bool
}

// ItensEmPontos lista os itens que as lojas de NPC vendem por pontos, em ordem
// de índice.
//
// Serve ao gerador do cliente (cmd/lojapontoscliente): o texto do tooltip é
// estático e por item, então ele precisa saber quais itens marcar e, mais
// importante, quais marcaria errado.
func (s *Store) ItensEmPontos(ctx context.Context) ([]ItemEmPontos, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT item_index,
		       array_agg(DISTINCT price_points ORDER BY price_points)
		           FILTER (WHERE price_points IS NOT NULL),
		       bool_or(price_points IS NULL)
		FROM npc_shop_item
		GROUP BY item_index
		HAVING bool_or(price_points IS NOT NULL)
		ORDER BY item_index`)
	if err != nil {
		return nil, fmt.Errorf("store: listar itens vendidos em pontos: %w", err)
	}
	defer rows.Close()

	var out []ItemEmPontos
	for rows.Next() {
		var it ItemEmPontos
		if err := rows.Scan(&it.ItemIndex, &it.Precos, &it.TambemEmOuro); err != nil {
			return nil, fmt.Errorf("store: ler item vendido em pontos: %w", err)
		}
		out = append(out, it)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterar itens vendidos em pontos: %w", err)
	}
	return out, nil
}
