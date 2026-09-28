package main

import (
	"io"
	"log/slog"
	"strings"
	"testing"
)

// O QUE ESTES TESTES GUARDAM: que o criar-conta recuse ANTES de abrir o banco.
//
// Todos rodam sem DSN, e é isso que os torna a rede de segurança certa. A recusa que
// chega depois do banco custa a espera das migrações inteiras — cinco minutos, no
// ambiente novo que é justamente onde este comando roda — e a pessoa fica olhando um
// terminal parado para descobrir no fim que digitou o cargo errado.
//
// Como não há DSN, qualquer caso que PASSASSE pela validação morreria em "falta o
// DSN". Então cada teste confere a MENSAGEM, e não só que deu erro: se um dia a ordem
// das conferências mudar e a validação for parar depois da abertura do banco, o teste
// vê "falta o DSN" onde esperava "cargo" e falha.

func semLogger() *slog.Logger { return slog.New(slog.DiscardHandler) }

func TestCriarContaRecusaAntesDeTocarOBanco(t *testing.T) {
	t.Parallel()
	casos := []struct {
		nome   string
		args   []string
		senha  string
		espera string
	}{
		{
			nome:   "sem login",
			args:   []string{"-dsn", "postgres://nao-usado"},
			senha:  "abc123",
			espera: "falta -login",
		},
		{
			nome:   "cargo que nao existe",
			args:   []string{"-dsn", "postgres://nao-usado", "-login", "marco", "-cargo", "chefe"},
			senha:  "abc123",
			espera: "cargo",
		},
		{
			// A senha de JOGO tem MÁXIMO de 12. Treze passa pela régua do painel (que
			// tem mínimo de 12) e tem de morrer aqui — é a confusão que o comando existe
			// para não deixar acontecer.
			nome:   "senha de jogo longa demais",
			args:   []string{"-dsn", "postgres://nao-usado", "-login", "marco"},
			senha:  "1234567890123",
			espera: "senha de jogo",
		},
		{
			nome:   "senha com espaco",
			args:   []string{"-dsn", "postgres://nao-usado", "-login", "marco"},
			senha:  "abc 123",
			espera: "senha de jogo",
		},
		{
			nome:   "senha com acento nao cabe no cliente de 2003",
			args:   []string{"-dsn", "postgres://nao-usado", "-login", "marco"},
			senha:  "senhaç",
			espera: "senha de jogo",
		},
		{
			nome:   "senha vazia",
			args:   []string{"-dsn", "postgres://nao-usado", "-login", "marco"},
			senha:  "",
			espera: "senha de jogo",
		},
	}
	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			t.Parallel()
			err := criarConta(semLogger(), c.args, strings.NewReader(c.senha))
			if err == nil {
				t.Fatalf("passou, e devia ter sido recusado")
			}
			if !strings.Contains(err.Error(), c.espera) {
				t.Fatalf("erro = %q, queria algo com %q", err, c.espera)
			}
			// A prova de que parou antes do banco: a abertura do pool é a primeira coisa
			// depois da validação, e ela reclamaria do DSN.
			if strings.Contains(err.Error(), "abrindo o banco") {
				t.Fatalf("chegou a abrir o banco: %v", err)
			}
		})
	}
}

// TestCriarContaAceitaOsTresCargos guarda a lista contra quem a encurtar sem querer.
// Sem DSN válido os três têm de atravessar a validação e morrer no banco, e não antes.
func TestCriarContaAceitaOsTresCargos(t *testing.T) {
	t.Parallel()
	for _, cargo := range []string{"player", "moderator", "admin"} {
		t.Run(cargo, func(t *testing.T) {
			t.Parallel()
			err := criarConta(semLogger(),
				[]string{"-dsn", "postgres://127.0.0.1:1/naoexiste", "-login", "marco", "-cargo", cargo,
					"-prazo", "1s"},
				strings.NewReader("abc123"))
			if err == nil {
				t.Fatal("sem banco, tinha de falhar em algum lugar")
			}
			if strings.Contains(err.Error(), "cargo") {
				t.Fatalf("o cargo %q foi recusado e é válido: %v", cargo, err)
			}
		})
	}
}

// TestLeTextoDaEntradaCortaSoOFim: a quebra do `echo` sai, o espaço do meio fica.
func TestLeTextoDaEntradaCortaSoOFim(t *testing.T) {
	t.Parallel()
	casos := []struct{ entrada, quer string }{
		{"abc123\n", "abc123"},
		{"abc123\r\n", "abc123"},
		{"abc123", "abc123"},
		{"abc123\n\n", "abc123"},
		{" a b ", " a b "},
		{"\tabc\t\n", "\tabc\t"},
	}
	for _, c := range casos {
		got, err := leTextoDaEntrada(strings.NewReader(c.entrada))
		if err != nil {
			t.Fatalf("entrada %q: %v", c.entrada, err)
		}
		if got != c.quer {
			t.Errorf("entrada %q -> %q, queria %q", c.entrada, got, c.quer)
		}
	}
}

// TestLeSenhaDeJogoRecusaAntesDeHashear: a régua do jogo é MÁXIMO 12, e a do painel é
// MÍNIMO 12. Este teste fixa a diferença, que é a armadilha mais fácil aqui.
func TestAsDuasReguasDeSenhaSaoOpostas(t *testing.T) {
	t.Parallel()
	doze := "123456789012"

	// Doze serve nas duas: é o teto do jogo e o piso do painel.
	if _, err := leSenhaDeJogoDaEntrada(strings.NewReader(doze)); err != nil {
		t.Errorf("doze caracteres tinham de servir para o jogo: %v", err)
	}
	if _, err := leSenhaDaEntrada(strings.NewReader(doze)); err != nil {
		t.Errorf("doze caracteres tinham de servir para o painel: %v", err)
	}

	// Treze serve no painel e NÃO no jogo.
	treze := doze + "3"
	if _, err := leSenhaDeJogoDaEntrada(strings.NewReader(treze)); err == nil {
		t.Error("treze caracteres não podem servir para o jogo")
	}
	if _, err := leSenhaDaEntrada(strings.NewReader(treze)); err != nil {
		t.Errorf("treze caracteres tinham de servir para o painel: %v", err)
	}

	// Seis serve no jogo e NÃO no painel.
	seis := "abc123"
	if _, err := leSenhaDeJogoDaEntrada(strings.NewReader(seis)); err != nil {
		t.Errorf("seis caracteres tinham de servir para o jogo: %v", err)
	}
	if _, err := leSenhaDaEntrada(strings.NewReader(seis)); err == nil {
		t.Error("seis caracteres não podem servir para o painel")
	}
}

var _ io.Reader = strings.NewReader("")
