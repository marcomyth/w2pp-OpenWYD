//go:build integration

// Testes de integração do emblema de guilda: quem pode trocar, o que é imagem
// válida, e o limite de uma troca por guilda.
//
//	W2PP_TEST_DSN=postgres://postgres:dev@localhost:5432/postgres go test -tags=integration ./internal/store/
package store

import (
	"context"
	"encoding/binary"
	"errors"
	"testing"
	"time"

	"github.com/jeanluca/w2pp-openwyd/internal/emblema"
)

// bmpDoCliente monta o emblema exato que o jogo desenha.
func bmpDoCliente() []byte {
	b := make([]byte, emblema.Tamanho)
	b[0], b[1] = 'B', 'M'
	binary.LittleEndian.PutUint32(b[2:], emblema.Tamanho)
	binary.LittleEndian.PutUint32(b[10:], 54)
	binary.LittleEndian.PutUint32(b[14:], 40)
	binary.LittleEndian.PutUint32(b[18:], emblema.Largura)
	binary.LittleEndian.PutUint32(b[22:], emblema.Altura)
	binary.LittleEndian.PutUint16(b[26:], 1)
	binary.LittleEndian.PutUint16(b[28:], 24)
	binary.LittleEndian.PutUint32(b[30:], 0)
	// Um pixel diferente de zero, para o teste distinguir a imagem gravada da vazia.
	b[54] = 0x7f
	return b
}

// guildaComLider cria a guilda e põe um personagem da conta com o nível pedido.
func guildaComLider(ctx context.Context, t *testing.T, s *Store, nome string, id int, conta int64, nivel int) {
	t.Helper()
	if _, err := s.pool.Exec(ctx,
		`INSERT INTO guild (id, name) VALUES ($1, $2)`, id, nome); err != nil {
		t.Fatalf("criando a guilda: %v", err)
	}
	var charID int64
	if err := s.pool.QueryRow(ctx, `
		INSERT INTO character (account_id, slot, name, class, level, guild_id, guild_level)
		VALUES ($1, 0, $2, 1, 50, $3, $4) RETURNING id`,
		conta, nome+"_p", id, nivel).Scan(&charID); err != nil {
		t.Fatalf("criando o personagem: %v", err)
	}
	if _, err := s.pool.Exec(ctx, `
		INSERT INTO guild_member (guild_id, character_id, account_id, slot, name, guild_level)
		VALUES ($1, $2, $3, 0, $4, $5)`, id, charID, conta, nome+"_p", nivel); err != nil {
		t.Fatalf("criando o membro: %v", err)
	}
}

// TestSoOLiderTrocaOEmblema: líder é guild_level 9, e não "quem criou".
func TestSoOLiderTrocaOEmblema(t *testing.T) {
	s, ctx := freshStore(t)
	lider := contaPix(ctx, t, s, "emb_lider")
	subLider := contaPix(ctx, t, s, "emb_sub")
	guildaComLider(ctx, t, s, "Lobos", 4001, lider, 9)
	guildaComLider(ctx, t, s, "Sub", 4002, subLider, 8) // sub-líder, não líder

	if err := s.SalvarEmblemaDaGuilda(ctx, lider, 4001, bmpDoCliente()); err != nil {
		t.Fatalf("o líder não conseguiu trocar: %v", err)
	}
	// O sub-líder da própria guilda não troca.
	if err := s.SalvarEmblemaDaGuilda(ctx, subLider, 4002, bmpDoCliente()); !errors.Is(err, ErrNaoLideraAGuilda) {
		t.Fatalf("sub-líder: erro = %v, queria ErrNaoLideraAGuilda", err)
	}
	// E o líder de uma guilda não troca o emblema de OUTRA.
	if err := s.SalvarEmblemaDaGuilda(ctx, lider, 4002, bmpDoCliente()); !errors.Is(err, ErrNaoLideraAGuilda) {
		t.Fatalf("guilda alheia: erro = %v, queria ErrNaoLideraAGuilda", err)
	}
	// Guilda que não existe dá a MESMA resposta, para não contar o que existe.
	if err := s.SalvarEmblemaDaGuilda(ctx, lider, 4999, bmpDoCliente()); !errors.Is(err, ErrNaoLideraAGuilda) {
		t.Fatalf("guilda inexistente: erro = %v, queria ErrNaoLideraAGuilda", err)
	}
}

// TestEmblemaVoltaComoFoiGravado: o que a página mostra tem de ser byte a byte o que
// o líder mandou — é o jogo que lê esses bytes.
func TestEmblemaVoltaComoFoiGravado(t *testing.T) {
	s, ctx := freshStore(t)
	conta := contaPix(ctx, t, s, "emb_volta")
	guildaComLider(ctx, t, s, "Voltando", 4010, conta, 9)

	quero := bmpDoCliente()
	if err := s.SalvarEmblemaDaGuilda(ctx, conta, 4010, quero); err != nil {
		t.Fatal(err)
	}
	img, em, tem, err := s.EmblemaDaGuilda(ctx, 4010)
	if err != nil {
		t.Fatal(err)
	}
	if !tem {
		t.Fatal("gravou e voltou sem emblema")
	}
	if len(img) != len(quero) {
		t.Fatalf("tamanho = %d, queria %d", len(img), len(quero))
	}
	for i := range quero {
		if img[i] != quero[i] {
			t.Fatalf("byte %d = %#x, queria %#x", i, img[i], quero[i])
		}
	}
	if em.IsZero() {
		t.Error("a hora da troca não foi gravada — o Last-Modified da imagem sai dela")
	}
}

// TestEmblemaInvalidoNaoEntra: a conferência é do SERVIDOR. Um cliente remendado não
// pode gravar imagem que o jogo não desenha.
func TestEmblemaInvalidoNaoEntra(t *testing.T) {
	s, ctx := freshStore(t)
	conta := contaPix(ctx, t, s, "emb_invalido")
	guildaComLider(ctx, t, s, "Invalida", 4020, conta, 9)

	ruim := bmpDoCliente()
	binary.LittleEndian.PutUint16(ruim[28:], 32) // 32 bits
	if err := s.SalvarEmblemaDaGuilda(ctx, conta, 4020, ruim); !errors.Is(err, ErrEmblemaInvalido) {
		t.Fatalf("erro = %v, queria ErrEmblemaInvalido", err)
	}
	if _, _, tem, _ := s.EmblemaDaGuilda(ctx, 4020); tem {
		t.Error("a imagem inválida entrou")
	}
}

// TestUmaTrocaPorGuildaPorPrazo: o limite é por GUILDA, e o REMOVER conta como
// troca — senão alternar pôr-e-tirar burlaria o limite.
func TestUmaTrocaPorGuildaPorPrazo(t *testing.T) {
	s, ctx := freshStore(t)
	conta := contaPix(ctx, t, s, "emb_prazo")
	guildaComLider(ctx, t, s, "Prazo", 4030, conta, 9)

	if err := s.SalvarEmblemaDaGuilda(ctx, conta, 4030, bmpDoCliente()); err != nil {
		t.Fatal(err)
	}
	if err := s.SalvarEmblemaDaGuilda(ctx, conta, 4030, bmpDoCliente()); !errors.Is(err, ErrEmblemaMuitoCedo) {
		t.Fatalf("segunda troca seguida: erro = %v, queria ErrEmblemaMuitoCedo", err)
	}
	// E o remover também espera.
	if err := s.LimparEmblemaDaGuilda(ctx, conta, 4030); !errors.Is(err, ErrEmblemaMuitoCedo) {
		t.Fatalf("remover logo depois: erro = %v, queria ErrEmblemaMuitoCedo", err)
	}

	// Passado o prazo, a troca passa. O relógio anda no banco, sem dormir o teste.
	if _, err := s.pool.Exec(ctx,
		`UPDATE guild SET emblema_trocado_em = now() - $2::interval WHERE id = $1`,
		4030, IntervaloEntreTrocasDeEmblema+time.Minute); err != nil {
		t.Fatal(err)
	}
	if err := s.LimparEmblemaDaGuilda(ctx, conta, 4030); err != nil {
		t.Fatalf("depois do prazo: %v", err)
	}
	if _, _, tem, _ := s.EmblemaDaGuilda(ctx, 4030); tem {
		t.Error("o remover não tirou o emblema")
	}
}

// TestGuildasQueLideroSoTrazAsMinhas, e traz TODAS: uma conta tem até quatro
// personagens, e nada impede dois deles liderarem guildas diferentes.
func TestGuildasQueLideroSoTrazAsMinhas(t *testing.T) {
	s, ctx := freshStore(t)
	minha := contaPix(ctx, t, s, "emb_lista")
	outra := contaPix(ctx, t, s, "emb_lista_outra")
	guildaComLider(ctx, t, s, "Alfa", 4040, minha, 9)
	guildaComLider(ctx, t, s, "Beta", 4041, outra, 9)
	// Segundo personagem da MESMA conta, liderando outra guilda.
	if _, err := s.pool.Exec(ctx, `INSERT INTO guild (id, name) VALUES (4042, 'Gama')`); err != nil {
		t.Fatal(err)
	}
	var charID int64
	if err := s.pool.QueryRow(ctx, `
		INSERT INTO character (account_id, slot, name, class, level, guild_id, guild_level)
		VALUES ($1, 1, 'segundo', 1, 50, 4042, 9) RETURNING id`, minha).Scan(&charID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.pool.Exec(ctx, `
		INSERT INTO guild_member (guild_id, character_id, account_id, slot, name, guild_level)
		VALUES (4042, $1, $2, 1, 'segundo', 9)`, charID, minha); err != nil {
		t.Fatal(err)
	}
	if err := s.SalvarEmblemaDaGuilda(ctx, minha, 4040, bmpDoCliente()); err != nil {
		t.Fatal(err)
	}

	lista, err := s.GuildasQueLidero(ctx, minha)
	if err != nil {
		t.Fatalf("GuildasQueLidero: %v", err)
	}
	if len(lista) != 2 {
		t.Fatalf("guildas = %d (%v), queria 2 — os dois personagens da conta", len(lista), lista)
	}
	porID := map[uint16]GuildaQueLidero{}
	for _, g := range lista {
		porID[g.ID] = g
	}
	if !porID[4040].TemBrasao {
		t.Error("a guilda com emblema veio sem a marca")
	}
	if porID[4042].TemBrasao {
		t.Error("a guilda sem emblema veio marcada")
	}
	if _, temAlheia := porID[4041]; temAlheia {
		t.Error("a lista trouxe guilda de outra conta")
	}
}

// TestEmblemaDeGuildaQueNaoExiste: para quem lê a imagem, "não existe" e "não tem"
// pedem a mesma resposta — e uma rota de imagem não pode virar 500 por isso.
func TestEmblemaDeGuildaQueNaoExiste(t *testing.T) {
	s, ctx := freshStore(t)
	_, _, tem, err := s.EmblemaDaGuilda(ctx, 4999)
	if err != nil {
		t.Fatalf("erro = %v, queria nenhum", err)
	}
	if tem {
		t.Error("guilda inexistente devolveu emblema")
	}
}
