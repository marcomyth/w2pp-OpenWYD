package store

import (
	"context"
	"fmt"
)

// PrateleiraOcupada é uma prateleira de dinheiro real que já tem alguém pagando.
type PrateleiraOcupada struct {
	VendedorConta int64
	CargoSlot     int16
}

// PrateleirasComCobrancaAberta lista as prateleiras que não podem ser compradas
// agora, porque um comprador já abriu a cobrança delas.
//
// POR QUE ISTO EXISTE FORA DO JOGO: a barraca vive na memória do servidor de jogo,
// e a cobrança vive aqui. Nenhum dos dois lados sabe sozinho que a prateleira está
// ocupada — o jogo não fala com o banco, e o banco não enxerga a barraca. Quem junta
// os dois é o web-api, que tem o link com o jogo e a conexão com o banco.
//
// A CHAVE É (conta do vendedor, slot do baú), e não o id do anúncio, porque é o que
// os dois lados têm em comum: o jogo conhece o slot de onde a prateleira vende, e o
// anúncio guarda os dois. O id do anúncio o jogo não tem.
//
// Devolve o conjunto inteiro e não uma consulta por prateleira: são poucas por
// construção — no máximo uma cobrança aberta por anúncio —, e uma consulta por linha
// da vitrine seria uma ida ao banco por item numa página pública.
func (s *Store) PrateleirasComCobrancaAberta(ctx context.Context) ([]PrateleiraOcupada, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT a.vendedor_conta, a.cargo_slot
		  FROM rmt_cobranca c
		  JOIN rmt_anuncio a ON a.id = c.anuncio_id
		 WHERE c.status = $1`, cobrancaAberta)
	if err != nil {
		return nil, fmt.Errorf("store: prateleiras com cobranca aberta: %w", err)
	}
	defer rows.Close()
	var lista []PrateleiraOcupada
	for rows.Next() {
		var p PrateleiraOcupada
		if err := rows.Scan(&p.VendedorConta, &p.CargoSlot); err != nil {
			return nil, fmt.Errorf("store: prateleiras com cobranca aberta: %w", err)
		}
		lista = append(lista, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: prateleiras com cobranca aberta: %w", err)
	}
	return lista, nil
}
