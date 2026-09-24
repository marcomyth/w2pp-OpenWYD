package store

import (
	"context"
	"fmt"
	"time"
)

// PagamentoOrfao é dinheiro que entrou e não achou cobrança.
//
// Os três campos de valor são ponteiros porque o primeiro aviso pode chegar antes
// de a processadora ter respondido tudo. Registrar incompleto é melhor do que não
// registrar: o que não pode faltar é o identifier, que é por onde uma pessoa acha o
// pagamento lá.
type PagamentoOrfao struct {
	ID              int64
	Identifier      string
	ValorCentavos   *int64
	PagoEm          *time.Time
	ReferenciaVista string
	Motivo          string
	VistoEm         time.Time
}

// Motivos, em texto e não enum de banco, porque o conjunto ainda está crescendo e
// um enum errado esconde o caso novo dentro do "outro".
const (
	// MotivoOrfaoSemCobranca: nem o identifier nem a referência que a processadora
	// devolveu achou cobrança nossa. Pode ser tráfego da loja de doação (o normal),
	// pode ser aviso forjado.
	MotivoOrfaoSemCobranca = "sem cobranca para o identifier nem para a referencia"
	// MotivoOrfaoReferenciaDivergente: o identifier aponta para uma cobrança nossa
	// e a processadora devolveu a referência de OUTRA. Ninguém entrega nada aqui: é
	// erro de alguém, e confirmar pelo palpite errado dá item ao comprador errado.
	MotivoOrfaoReferenciaDivergente = "a referencia da processadora nao bate com a gravada"
	// MotivoOrfaoContestado: a venda entrou em contestação do Pix (MED) ou foi
	// estornada. O dinheiro existiu e está indo embora, e ninguém entrega item.
	MotivoOrfaoContestado = "pagamento contestado ou estornado na processadora"
)

// RegistrarPagamentoOrfao põe o pagamento na fila da staff.
//
// POR QUE ISTO NÃO É UM LOG, que era o que o caminho do aviso fazia antes: um log
// é rotacionado, não tem fila, não tem dono e não dá para marcar como resolvido. O
// que sobrava dele era alguém ter pagado e o servidor ter escrito uma linha que
// ninguém lê.
//
// REPETIÇÃO NÃO CRIA LINHA NOVA, e é o índice único sobre o identifier que garante
// isso. A processadora avisa mais de uma vez sobre o mesmo pagamento; sem isto, um
// webhook repetido viraria dez linhas sobre um problema só, e quem olhasse a fila
// contaria dez.
//
// O que a repetição faz é COMPLETAR o que estava nulo. O primeiro aviso pode chegar
// sem valor e sem a hora; o segundo, já com eles. Por isso o preenchimento é com
// coalesce do valor NOVO sobre o gravado: quem tem dado ganha de quem não tem, e um
// nulo que chega depois nunca apaga um valor que já estava lá.
//
// E NÃO reabre o que uma pessoa já resolveu: um aviso atrasado chegando depois da
// resolução não deve pôr a linha de volta na fila.
func (s *Store) RegistrarPagamentoOrfao(ctx context.Context, p PagamentoOrfao) error {
	if p.Identifier == "" {
		// Sem identifier não há o que registrar nem por onde procurar. Erro, e não
		// silêncio: o caminho que chegou aqui sem identifier tem um defeito.
		return fmt.Errorf("store: pagamento orfao sem identifier")
	}
	_, err := s.pool.Exec(ctx, `
		INSERT INTO rmt_pagamento_orfao
			(identifier, valor_centavos, pago_em, referencia_vista, motivo)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (identifier) DO UPDATE SET
			valor_centavos   = coalesce(EXCLUDED.valor_centavos, rmt_pagamento_orfao.valor_centavos),
			pago_em          = coalesce(EXCLUDED.pago_em, rmt_pagamento_orfao.pago_em),
			referencia_vista = coalesce(nullif(EXCLUDED.referencia_vista, ''), rmt_pagamento_orfao.referencia_vista),
			motivo           = EXCLUDED.motivo`,
		p.Identifier, p.ValorCentavos, p.PagoEm, p.ReferenciaVista, p.Motivo)
	if err != nil {
		return fmt.Errorf("store: registrando o pagamento orfao %q: %w", p.Identifier, err)
	}
	return nil
}

// PagamentosOrfaos lista o que ainda precisa de gente, mais antigo primeiro.
func (s *Store) PagamentosOrfaos(ctx context.Context) ([]PagamentoOrfao, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, identifier, valor_centavos, pago_em,
		       coalesce(referencia_vista, ''), motivo, visto_em
		  FROM rmt_pagamento_orfao
		 WHERE resolvido_em IS NULL
		 ORDER BY visto_em, id`)
	if err != nil {
		return nil, fmt.Errorf("store: pagamentos orfaos: %w", err)
	}
	defer rows.Close()
	var fila []PagamentoOrfao
	for rows.Next() {
		var p PagamentoOrfao
		if err := rows.Scan(&p.ID, &p.Identifier, &p.ValorCentavos, &p.PagoEm,
			&p.ReferenciaVista, &p.Motivo, &p.VistoEm); err != nil {
			return nil, fmt.Errorf("store: pagamentos orfaos: %w", err)
		}
		fila = append(fila, p)
	}
	return fila, rows.Err()
}

// ResolverPagamentoOrfao tira da fila o que uma pessoa já tratou.
//
// Só a MARCA, e nunca o dinheiro: o que fazer com o pagamento — devolver, achar a
// venda na mão, deixar como está — é decisão de quem manda, e ela é executada por
// fora. Esta função só registra que foi decidido, por quem, e o que foi escrito.
func (s *Store) ResolverPagamentoOrfao(ctx context.Context, id int64, por, nota string) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE rmt_pagamento_orfao
		   SET resolvido_em = now(), resolvido_por = $2, nota = $3
		 WHERE id = $1 AND resolvido_em IS NULL`, id, por, nota)
	if err != nil {
		return fmt.Errorf("store: resolvendo o pagamento orfao %d: %w", id, err)
	}
	return nil
}
