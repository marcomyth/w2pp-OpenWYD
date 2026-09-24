package buildinfo

import (
	"strings"
	"testing"
)

// A REVISÃO CAI NA VARIÁVEL DA PLATAFORMA em vez de "unknown".
//
// É o caso medido: em produção, 24/09/2026, o boot dizia `revision=unknown`, porque o
// construtor compila fora de um checkout git e ninguém passa o -ldflags. A variável
// estava lá o tempo todo.
func TestARevisaoUsaAVariavelDaPlataforma(t *testing.T) {
	t.Setenv("GIT_COMMIT", "2a2c9adbe2c243575603456a8f259db616a00d0f")
	got := doAmbiente()
	if !strings.HasPrefix(got, "2a2c9adb") {
		t.Errorf("doAmbiente() = %q; queria comecar com a revisao curta", got)
	}
	// E ELA DIZ DE ONDE VEIO. A palavra de quem construiu não é a mesma coisa que o
	// carimbo do compilador sobre o próprio binário, e quem lê o log precisa saber
	// qual das duas está vendo.
	if !strings.Contains(got, "ambiente") {
		t.Errorf("doAmbiente() = %q; nao diz que veio do ambiente", got)
	}
}

// A SEGUNDA VARIÁVEL TAMBÉM SERVE, e a primeira tem precedência.
func TestAOrdemDasVariaveis(t *testing.T) {
	t.Setenv("GIT_COMMIT", "")
	t.Setenv("RAILWAY_GIT_COMMIT_SHA", "abcdef1234567890")
	if got := doAmbiente(); !strings.HasPrefix(got, "abcdef12") {
		t.Errorf("sem GIT_COMMIT, deveria cair na da plataforma; deu %q", got)
	}
	t.Setenv("GIT_COMMIT", "1111111122222222")
	if got := doAmbiente(); !strings.HasPrefix(got, "11111111") {
		t.Errorf("com as duas, a GIT_COMMIT manda; deu %q", got)
	}
}

// SEM NENHUMA DAS DUAS, continua "unknown" — e isso é certo: inventar uma revisão
// seria pior do que admitir que não se sabe qual é.
func TestSemVariavelContinuaDesconhecido(t *testing.T) {
	t.Setenv("GIT_COMMIT", "")
	t.Setenv("RAILWAY_GIT_COMMIT_SHA", "   ")
	if got := doAmbiente(); got != "unknown" {
		t.Errorf("doAmbiente() = %q, queria \"unknown\"", got)
	}
}
