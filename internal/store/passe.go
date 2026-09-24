package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// PasseNivelMax é o maior nível que existe: o cliente sabe desenhar quatro
// molduras, e o zero é "sem passe".
const PasseNivelMax = 4

// ErrPasseNivelInvalido é o nível fora da faixa. Recusa prevista, e não falha: quem
// chama traduz numa mensagem de tela.
var ErrPasseNivelInvalido = errors.New("store: nivel de passe fora de 0..4")

// DefinirPasseNivel grava o nível do passe da conta.
//
// NA CONTA, e não no personagem: a pessoa compra o passe uma vez e ele vale para
// todos os personagens dela. É também o que casa com o caminho de depois, em que o
// site dá o passe sozinho quando a doação for confirmada — e doação é por conta.
//
// A FAIXA É CONFERIDA AQUI ALÉM DO CHECK DO BANCO, e a repetição é de propósito: o
// CHECK devolveria um erro de infraestrutura para o que na verdade é um pedido
// inválido, e quem chama não teria como dizer "esse nível não existe" em vez de
// "erro ao gravar".
//
// ELA NÃO AVISA O JOGO. Quem está em jogo continua com a moldura antiga até alguém
// reanunciar a entidade — isso é trabalho de quem chama, pelo link de controle, e
// está separado porque o banco é a verdade e o jogo é a cortesia. Se o aviso falhar,
// o nível continua gravado e vale no próximo login.
func (s *Store) DefinirPasseNivel(ctx context.Context, accountID int64, nivel int16) error {
	if nivel < 0 || nivel > PasseNivelMax {
		return ErrPasseNivelInvalido
	}
	tag, err := s.pool.Exec(ctx,
		`UPDATE account SET passe_nivel = $2 WHERE id = $1`, accountID, nivel)
	if err != nil {
		return fmt.Errorf("store: definir passe a=%d: %w", accountID, err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// PasseNivel lê o nível da conta.
func (s *Store) PasseNivel(ctx context.Context, accountID int64) (int16, error) {
	var n int16
	err := s.pool.QueryRow(ctx,
		`SELECT passe_nivel FROM account WHERE id = $1`, accountID).Scan(&n)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, ErrNotFound
		}
		return 0, fmt.Errorf("store: lendo o passe a=%d: %w", accountID, err)
	}
	return n, nil
}
