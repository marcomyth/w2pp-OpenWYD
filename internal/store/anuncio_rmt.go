package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// ErrSemChavePix é a recusa de anunciar sem ter para onde receber. Resposta
// prevista, não falha: quem chama traduz em aviso ao vendedor.
var ErrSemChavePix = errors.New("store: a conta nao tem chave pix de recebimento")

// A abertura do anúncio em dinheiro real: o começo do caminho do dinheiro.
//
// Aqui o escrow NASCE. Até este ponto ele era uma coluna e uma armadilha; a
// partir daqui existe uma linha em rmt_anuncio, com a fotografia do item, e o
// slot do baú passa a carregar o id dela.

// ItemAnunciado é uma prateleira em dinheiro real, do jeito que o jogo a montou.
type ItemAnunciado struct {
	CargoSlot     int16
	ItemIndex     int16
	Eff1, EffV1   uint8
	Eff2, EffV2   uint8
	Eff3, EffV3   uint8
	PrecoCentavos int64
}

// AbrirAnunciosRMT cria os anúncios de uma barraca, TODOS ou NENHUM.
//
// Uma transação só porque a barraca é uma coisa só para quem a monta: metade dos
// anúncios criados seria uma vitrine que promete o que não pode cumprir, e o
// vendedor não teria como saber quais metades.
//
// Devolve os ids na MESMA ORDEM dos itens recebidos, porque é assim que quem
// chamou liga cada id ao slot do baú que ele vai marcar.
//
// ELA RECUSA SEM CHAVE PIX, e a conferência acontece DENTRO desta transação em
// vez de numa chamada antes. Duas chamadas deixariam uma fresta — a chave apagada
// entre a pergunta e a criação — e, pior, espalhariam a regra por dois lugares.
// Aqui não há fresta: ou a chave está lá quando os anúncios nascem, ou eles não
// nascem.
//
// E a recusa é na hora de ANUNCIAR, não na hora de pagar. Ao anunciar, o vendedor
// está olhando para a tela e pode resolver; na hora de pagar, quem está com o QR
// aberto é o COMPRADOR, e o único que poderia resolver foi embora.
//
// O QUE ESTA FUNÇÃO NÃO FAZ: pôr a marca no item. A marca vive no baú vivo, que é
// do laço do tmServer, e escrever nela daqui seria escrever por cima do dono. O
// laço põe a marca com os ids que voltam daqui e salva.
//
// A JANELA QUE ISSO ABRE, dita em voz alta: entre esta transação e o save do baú
// existe um instante em que o anúncio está ATIVO e o item não está marcado. Se o
// servidor cair exatamente ali, sobra um anúncio ativo sem escrow. Ele não vende
// nada — a compra confere a marca — mas fica na vitrine. Quem varre isso é a
// reconciliação do anúncio órfão, que ainda não existe e está anotada.
func (s *Store) AbrirAnunciosRMT(ctx context.Context, vendedorConta int64, itens []ItemAnunciado) ([]int64, error) {
	if len(itens) == 0 {
		return nil, nil
	}
	ids := make([]int64, len(itens))
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		var temChave bool
		if err := tx.QueryRow(ctx,
			`SELECT EXISTS(SELECT 1 FROM rmt_recebedor WHERE account_id = $1 FOR UPDATE)`,
			vendedorConta).Scan(&temChave); err != nil {
			return fmt.Errorf("store: abrir anuncio: conferindo a chave a=%d: %w", vendedorConta, err)
		}
		if !temChave {
			return ErrSemChavePix
		}
		for i, it := range itens {
			if err := tx.QueryRow(ctx, `
				INSERT INTO rmt_anuncio
					(vendedor_conta, cargo_slot, item_index,
					 eff1, effv1, eff2, effv2, eff3, effv3, preco_centavos, status)
				VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
				RETURNING id`,
				vendedorConta, it.CargoSlot, it.ItemIndex,
				it.Eff1, it.EffV1, it.Eff2, it.EffV2, it.Eff3, it.EffV3,
				it.PrecoCentavos, anuncioAtivo).Scan(&ids[i]); err != nil {
				return fmt.Errorf("store: abrir anuncio a=%d slot=%d: %w", vendedorConta, it.CargoSlot, err)
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return ids, nil
}

// CancelarAnunciosRMT fecha anúncios que não chegaram a valer.
//
// É a ação de compensação da janela descrita acima: se a barraca não subir depois
// de os anúncios nascerem — a sessão caiu, o baú mudou entre a ida ao banco e a
// volta ao laço —, eles têm de sair da vitrine. Sem isso ficariam ativos para
// sempre, prendendo o slot pelo índice único de um ativo por slot.
//
// Só fecha o que ainda está ATIVO: um anúncio já vendido no meio-tempo não pode
// ser cancelado por um caminho de desistência, e o WHERE é quem garante isso — não
// a ordem em que as coisas acontecem.
func (s *Store) CancelarAnunciosRMT(ctx context.Context, ids []int64) error {
	if len(ids) == 0 {
		return nil
	}
	if _, err := s.pool.Exec(ctx, `
		UPDATE rmt_anuncio SET status = $2, encerrado_em = now()
		 WHERE id = ANY($1) AND status = $3`, ids, anuncioCancelado, anuncioAtivo); err != nil {
		return fmt.Errorf("store: cancelar anuncios %v: %w", ids, err)
	}
	return nil
}
