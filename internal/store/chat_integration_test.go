//go:build integration

// Integration tests for the chat log, against a real PostgreSQL.
//
//	W2PP_TEST_DSN=postgres://postgres@localhost:5432/postgres go test -tags=integration ./internal/store/
//
// What earns a database here is the retention and the search. The retention is
// the promise made to the players — thirty days, and lowering the number has to
// shrink what is ALREADY stored — and it is silent when wrong: a sweep that
// never fires looks like nothing at all until the table is enormous.
package store

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/jeanluca/w2pp-openwyd/internal/domain"
)

func storeChat(t *testing.T) *Store {
	t.Helper()
	pool := testPool(t)
	if err := Migrate(context.Background(), pool); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	s := New(pool)
	limpar := func() {
		ctx := context.Background()
		_, _ = s.pool.Exec(ctx, `DELETE FROM chat_log`)
		_, _ = s.pool.Exec(ctx, `UPDATE chat_log_meta SET varrido_em = NULL, dias = 30, apagadas = 0`)
	}
	limpar()
	t.Cleanup(limpar)
	return s
}

func fala(quando time.Time, tipo domain.ChatTipo, quem, alvo, texto string) domain.ChatLinha {
	return domain.ChatLinha{
		At: quando, Tipo: tipo, Character: quem, Alvo: alvo, Texto: texto,
		X: 2100, Y: 2100,
	}
}

func TestChatGravaLoteEDevolveOQueFoiDito(t *testing.T) {
	ctx := context.Background()
	s := storeChat(t)
	agora := time.Now()

	err := s.RecordChat(ctx, []domain.ChatLinha{
		fala(agora.Add(-2*time.Minute), domain.ChatPublico, "Falante", "", "oi geral"),
		fala(agora.Add(-time.Minute), domain.ChatSussurro, "Falante", "Ouvinte", "so pra voce"),
	})
	if err != nil {
		t.Fatalf("RecordChat: %v", err)
	}

	linhas, err := s.ListChat(ctx, ChatQuery{})
	if err != nil {
		t.Fatalf("ListChat: %v", err)
	}
	if len(linhas) != 2 {
		t.Fatalf("linhas = %d, want 2", len(linhas))
	}
	// Mais novo primeiro.
	if linhas[0].Texto != "so pra voce" || linhas[0].Tipo != domain.ChatSussurro {
		t.Errorf("primeira linha = %+v", linhas[0])
	}
	if linhas[0].Alvo != "Ouvinte" {
		t.Errorf("alvo = %q, want Ouvinte", linhas[0].Alvo)
	}
	// Público não tem destinatário, e gravar um vazio como se fosse nome faria a
	// busca por alvo encontrar coisa que não existe.
	if linhas[1].Alvo != "" {
		t.Errorf("fala pública veio com alvo %q", linhas[1].Alvo)
	}
	if linhas[1].X != 2100 || linhas[1].Y != 2100 {
		t.Errorf("posição = (%d,%d)", linhas[1].X, linhas[1].Y)
	}
	// A hora é a da FALA, não a da gravação: o lote sai segundos depois, e
	// carimbar tudo no flush juntaria a conversa toda num instante só.
	if d := linhas[1].At.Sub(agora.Add(-2 * time.Minute)).Abs(); d > time.Second {
		t.Errorf("hora gravada difere da hora da fala em %v", d)
	}
}

// O nome procura os dois lados. Perguntar "o que passou por este jogador" e
// receber só metade é como ler um lado de uma discussão.
func TestChatBuscaPorNomeAchaOsDoisLados(t *testing.T) {
	ctx := context.Background()
	s := storeChat(t)
	agora := time.Now()

	if err := s.RecordChat(ctx, []domain.ChatLinha{
		fala(agora, domain.ChatSussurro, "Acusado", "Vitima", "me da o item"),
		fala(agora, domain.ChatSussurro, "Vitima", "Acusado", "nao vou dar"),
		fala(agora, domain.ChatPublico, "Terceiro", "", "papo alheio"),
	}); err != nil {
		t.Fatalf("RecordChat: %v", err)
	}

	linhas, err := s.ListChat(ctx, ChatQuery{Char: "Vitima"})
	if err != nil {
		t.Fatalf("ListChat: %v", err)
	}
	if len(linhas) != 2 {
		t.Fatalf("linhas = %d, want 2 (o que ela disse e o que mandaram para ela): %+v", len(linhas), linhas)
	}
	for _, l := range linhas {
		if l.Character != "Vitima" && l.Alvo != "Vitima" {
			t.Errorf("linha sem relação com a busca: %+v", l)
		}
	}
}

func TestChatBuscaPorPalavraIgnoraMaiuscula(t *testing.T) {
	ctx := context.Background()
	s := storeChat(t)
	agora := time.Now()

	if err := s.RecordChat(ctx, []domain.ChatLinha{
		fala(agora, domain.ChatPublico, "A", "", "vende ARMADURA celestial"),
		fala(agora, domain.ChatPublico, "B", "", "compro poção"),
	}); err != nil {
		t.Fatalf("RecordChat: %v", err)
	}
	linhas, err := s.ListChat(ctx, ChatQuery{Texto: "armadura"})
	if err != nil {
		t.Fatalf("ListChat: %v", err)
	}
	if len(linhas) != 1 || linhas[0].Character != "A" {
		t.Fatalf("linhas = %+v, want só a de A", linhas)
	}
}

func TestChatFiltraPorCanalEPorPeriodo(t *testing.T) {
	ctx := context.Background()
	s := storeChat(t)
	agora := time.Now()

	if err := s.RecordChat(ctx, []domain.ChatLinha{
		fala(agora, domain.ChatPublico, "A", "", "hoje publico"),
		fala(agora, domain.ChatSussurro, "A", "B", "hoje sussurro"),
		fala(agora.Add(-72*time.Hour), domain.ChatSussurro, "A", "B", "tres dias atras"),
	}); err != nil {
		t.Fatalf("RecordChat: %v", err)
	}

	so, err := s.ListChat(ctx, ChatQuery{Char: "A", Tipo: domain.ChatSussurro})
	if err != nil {
		t.Fatalf("ListChat: %v", err)
	}
	if len(so) != 2 {
		t.Errorf("sussurros = %d, want 2", len(so))
	}

	recente, err := s.ListChat(ctx, ChatQuery{Char: "A", Desde: agora.Add(-24 * time.Hour)})
	if err != nil {
		t.Fatalf("ListChat: %v", err)
	}
	if len(recente) != 2 {
		t.Errorf("últimas 24h = %d, want 2", len(recente))
	}
}

func TestChatRecusaCanalDesconhecido(t *testing.T) {
	ctx := context.Background()
	s := storeChat(t)
	if err := s.RecordChat(ctx, []domain.ChatLinha{
		fala(time.Now(), "grito", "A", "", "aaa"),
	}); err == nil {
		t.Error("RecordChat aceitou um canal que não existe")
	}
	if _, err := s.ListChat(ctx, ChatQuery{Tipo: "grito"}); err == nil {
		t.Error("ListChat aceitou um canal que não existe")
	}
}

// Meio lote é meia conversa, e o buraco pareceria silêncio em vez de falha.
func TestChatRecusaLoteGrandeDemaisSemGravarNada(t *testing.T) {
	ctx := context.Background()
	s := storeChat(t)
	agora := time.Now()

	grande := make([]domain.ChatLinha, chatLoteMax+1)
	for i := range grande {
		grande[i] = fala(agora, domain.ChatPublico, "A", "", fmt.Sprintf("linha %d", i))
	}
	if err := s.RecordChat(ctx, grande); err == nil {
		t.Fatal("aceitou um lote acima do teto")
	}
	linhas, err := s.ListChat(ctx, ChatQuery{})
	if err != nil {
		t.Fatalf("ListChat: %v", err)
	}
	if len(linhas) != 0 {
		t.Errorf("gravou %d linhas de um lote recusado", len(linhas))
	}
}

// O pedido da dona do servidor, ao pé da letra: trinta dias, e vinte se pesar.
// Baixar o número tem de encolher o que JÁ está gravado — é por isso que o
// prazo não é uma coluna em cada linha.
func TestChatBaixarOPrazoEncolheOQueJaEstaGravado(t *testing.T) {
	ctx := context.Background()
	s := storeChat(t)
	agora := time.Now()

	if err := s.RecordChat(ctx, []domain.ChatLinha{
		fala(agora.Add(-31*24*time.Hour), domain.ChatPublico, "A", "", "velha demais"),
		fala(agora.Add(-25*24*time.Hour), domain.ChatPublico, "A", "", "morre com 20"),
		fala(agora.Add(-10*24*time.Hour), domain.ChatPublico, "A", "", "fica"),
	}); err != nil {
		t.Fatalf("RecordChat: %v", err)
	}

	n, err := s.PurgeChat(ctx, 30)
	if err != nil {
		t.Fatalf("PurgeChat(30): %v", err)
	}
	if n != 1 {
		t.Errorf("com 30 dias apagou %d, want 1", n)
	}

	// E agora o servidor pesou.
	n, err = s.PurgeChat(ctx, 20)
	if err != nil {
		t.Fatalf("PurgeChat(20): %v", err)
	}
	if n != 1 {
		t.Errorf("baixando para 20 apagou %d, want 1 (a de 25 dias, que já estava gravada)", n)
	}
	linhas, err := s.ListChat(ctx, ChatQuery{})
	if err != nil {
		t.Fatalf("ListChat: %v", err)
	}
	if len(linhas) != 1 || linhas[0].Texto != "fica" {
		t.Errorf("sobrou %+v, want só a de 10 dias", linhas)
	}
}

func TestChatVarreduraContaOQueFez(t *testing.T) {
	ctx := context.Background()
	s := storeChat(t)
	agora := time.Now()

	if err := s.RecordChat(ctx, []domain.ChatLinha{
		fala(agora.Add(-40*24*time.Hour), domain.ChatPublico, "A", "", "uma"),
		fala(agora.Add(-40*24*time.Hour), domain.ChatPublico, "A", "", "duas"),
	}); err != nil {
		t.Fatalf("RecordChat: %v", err)
	}
	if _, err := s.PurgeChat(ctx, 30); err != nil {
		t.Fatalf("PurgeChat: %v", err)
	}

	v, err := s.ChatSweep(ctx)
	if err != nil {
		t.Fatalf("ChatSweep: %v", err)
	}
	// A tela lê isto para dizer o prazo que está VALENDO, em vez de afirmar o
	// que ela própria acha: o número mora no ambiente de outro processo.
	if v.Dias != 30 {
		t.Errorf("dias = %d, want 30", v.Dias)
	}
	if v.Apagadas != 2 {
		t.Errorf("apagadas = %d, want 2", v.Apagadas)
	}
	if v.VarridoEm.IsZero() {
		t.Error("varrido_em vazio depois de uma varredura")
	}
}

func TestChatRecusaPrazoForaDaFaixa(t *testing.T) {
	s := storeChat(t)
	for _, d := range []int{0, -1, domain.ChatRetencaoMax + 1} {
		if _, err := s.PurgeChat(context.Background(), d); err == nil {
			t.Errorf("aceitou varrer com prazo de %d dias", d)
		}
	}
}

// Antes da primeira varredura não há o que relatar, e isso não é erro: é um
// servidor que acabou de subir.
func TestChatVarreduraAntesDaPrimeiraNaoEhErro(t *testing.T) {
	s := storeChat(t)
	v, err := s.ChatSweep(context.Background())
	if err != nil {
		t.Fatalf("ChatSweep: %v", err)
	}
	if !v.VarridoEm.IsZero() || v.Apagadas != 0 {
		t.Errorf("varredura = %+v, want vazia", v)
	}
}
