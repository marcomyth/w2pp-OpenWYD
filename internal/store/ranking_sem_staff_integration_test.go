//go:build integration

// Quem NÃO aparece no ranking do site e do bot.
//
//	W2PP_TEST_DSN=postgres://postgres:dev@localhost:5432/postgres go test -tags=integration ./internal/store/
//
// No dia em que o servidor abriu ao público, a equipe estava no topo das duas listas. Um
// ranking que mostra quem testou o jogo não mede nada — e pior, diz ao jogador novo que
// ele nunca vai alcançar o primeiro lugar.
package store

import (
	"context"
	"testing"
)

// personagemDaConta cria um personagem com EXP e uma linha de duelo, para a conta
// aparecer NAS DUAS listas. As duas juntas de propósito: o filtro tem de valer nas duas,
// e um teste que só olhasse uma passaria com metade do conserto feito.
func personagemDaConta(ctx context.Context, t *testing.T, s *Store, conta int64, nome string, exp int64) {
	t.Helper()
	var charID int64
	if err := s.pool.QueryRow(ctx, `
		INSERT INTO character (account_id, slot, name, class, level, exp)
		VALUES ($1, 0, $2, 0, 100, $3) RETURNING id`, conta, nome, exp).Scan(&charID); err != nil {
		t.Fatalf("criando o personagem %q: %v", nome, err)
	}
	if _, err := s.pool.Exec(ctx, `
		INSERT INTO character_pvp_stats (character_id, wins, losses) VALUES ($1, 10, 1)`,
		charID); err != nil {
		t.Fatalf("criando o duelo de %q: %v", nome, err)
	}
}

// naLista diz se o nome aparece na lista de EXP e na de duelo, e o total de cada uma.
func naLista(ctx context.Context, t *testing.T, s *Store, nome string) (exp, duelo bool, totalExp, totalDuelo int) {
	t.Helper()
	listaExp, totalExp, err := s.ListExpRanking(ctx, 100, 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range listaExp {
		if e.Name == nome {
			exp = true
		}
	}
	listaDuelo, totalDuelo, err := s.ListDuelRanking(ctx, 100, 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range listaDuelo {
		if e.Name == nome {
			duelo = true
		}
	}
	return exp, duelo, totalExp, totalDuelo
}

// TestORankingEscondeAEquipeEQuemFoiMarcado.
//
// Três contas e três respostas. O jogador aparece; quem tem cargo não; quem foi marcado
// não. Os três no MESMO teste porque o que importa é a diferença entre eles: um teste
// que só provasse "o admin sumiu" passaria com uma consulta que esvaziou a lista.
//
// O TOTAL É CONFERIDO JUNTO com a lista, e é metade do conserto. Se só a lista filtrasse,
// a página diria "1 de 3" mostrando 1, e o leitor procuraria as outras duas numa página
// que não existe.
func TestORankingEscondeAEquipeEQuemFoiMarcado(t *testing.T) {
	s, ctx := freshStore(t)

	jogador := contaComCargo(ctx, t, s, "rk_jogador", "player")
	chefe := contaComCargo(ctx, t, s, "rk_admin", "admin")
	moderador := contaComCargo(ctx, t, s, "rk_moderator", "moderator")
	marcado := contaComCargo(ctx, t, s, "rk_marcado", "player")

	personagemDaConta(ctx, t, s, jogador, "RkJogador", 5_000)
	personagemDaConta(ctx, t, s, chefe, "RkChefe", 9_000)
	personagemDaConta(ctx, t, s, moderador, "RkModerador", 8_000)
	personagemDaConta(ctx, t, s, marcado, "RkMarcado", 7_000)

	// A marca é escrita por SQL aqui, e não pela função do painel, porque o painel é
	// de outro módulo (adminserver) e este teste é do ranking. O que ele mede é a
	// CONSULTA, não quem apertou o botão.
	if _, err := s.pool.Exec(ctx,
		`UPDATE account SET fora_do_ranking = TRUE WHERE id = $1`, marcado); err != nil {
		t.Fatal(err)
	}

	casos := []struct {
		personagem string
		aparece    bool
		porque     string
	}{
		{"RkJogador", true, "conta comum, sem marca: e para quem o ranking existe"},
		{"RkChefe", false, "cargo de admin"},
		{"RkModerador", false, "cargo de moderator"},
		{"RkMarcado", false, "conta comum MARCADA como fora do ranking"},
	}
	for _, c := range casos {
		exp, duelo, _, _ := naLista(ctx, t, s, c.personagem)
		if exp != c.aparece {
			t.Errorf("%s no ranking de EXP = %v, queria %v (%s)", c.personagem, exp, c.aparece, c.porque)
		}
		if duelo != c.aparece {
			t.Errorf("%s no ranking de duelo = %v, queria %v (%s)", c.personagem, duelo, c.aparece, c.porque)
		}
	}

	// O total tem de contar UM: só o jogador.
	_, _, totalExp, totalDuelo := naLista(ctx, t, s, "RkJogador")
	if totalExp != 1 {
		t.Errorf("total do ranking de EXP = %d, queria 1: o count nao esta filtrando junto "+
			"com a lista, e a pagina vai prometer gente que ela nao mostra", totalExp)
	}
	if totalDuelo != 1 {
		t.Errorf("total do ranking de duelo = %d, queria 1", totalDuelo)
	}
}

// TestDesmarcarDevolveAoRanking: a marca tem volta, e a volta é o caso que ninguém testa.
//
// Marcar errado acontece — é um clique. Se desmarcar não funcionasse, o jeito de
// consertar seria mexer no banco à mão, e é assim que uma tela vira armadilha.
func TestDesmarcarDevolveAoRanking(t *testing.T) {
	s, ctx := freshStore(t)
	conta := contaComCargo(ctx, t, s, "rk_volta", "player")
	personagemDaConta(ctx, t, s, conta, "RkVolta", 1_000)

	if _, err := s.pool.Exec(ctx,
		`UPDATE account SET fora_do_ranking = TRUE WHERE id = $1`, conta); err != nil {
		t.Fatal(err)
	}
	if exp, duelo, _, _ := naLista(ctx, t, s, "RkVolta"); exp || duelo {
		t.Fatalf("marcado ainda aparece: exp=%v duelo=%v", exp, duelo)
	}

	if _, err := s.pool.Exec(ctx,
		`UPDATE account SET fora_do_ranking = FALSE WHERE id = $1`, conta); err != nil {
		t.Fatal(err)
	}
	exp, duelo, totalExp, totalDuelo := naLista(ctx, t, s, "RkVolta")
	if !exp || !duelo {
		t.Errorf("desmarcado NAO voltou: exp=%v duelo=%v", exp, duelo)
	}
	if totalExp != 1 || totalDuelo != 1 {
		t.Errorf("totais depois da volta = %d e %d, queria 1 e 1", totalExp, totalDuelo)
	}
}
