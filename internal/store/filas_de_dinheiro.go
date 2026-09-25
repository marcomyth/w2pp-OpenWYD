package store

import (
	"context"
	"fmt"
)

// FilasDeDinheiro é quanto está parado em cada uma das quatro filas do dinheiro real.
type FilasDeDinheiro struct {
	Repasses    int
	Reembolsos  int
	Orfaos      int
	Divergentes int
}

// Total é o número que vai no menu, na entrada única de Dinheiro.
func (f FilasDeDinheiro) Total() int {
	return f.Repasses + f.Reembolsos + f.Orfaos + f.Divergentes
}

// ContarFilasDeDinheiro conta as quatro numa consulta só.
//
// UMA CONSULTA E NÃO QUATRO porque este número aparece no MENU, ou seja, em toda página
// que qualquer pessoa da equipe abrir. Quatro idas ao banco por clique, para um número
// que é decoração na maior parte do tempo, é caro pelo motivo errado.
//
// CADA CONTAGEM ESPELHA A CONSULTA DA SUA TELA, condição por condição, e isso é o ponto
// mais delicado deste arquivo. Um contador que diverge da lista é pior do que não ter
// contador: ele manda a pessoa abrir uma tela que está vazia, ou — muito pior — diz zero
// enquanto há dinheiro parado, e aí ninguém abre. As quatro origens estão citadas abaixo
// para quem mexer numa delas saber que tem de mexer aqui.
//
// Se alguém mudar o que uma tela mostra e esquecer deste arquivo, o teste de integração
// compara os dois lados e quebra.
func (s *Store) ContarFilasDeDinheiro(ctx context.Context) (FilasDeDinheiro, error) {
	var f FilasDeDinheiro
	err := s.pool.QueryRow(ctx, `
		SELECT
		  -- Repasses: espelha RepassesQuePrecisamDeGente (repasse_rmt.go). São os dois
		  -- estados de gente MAIS as pendentes sem destino, que o LEFT JOIN acha e que
		  -- sumiriam num JOIN normal — foi esse o buraco que deixava vendedor esperando.
		  (SELECT count(*) FROM rmt_repasse r
		    WHERE (r.status IN ($1, $2) AND r.resolvido_em IS NULL)
		       OR (r.status = $3 AND NOT EXISTS (
		             SELECT 1 FROM rmt_recebedor d
		              WHERE d.account_id = r.vendedor_conta AND d.documento IS NOT NULL))),
		  -- Reembolsos: espelha ReembolsosRecusados (reembolso_rmt.go).
		  (SELECT count(*) FROM rmt_cobranca WHERE reembolso_status IN ($4, $5)),
		  -- Órfãos: espelha PagamentosOrfaos (pagamento_orfao.go).
		  (SELECT count(*) FROM rmt_pagamento_orfao WHERE resolvido_em IS NULL),
		  -- Divergentes: espelha ValoresDivergentes (reembolso_rmt.go).
		  (SELECT count(*) FROM rmt_cobranca
		    WHERE valor_divergente_centavos IS NOT NULL AND status = $6)`,
		repasseRecusado, repasseIncerto, repassePendente,
		reembolsoRecusado, reembolsoIncerto, cobrancaAberta).
		Scan(&f.Repasses, &f.Reembolsos, &f.Orfaos, &f.Divergentes)
	if err != nil {
		return FilasDeDinheiro{}, fmt.Errorf("store: contando as filas de dinheiro: %w", err)
	}
	return f, nil
}
