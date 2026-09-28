//go:build integration

// As ações de dinheiro feitas por um USUÁRIO DO PAINEL.
//
//	W2PP_TEST_DSN=postgres://postgres:dev@localhost:5432/postgres go test -tags=integration ./internal/store/
//
// O DEFEITO QUE ESTE ARQUIVO PRENDE, e ele bloqueava a abertura do dinheiro real:
//
// Desde o #130 a staff entra no painel como usuário do painel, e essa pessoa NÃO tem
// conta de jogo — o AccountID dela é 0. As cinco escritas de auditoria desta pasta
// gravavam só o actor_account_id, então mandavam 0.
//
// Zero NÃO vira "sem ator": a coluna tem chave estrangeira para account (0022:34), então
// o banco vai procurar a conta de id 0, não acha, e recusa. Como a auditoria é gravada
// DENTRO da transação do dinheiro, a recusa derruba a transação inteira — o repasse não
// é marcado como pago, a chave não é revelada, e a pessoa vê um erro genérico.
//
// Com o RMT aberto, isso seria: o dinheiro entra e a staff não consegue pagar quem
// vendeu. Pior do que não abrir.
package store

import (
	"context"
	"testing"
)

// usuarioDoPainel cria uma staff que entra pelo painel, sem conta de jogo.
func usuarioDoPainel(ctx context.Context, t *testing.T, s *Store, login string) int64 {
	t.Helper()
	var id int64
	if err := s.pool.QueryRow(ctx, `
		INSERT INTO painel_usuario (login, senha_hash, papel)
		VALUES ($1, 'x', 'admin') RETURNING id`, login).Scan(&id); err != nil {
		t.Fatalf("criando o usuario do painel %q: %v", login, err)
	}
	return id
}

// oQueAAuditoriaGuardou devolve os dois atores da última linha daquela ação.
func oQueAAuditoriaGuardou(ctx context.Context, t *testing.T, s *Store, acao string) (conta, painel *int64) {
	t.Helper()
	if err := s.pool.QueryRow(ctx, `
		SELECT actor_account_id, actor_painel_usuario_id
		  FROM admin_audit_log WHERE action = $1
		 ORDER BY id DESC LIMIT 1`, acao).Scan(&conta, &painel); err != nil {
		t.Fatalf("a auditoria de %q nao foi gravada: %v", acao, err)
	}
	return conta, painel
}

// confereAtorDoPainel: a linha tem o usuário do painel e NÃO tem conta de jogo.
//
// Os dois lados importam. Só conferir que o painel está preenchido deixaria passar uma
// linha com os dois — que o CHECK do banco recusa, mas que um teste frouxo não pegaria se
// alguém tirasse o CHECK.
func confereAtorDoPainel(t *testing.T, acao string, conta, painel *int64, esperado int64) {
	t.Helper()
	if conta != nil {
		t.Errorf("%s: gravou conta de jogo %d; quem agiu foi um usuario do painel", acao, *conta)
	}
	if painel == nil {
		t.Fatalf("%s: nao gravou o usuario do painel", acao)
	}
	if *painel != esperado {
		t.Errorf("%s: usuario do painel = %d, queria %d", acao, *painel, esperado)
	}
}

// TestAStaffDoPainelConsegueMexerNoDinheiro.
//
// As três ações de que a abertura do dinheiro real depende, feitas por quem entra pelo
// painel novo. Antes deste conserto, as três FALHAVAM — e falhavam derrubando a
// transação, não só a auditoria.
//
// Cada uma confere DUAS coisas: que o dinheiro se moveu, e que a linha da auditoria tem
// dono. Conferir só a auditoria deixaria passar um conserto que grava a linha e não faz
// o trabalho; conferir só o trabalho deixaria passar o contrário.
func TestAStaffDoPainelConsegueMexerNoDinheiro(t *testing.T) {
	s, ctx := freshStore(t)
	painelID := usuarioDoPainel(ctx, t, s, "staff_do_painel")
	ator := AtorDoAjuste{PainelID: painelID, Papel: "admin", Nome: "staff_do_painel"}

	t.Run("marcar como pago", func(t *testing.T) {
		id, _ := repassePendenteDeVenda(ctx, t, s, "painel_pago")
		if err := s.MarcarRepassePagoAMao(ctx, id, ator, "pago no app, comprovante P1"); err != nil {
			t.Fatalf("a staff do painel nao conseguiu marcar como pago: %v", err)
		}
		var estado int16
		if err := s.pool.QueryRow(ctx,
			`SELECT status FROM rmt_repasse WHERE id = $1`, id).Scan(&estado); err != nil {
			t.Fatal(err)
		}
		if EstadoRepasse(estado) != RepassePago {
			t.Errorf("estado = %d, queria PAGO: a acao nao aconteceu", estado)
		}
		conta, painel := oQueAAuditoriaGuardou(ctx, t, s, AcaoRepassePagoAMao)
		confereAtorDoPainel(t, "marcar como pago", conta, painel, painelID)
	})

	t.Run("revelar a chave para pagar", func(t *testing.T) {
		id, _ := repassePendenteDeVenda(ctx, t, s, "painel_chave")
		chave, err := s.ChaveParaPagar(ctx, id, ator)
		if err != nil {
			t.Fatalf("a staff do painel nao conseguiu ler a chave: %v", err)
		}
		if chave.ChavePix == "" {
			t.Error("a chave voltou vazia: sem ela nao ha para onde mandar o dinheiro")
		}
		conta, painel := oQueAAuditoriaGuardou(ctx, t, s, AcaoChavePixRevelada)
		confereAtorDoPainel(t, "revelar a chave", conta, painel, painelID)
	})

	t.Run("ajustar o valor do repasse", func(t *testing.T) {
		id := repasseRecusadoParaAjuste(ctx, t, s, "painel_ajuste")
		if _, err := s.AjustarValorDoRepasse(ctx, id, 123, ator, "taxa da processadora"); err != nil {
			t.Fatalf("a staff do painel nao conseguiu ajustar: %v", err)
		}
		var valor int64
		if err := s.pool.QueryRow(ctx,
			`SELECT valor_centavos FROM rmt_repasse WHERE id = $1`, id).Scan(&valor); err != nil {
			t.Fatal(err)
		}
		if valor != 123 {
			t.Errorf("valor = %d, queria 123: o ajuste nao aconteceu", valor)
		}
		conta, painel := oQueAAuditoriaGuardou(ctx, t, s, AcaoAjusteDeRepasse)
		confereAtorDoPainel(t, "ajustar o valor", conta, painel, painelID)
	})
}

// TestAContaDeJogoContinuaFuncionando.
//
// O conserto não pode ter quebrado a outra porta. Quem entra com conta de jogo continua
// gravando a conta, e NÃO o painel — as duas colunas são exclusivas, e trocar uma pela
// outra atribuiria a ação à pessoa errada.
func TestAContaDeJogoContinuaFuncionando(t *testing.T) {
	s, ctx := freshStore(t)
	id, _ := repassePendenteDeVenda(ctx, t, s, "conta_de_jogo")
	// O staffQuePaga() usa ContaID 1, que existe como conta de jogo nesta base.
	if err := s.MarcarRepassePagoAMao(ctx, id, staffQuePaga(), "pago pela conta de jogo"); err != nil {
		t.Fatalf("a conta de jogo parou de funcionar: %v", err)
	}
	conta, painel := oQueAAuditoriaGuardou(ctx, t, s, AcaoRepassePagoAMao)
	if conta == nil {
		t.Error("nao gravou a conta de jogo")
	}
	if painel != nil {
		t.Errorf("gravou usuario do painel %d numa acao feita pela conta de jogo", *painel)
	}
}
