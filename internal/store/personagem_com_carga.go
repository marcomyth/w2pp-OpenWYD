package store

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/jeanluca/w2pp-openwyd/internal/domain"
)

// SalvarPersonagemComCarga grava o personagem E a carga da conta na MESMA
// transação, e marca junto as entregas que vieram com essa carga.
//
// POR QUE ISTO EXISTE: o ouro e os itens andam entre a mochila do personagem e a
// carga da conta, e as duas metades moram em lugares diferentes do banco. Enquanto
// eram duas gravações, uma queda entre elas deixava o mesmo ouro nos DOIS lados —
// o personagem já sem o que depositou, a carga ainda sem receber, ou o contrário,
// que é pior: o personagem já COM o que sacou e a carga ainda com ele. No próximo
// login o mesmo item existia duas vezes. A janela era pequena e real; foi medida.
//
// A cura não é ordenar melhor as duas gravações — não existe ordem segura entre
// duas transações — e sim não ter duas. Enquanto a conta tem personagem em jogo,
// personagem e carga vão ao banco juntos, do MESMO instantâneo, aqui.
//
// A marca das entregas entra na mesma transação pela mesma razão, que já valia
// para o SaveCargoWithDeliveries: uma queda antes do commit deixa as linhas em
// 'pending' e a carga como estava, e o próximo login entrega uma vez só.
func (s *Store) SalvarPersonagemComCarga(ctx context.Context, accountID int64, ch domain.Character,
	cargoCoin int32, cargoItems []domain.Item, deliveredIDs, lostIDs []int64,
) error {
	return s.inTx(ctx, func(tx pgx.Tx) error {
		if err := salvarPersonagemTx(ctx, tx, accountID, ch); err != nil {
			return err
		}
		return salvarCargaTx(ctx, tx, accountID, cargoCoin, cargoItems, deliveredIDs, lostIDs)
	})
}
