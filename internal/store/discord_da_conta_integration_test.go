//go:build integration

// Testes de integração do vínculo do Discord: um Discord para uma conta, e trocar é
// recusado.
//
//	W2PP_TEST_DSN=postgres://postgres:dev@localhost:5432/postgres go test -tags=integration ./internal/store/
package store

import (
	"errors"
	"testing"
)

// TestUmDiscordParaUmaConta é a trava inteira, vista de fora.
func TestUmDiscordParaUmaConta(t *testing.T) {
	s, ctx := freshStore(t)
	primeira := contaPix(ctx, t, s, "discord_um")
	segunda := contaPix(ctx, t, s, "discord_dois")
	const id = "123456789012345678"

	if err := s.VincularDiscord(ctx, primeira, id); err != nil {
		t.Fatalf("o primeiro vínculo: %v", err)
	}
	if got, err := s.DiscordDaConta(ctx, primeira); err != nil || got != id {
		t.Fatalf("leitura = %q, %v", got, err)
	}

	// O MESMO DISCORD EM OUTRA CONTA é recusado. É o índice único parcial que
	// decide, e não uma consulta antes dele.
	if err := s.VincularDiscord(ctx, segunda, id); !errors.Is(err, ErrDiscordEmOutraConta) {
		t.Fatalf("erro = %v, queria ErrDiscordEmOutraConta", err)
	}
	if got, _ := s.DiscordDaConta(ctx, segunda); got != "" {
		t.Errorf("a segunda conta ficou com %q", got)
	}
}

// TestGravarOMesmoDiscordDeNovoPassa: é quem refaz o OAuth. Recusar ali mandaria a
// pessoa pedir ajuda para uma coisa que já está do jeito que ela quer.
func TestGravarOMesmoDiscordDeNovoPassa(t *testing.T) {
	s, ctx := freshStore(t)
	conta := contaPix(ctx, t, s, "discord_repete")
	const id = "222222222222222222"

	for i := 0; i < 3; i++ {
		if err := s.VincularDiscord(ctx, conta, id); err != nil {
			t.Fatalf("vínculo %d: %v", i, err)
		}
	}
	if got, _ := s.DiscordDaConta(ctx, conta); got != id {
		t.Errorf("ficou %q", got)
	}
}

// TestTrocarODiscordDaContaERecusado.
//
// Sem isto, quem tomasse uma conta trocaria o vínculo em silêncio e levaria junto o
// cargo que o Discord dá. Só a staff desfaz.
func TestTrocarODiscordDaContaERecusado(t *testing.T) {
	s, ctx := freshStore(t)
	conta := contaPix(ctx, t, s, "discord_troca")
	const velho = "333333333333333333"
	const novo = "444444444444444444"

	if err := s.VincularDiscord(ctx, conta, velho); err != nil {
		t.Fatal(err)
	}
	if err := s.VincularDiscord(ctx, conta, novo); !errors.Is(err, ErrContaTemOutroDiscord) {
		t.Fatalf("erro = %v, queria ErrContaTemOutroDiscord", err)
	}
	if got, _ := s.DiscordDaConta(ctx, conta); got != velho {
		t.Errorf("o vínculo mudou para %q", got)
	}

	// A staff desfaz, e aí o novo entra.
	if err := s.DesvincularDiscord(ctx, conta); err != nil {
		t.Fatalf("desvincular: %v", err)
	}
	if got, _ := s.DiscordDaConta(ctx, conta); got != "" {
		t.Fatalf("depois de desvincular ficou %q", got)
	}
	if err := s.VincularDiscord(ctx, conta, novo); err != nil {
		t.Fatalf("depois de a staff desfazer: %v", err)
	}
}

// TestODiscordRecusaOQueNaoESnowflake: a validação é do servidor. O site faz o OAuth,
// mas um campo digitado à mão ou um cliente remendado não gravam qualquer coisa.
func TestODiscordRecusaOQueNaoESnowflake(t *testing.T) {
	s, ctx := freshStore(t)
	conta := contaPix(ctx, t, s, "discord_formato")

	ruins := []string{
		"",                        // vazio
		" ",                       // espaço
		"12345678901234567890123", // longo demais
		"12a34",                   // letra no meio
		"-123456789012345678",     // sinal
		"1234567890123456789.0",   // ponto
		"１２３４５６７８９",               // dígitos de largura total
	}
	for _, id := range ruins {
		if err := s.VincularDiscord(ctx, conta, id); !errors.Is(err, ErrDiscordInvalido) {
			t.Errorf("%q: erro = %v, queria ErrDiscordInvalido", id, err)
		}
	}
	if got, _ := s.DiscordDaConta(ctx, conta); got != "" {
		t.Errorf("algum inválido entrou: %q", got)
	}
}

// TestAStringVaziaNaoBurlaAUnicidade.
//
// Em Postgres NULL não conflita com NULL, então o índice parcial sozinho deixaria
// duas contas com string vazia — e a unicidade viraria uma promessa que o banco não cumpre. O
// CHECK é a segunda trava, e este teste vai DIRETO ao banco para provar que ela
// existe mesmo que alguém contorne o código.
func TestAStringVaziaNaoBurlaAUnicidade(t *testing.T) {
	s, ctx := freshStore(t)
	conta := contaPix(ctx, t, s, "discord_vazio")

	if _, err := s.pool.Exec(ctx,
		`UPDATE account SET discord_id = '' WHERE id = $1`, conta); err == nil {
		t.Fatal("o banco aceitou string vazia: o CHECK não está lá")
	}
}

// TestDiscordDeContaQueNaoExiste: a conta vem da sessão, então isto é conta apagada
// no meio do caminho — e tem de dizer NotFound, não "sem Discord".
func TestDiscordDeContaQueNaoExiste(t *testing.T) {
	s, ctx := freshStore(t)
	if _, err := s.DiscordDaConta(ctx, 999888); !errors.Is(err, ErrNotFound) {
		t.Errorf("leitura: erro = %v, queria ErrNotFound", err)
	}
	if err := s.VincularDiscord(ctx, 999888, "555555555555555555"); !errors.Is(err, ErrNotFound) {
		t.Errorf("vínculo: erro = %v, queria ErrNotFound", err)
	}
}

// TestODiscordSaiNoLoginDaConta: a página do site precisa dele no MESMO instante em
// que precisa do papel, e é por isso que ele viaja na leitura que o login já faz.
func TestODiscordSaiNoLoginDaConta(t *testing.T) {
	s, ctx := freshStore(t)
	const id = "666666666666666666"
	conta := contaPix(ctx, t, s, "discord_login")
	if err := s.VincularDiscord(ctx, conta, id); err != nil {
		t.Fatal(err)
	}
	auth, err := s.AccountByName(ctx, "discord_login")
	if err != nil {
		t.Fatalf("AccountByName: %v", err)
	}
	if auth.DiscordID != id {
		t.Errorf("o login trouxe %q, queria %q", auth.DiscordID, id)
	}

	// E sem vínculo ele vem VAZIO, não nulo: quem lê não precisa tratar ponteiro.
	outra := contaPix(ctx, t, s, "discord_login_sem")
	semVinculo, err := s.AccountByName(ctx, "discord_login_sem")
	if err != nil {
		t.Fatal(err)
	}
	if semVinculo.DiscordID != "" {
		t.Errorf("conta sem vínculo trouxe %q", semVinculo.DiscordID)
	}
	_ = outra
}
