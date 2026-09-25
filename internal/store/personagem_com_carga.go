package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/jeanluca/w2pp-openwyd/internal/domain"
)

// ErrParVelho é a recusa de gravar um par mais antigo que o já gravado.
//
// NÃO É ERRO DE NINGUÉM: é uma corrida perdida, e perder é o comportamento certo.
// Quem chama trata como "já tem coisa melhor no banco" e segue; transformar isto
// em falha visível faria o jogo avisar o jogador de uma coisa que não é problema
// dele.
var ErrParVelho = errors.New("store: ha par mais novo gravado; este nao entra")

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
	cargoCoin int32, cargoItems []domain.Item, deliveredIDs, lostIDs []int64, epoca, seq int64,
) error {
	return s.inTx(ctx, func(tx pgx.Tx) error {
		if err := tomaAOrdemDoPar(ctx, tx, accountID, epoca, seq); err != nil {
			return err
		}
		if err := salvarPersonagemTx(ctx, tx, accountID, ch); err != nil {
			return err
		}
		return salvarCargaTx(ctx, tx, accountID, cargoCoin, cargoItems, deliveredIDs, lostIDs)
	})
}

// tomaAOrdemDoPar prende a conta e recusa o par que já nasceu velho.
//
// A CONFERÊNCIA É A PRIMEIRA COISA DA TRANSAÇÃO, e não uma consulta antes dela: o
// UPDATE tranca a linha da conta, então dois pares da mesma conta passam por aqui
// um de cada vez, e o segundo lê o número que o primeiro gravou. Conferir fora da
// transação seria trocar uma corrida por outra.
//
// A ÉPOCA GANHA DO NÚMERO: um processo novo tem época maior e escreve por cima de
// tudo o que o anterior deixou, que é o certo — as gravações em voo do processo
// morto não existem mais. Dentro de uma época, o número ordena.
//
// epoca == 0 desliga a guarda, e é o que os caminhos sem numeração usam.
func tomaAOrdemDoPar(ctx context.Context, tx pgx.Tx, accountID, epoca, seq int64) error {
	if epoca <= 0 {
		return nil
	}
	tag, err := tx.Exec(ctx, `
		UPDATE account SET par_epoca = $2, par_seq = $3
		 WHERE id = $1 AND (par_epoca, par_seq) < ($2, $3)`, accountID, epoca, seq)
	if err != nil {
		return fmt.Errorf("store: ordem do par a=%d: %w", accountID, err)
	}
	if tag.RowsAffected() == 1 {
		return nil
	}
	// Nenhuma linha: ou a conta não existe, ou o número já foi passado. São coisas
	// diferentes e quem chama trata cada uma de um jeito.
	var existe bool
	if err := tx.QueryRow(ctx, `SELECT true FROM account WHERE id = $1`, accountID).Scan(&existe); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		return fmt.Errorf("store: conferindo a conta do par a=%d: %w", accountID, err)
	}
	return ErrParVelho
}

// NovaEpocaDePar pega o número desta execução do tmServer na sequência do banco.
//
// É chamada UMA vez, no boot. A sequência é do banco de propósito: ela é
// monotônica sem depender do relógio da máquina, e é isso que impede o travamento
// silencioso em que um contador zerado no reinício fica abaixo do que o banco
// guardou e nenhuma gravação passa mais.
func (s *Store) NovaEpocaDePar(ctx context.Context) (int64, error) {
	var n int64
	if err := s.pool.QueryRow(ctx, `SELECT nextval('par_epoca_seq')`).Scan(&n); err != nil {
		return 0, fmt.Errorf("store: nova epoca de par: %w", err)
	}
	return n, nil
}
