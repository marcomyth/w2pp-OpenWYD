package donate

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode"

	"github.com/jackc/pgx/v5"

	"github.com/jeanluca/w2pp-openwyd/adminserver/internal/entrega"
	"github.com/jeanluca/w2pp-openwyd/internal/pilha"
)

// O envio de pacote de apoiador pelo painel: o mesmo pacote que o site vende,
// entregue sem pagamento, para quem a equipe decidir (influencer, parceria,
// compensação).
//
// É o pacote COMPLETO — os Rcoins e os brindes —, lidos das mesmas tabelas que a
// confirmação do pagamento lê (donate_pacote e donate_pacote_item, 0122/0123).
// Uma lista redigitada aqui entregaria o pacote que alguém lembrou, e não o que o
// site promete.
//
// NÃO É UMA VENDA, e por isso não passa por donate_topup_order: aquela tabela é o
// dinheiro que entrou, e um pacote dado ali inflaria a receita e os repasses. O
// crédito vai como 'credit_balance', que é exatamente o que o painel de receita
// soma como "a equipe deu donate".

var (
	// ErrPacoteDesconhecido é um id que não está em donate_pacote.
	ErrPacoteDesconhecido = errors.New("donate: pacote desconhecido")
	// ErrPacoteIndisponivel é um pacote desligado ou reservado à staff. Os
	// espelhos de teste ("teste-*") existem para testar pagamento, e dar um deles
	// a alguém entregaria o conteúdo de um pacote real com o nome de um teste no
	// registro.
	ErrPacoteIndisponivel = errors.New("donate: pacote fora de venda")
	// ErrPacoteRepetido é o mesmo pacote para a mesma conta há menos de
	// JanelaDeRepeticao. Quase sempre é o botão clicado duas vezes, e o custo de
	// errar para o outro lado é um Supremo a mais dado de graça.
	ErrPacoteRepetido = errors.New("donate: pacote enviado agora há pouco")
)

// JanelaDeRepeticao é o intervalo em que um segundo envio igual é recusado.
// Quem quer mesmo dar dois pacotes espera um minuto; um clique duplo não espera.
const JanelaDeRepeticao = "1 minute"

// Pacote é um pacote de apoiador como o painel o mostra e o envia.
type Pacote struct {
	ID       string
	Creditos int32
	Brindes  []Brinde
}

// Nome é o id legível: "apoiador-supremo" vira "Apoiador Supremo".
func (p Pacote) Nome() string { return NomeDoPacote(p.ID) }

// Espacos é quantos espaços do baú da conta os brindes ocupam, pela mesma regra
// de empilhar que a entrega usa.
func (p Pacote) Espacos() int {
	total := 0
	for _, b := range p.Brindes {
		total += len(pilha.Divide(int16(b.Index), b.Quantidade))
	}
	return total
}

// Brinde é um item do pacote com a quantidade separada dos efeitos.
//
// Na tabela a quantidade é o EF_AMOUNT dentro dos efeitos; aqui ela sai de lá,
// porque quem reparte em pilhas é o entrega.Lote, e ele recusa um EF_AMOUNT
// escrito à mão (escreve o dele, pilha por pilha).
type Brinde struct {
	Index      int32
	Quantidade int
	Eff        [3][2]uint8
}

// NomeDoPacote transforma o id do site em nome de tela.
func NomeDoPacote(id string) string {
	partes := strings.Split(id, "-")
	for i, p := range partes {
		if p == "" {
			continue
		}
		r := []rune(p)
		r[0] = unicode.ToUpper(r[0])
		partes[i] = string(r)
	}
	return strings.Join(partes, " ")
}

// Pacotes lista os pacotes que o painel pode enviar: ativos e à venda para
// qualquer conta, na ordem do menor para o maior.
//
// A ordem é pelos créditos e não pelo preço, porque a 0158 pôs todos a R$ 1,00
// e o preço deixou de dizer qual pacote é maior.
func (s *Store) Pacotes(ctx context.Context) ([]Pacote, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT p.id, p.credits,
		       i.item_index, i.eff1, i.effv1, i.eff2, i.effv2, i.eff3, i.effv3
		  FROM donate_pacote p
		  LEFT JOIN donate_pacote_item i ON i.pacote_id = p.id
		 WHERE p.ativo AND NOT p.so_staff
		 ORDER BY p.credits, p.id, i.ordem, i.id`)
	if err != nil {
		return nil, fmt.Errorf("donate: listar pacotes: %w", err)
	}
	defer rows.Close()

	var out []Pacote
	for rows.Next() {
		var (
			id       string
			creditos int32
			index    *int32
			e        [6]*int16
		)
		if err := rows.Scan(&id, &creditos, &index, &e[0], &e[1], &e[2], &e[3], &e[4], &e[5]); err != nil {
			return nil, fmt.Errorf("donate: listar pacotes: scan: %w", err)
		}
		if len(out) == 0 || out[len(out)-1].ID != id {
			out = append(out, Pacote{ID: id, Creditos: creditos})
		}
		if index != nil {
			p := &out[len(out)-1]
			p.Brindes = append(p.Brindes, brindeDaLinha(*index, e))
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("donate: listar pacotes: rows: %w", err)
	}
	return out, nil
}

// brindeDaLinha separa o EF_AMOUNT dos outros efeitos. Sem EF_AMOUNT a
// quantidade é 1: montaria, fada e poção não carregam quantidade, e ler zero
// como zero faria o pacote entregar nada.
func brindeDaLinha(index int32, e [6]*int16) Brinde {
	b := Brinde{Index: index, Quantidade: 1}
	livre := 0
	for i := 0; i < 3; i++ {
		ef, vl := valor(e[2*i]), valor(e[2*i+1])
		if ef == pilha.EfAmount {
			if vl > 0 {
				b.Quantidade = int(vl)
			}
			continue
		}
		if ef == 0 {
			continue
		}
		b.Eff[livre] = [2]uint8{ef, vl}
		livre++
	}
	return b
}

func valor(v *int16) uint8 {
	if v == nil {
		return 0
	}
	return uint8(*v)
}

// Envio é o que um envio de pacote fez.
type Envio struct {
	Pacote   Pacote
	Saldo    int32   // o saldo de Rcoins da conta depois do crédito
	Entregas []int64 // as linhas da delivery_queue, na ordem
}

// EnviarPacote dá o pacote inteiro à conta: credita os Rcoins e enfileira os
// brindes, NA MESMA TRANSAÇÃO.
//
// Pela mesma razão da confirmação de pagamento (internal/store/donate_topup.go):
// fora dela, uma queda no meio deixa os Rcoins creditados e os brindes não, e
// ninguém sabe o que faltou. Um envio pela metade é pior do que nenhum, porque
// quem clicar de novo dá os Rcoins duas vezes.
//
// Os brindes vão sem prazo absoluto, como os da compra: a duração é o EF_WDAY
// não iniciado e começa no primeiro uso.
func (s *Store) EnviarPacote(ctx context.Context, actorID, accountID int64, pacoteID, motivo string) (Envio, error) {
	motivo = strings.TrimSpace(motivo)
	if motivo == "" {
		return Envio{}, ErrMotivoVazio
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Envio{}, fmt.Errorf("donate: enviar pacote: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }() // no-op depois do commit

	p, err := pacoteNaTransacao(ctx, tx, pacoteID)
	if err != nil {
		return Envio{}, err
	}

	// O lote inteiro é montado ANTES de qualquer escrita: um brinde que não
	// reparte (quantidade fora do baú) tem de recusar o envio, e não ser
	// descoberto com os Rcoins já creditados — o rollback desfaria, mas a
	// mensagem de erro seria a do meio do caminho.
	var lote []entrega.Item
	for _, b := range p.Brindes {
		partes, err := entrega.Lote(entrega.Item{Index: b.Index, Eff: b.Eff}, b.Quantidade)
		if err != nil {
			return Envio{}, fmt.Errorf("donate: enviar pacote %q: brinde %d: %w", p.ID, b.Index, err)
		}
		lote = append(lote, partes...)
	}

	// A trava da linha da conta serializa dois envios simultâneos para ela, e é
	// o que faz a checagem de repetição logo abaixo valer: o segundo clique só
	// lê depois de o primeiro gravar.
	var antes int32
	err = tx.QueryRow(ctx,
		`SELECT donate_balance FROM account WHERE id = $1 FOR UPDATE`, accountID).Scan(&antes)
	if errors.Is(err, pgx.ErrNoRows) {
		return Envio{}, ErrNaoEncontrado
	}
	if err != nil {
		return Envio{}, fmt.Errorf("donate: enviar pacote: ler saldo: %w", err)
	}

	var repetido bool
	if err := tx.QueryRow(ctx, `
		SELECT EXISTS (
		  SELECT 1 FROM donate_shop_audit
		   WHERE action = 'credit_balance'
		     AND (after->>'account_id')::bigint = $1
		     AND after->>'pacote' = $2
		     AND created_at > now() - $3::interval)`,
		accountID, p.ID, JanelaDeRepeticao).Scan(&repetido); err != nil {
		return Envio{}, fmt.Errorf("donate: enviar pacote: checar repetição: %w", err)
	}
	if repetido {
		return Envio{}, ErrPacoteRepetido
	}

	depois := antes
	if p.Creditos > 0 {
		if err := tx.QueryRow(ctx,
			`UPDATE account SET donate_balance = donate_balance + $2 WHERE id = $1 RETURNING donate_balance`,
			accountID, p.Creditos).Scan(&depois); err != nil {
			return Envio{}, fmt.Errorf("donate: enviar pacote: creditar a=%d: %w", accountID, err)
		}
	}

	// O registro é o mesmo 'credit_balance' do ajuste manual, com o pacote a
	// mais. Mesmo com zero créditos ele é gravado: é por ele que a carteira e a
	// checagem de repetição sabem que o pacote saiu.
	registro := mustJSON(map[string]any{
		"account_id": accountID,
		"amount":     p.Creditos,
		"balance":    depois,
		"reason":     fmt.Sprintf("Pacote %s: %s", p.Nome(), motivo),
		"pacote":     p.ID,
	})
	if _, err := tx.Exec(ctx, `
		INSERT INTO donate_shop_audit (shop_item_id, account_id, action, before, after)
		VALUES (NULL, $1, 'credit_balance', $2, $3)`,
		actorID, mustJSON(map[string]any{"balance": antes}), registro); err != nil {
		return Envio{}, fmt.Errorf("donate: enviar pacote: auditoria: %w", err)
	}

	// A origem começa por "painel:" porque é esse prefixo que a API do site lê
	// para mostrar "equipe" ao jogador; o pacote vai depois, para a fila dizer
	// de onde veio cada linha.
	origem := entrega.OrigemDoPainel(actorID) + ":pacote:" + p.ID
	ids := make([]int64, 0, len(lote))
	for _, it := range lote {
		id, err := entrega.EnfileirarNa(ctx, tx, origem, accountID, it)
		if err != nil {
			return Envio{}, fmt.Errorf("donate: enviar pacote %q: %w", p.ID, err)
		}
		ids = append(ids, id)
	}

	if err := tx.Commit(ctx); err != nil {
		return Envio{}, fmt.Errorf("donate: enviar pacote: commit: %w", err)
	}
	return Envio{Pacote: p, Saldo: depois, Entregas: ids}, nil
}

// pacoteNaTransacao lê um pacote e os brindes dele, recusando o que não pode ser
// enviado.
func pacoteNaTransacao(ctx context.Context, tx pgx.Tx, id string) (Pacote, error) {
	var (
		p              Pacote
		ativo, soStaff bool
	)
	err := tx.QueryRow(ctx,
		`SELECT id, credits, ativo, so_staff FROM donate_pacote WHERE id = $1`, id).
		Scan(&p.ID, &p.Creditos, &ativo, &soStaff)
	if errors.Is(err, pgx.ErrNoRows) {
		return Pacote{}, ErrPacoteDesconhecido
	}
	if err != nil {
		return Pacote{}, fmt.Errorf("donate: ler pacote %q: %w", id, err)
	}
	if !ativo || soStaff {
		return Pacote{}, ErrPacoteIndisponivel
	}

	rows, err := tx.Query(ctx, `
		SELECT item_index, eff1, effv1, eff2, effv2, eff3, effv3
		  FROM donate_pacote_item WHERE pacote_id = $1 ORDER BY ordem, id`, id)
	if err != nil {
		return Pacote{}, fmt.Errorf("donate: ler brindes de %q: %w", id, err)
	}
	defer rows.Close()
	for rows.Next() {
		var (
			index int32
			e     [6]int16
		)
		if err := rows.Scan(&index, &e[0], &e[1], &e[2], &e[3], &e[4], &e[5]); err != nil {
			return Pacote{}, fmt.Errorf("donate: ler brindes de %q: %w", id, err)
		}
		p.Brindes = append(p.Brindes, brindeDaLinha(index,
			[6]*int16{&e[0], &e[1], &e[2], &e[3], &e[4], &e[5]}))
	}
	if err := rows.Err(); err != nil {
		return Pacote{}, fmt.Errorf("donate: ler brindes de %q: %w", id, err)
	}
	return p, nil
}
