package grpcsrv

import (
	"context"
	"errors"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	webv1 "github.com/jeanluca/w2pp-openwyd/api/web/v1"
	"github.com/jeanluca/w2pp-openwyd/internal/store"
)

// emblemasFake responde o que o teste mandar, e guarda o que recebeu: o id que
// chega ao store é metade do que este arquivo prova.
type emblemasFake struct {
	guildas   []store.GuildaQueLidero
	erro      error
	imagem    []byte
	trocadoEm time.Time
	tem       bool

	guildaPedida uint16
	contaPedida  int64
}

func (f *emblemasFake) GuildasQueLidero(_ context.Context, accountID int64) ([]store.GuildaQueLidero, error) {
	f.contaPedida = accountID
	return f.guildas, f.erro
}

func (f *emblemasFake) SalvarEmblemaDaGuilda(_ context.Context, accountID int64, guildID uint16, _ []byte) error {
	f.contaPedida, f.guildaPedida = accountID, guildID
	return f.erro
}

func (f *emblemasFake) LimparEmblemaDaGuilda(_ context.Context, accountID int64, guildID uint16) error {
	f.contaPedida, f.guildaPedida = accountID, guildID
	return f.erro
}

func (f *emblemasFake) EmblemaDaGuilda(_ context.Context, guildID uint16) ([]byte, time.Time, bool, error) {
	f.guildaPedida = guildID
	return f.imagem, f.trocadoEm, f.tem, f.erro
}

func servidorDeEmblema(f *emblemasFake) *CharacterServer {
	return NewCharacters(nil).ComEmblemas(f)
}

// TestCadaRecusaTemSeuResultado: a página diz coisas diferentes para "você não
// lidera", "essa imagem não serve" e "espere um pouco". Se as três virassem a mesma,
// o líder ficaria trocando a imagem tentando adivinhar qual era o problema.
func TestCadaRecusaTemSeuResultado(t *testing.T) {
	casos := []struct {
		nome string
		erro error
		quer webv1.GuildEmblemResult
	}{
		{"passou", nil, webv1.GuildEmblemResult_GUILD_EMBLEM_RESULT_OK},
		{"nao lidera", store.ErrNaoLideraAGuilda, webv1.GuildEmblemResult_GUILD_EMBLEM_RESULT_NOT_LEADER},
		{"imagem ruim", store.ErrEmblemaInvalido, webv1.GuildEmblemResult_GUILD_EMBLEM_RESULT_INVALID_IMAGE},
		{"muito cedo", store.ErrEmblemaMuitoCedo, webv1.GuildEmblemResult_GUILD_EMBLEM_RESULT_RATE_LIMITED},
	}
	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			f := &emblemasFake{erro: c.erro}
			s := servidorDeEmblema(f)
			resp, err := s.SetMyGuildEmblem(context.Background(), &webv1.SetMyGuildEmblemRequest{
				AccountId: 7, GuildId: 12, Image: []byte("nao importa aqui"),
			})
			if err != nil {
				t.Fatalf("erro de gRPC inesperado: %v", err)
			}
			if resp.GetResult() != c.quer {
				t.Errorf("result = %v, queria %v", resp.GetResult(), c.quer)
			}
			// E o pedido chegou ao store com a conta e a guilda que vieram.
			if f.contaPedida != 7 || f.guildaPedida != 12 {
				t.Errorf("o store recebeu conta %d guilda %d", f.contaPedida, f.guildaPedida)
			}
		})
	}
}

// TestErroDesconhecidoViraInternal: um erro novo do banco não pode virar "você não
// lidera". A pessoa procuraria o problema na própria conta, e não existe.
func TestErroDesconhecidoViraInternal(t *testing.T) {
	f := &emblemasFake{erro: errors.New("o banco caiu")}
	s := servidorDeEmblema(f)
	if _, err := s.SetMyGuildEmblem(context.Background(), &webv1.SetMyGuildEmblemRequest{
		AccountId: 7, GuildId: 12, Image: []byte("x"),
	}); status.Code(err) != codes.Internal {
		t.Errorf("código = %v, queria Internal", status.Code(err))
	}
	if _, err := s.ClearMyGuildEmblem(context.Background(), &webv1.ClearMyGuildEmblemRequest{
		AccountId: 7, GuildId: 12,
	}); status.Code(err) != codes.Internal {
		t.Errorf("no limpar: código = %v, queria Internal", status.Code(err))
	}
}

// TestGuildaForaDaFaixaNaoChegaAoBanco: o id do jogo é um ushort e nunca é zero.
// Um int32 que não cabe ali não é guilda, e estreitá-lo em silêncio viraria OUTRA
// guilda — trocar o emblema da guilda errada é o pior resultado possível aqui.
func TestGuildaForaDaFaixaNaoChegaAoBanco(t *testing.T) {
	for _, id := range []int32{0, -1, 65536, 65537, 131072} {
		f := &emblemasFake{}
		s := servidorDeEmblema(f)
		resp, err := s.SetMyGuildEmblem(context.Background(), &webv1.SetMyGuildEmblemRequest{
			AccountId: 7, GuildId: id, Image: []byte("x"),
		})
		if err != nil {
			t.Fatalf("id %d: erro %v", id, err)
		}
		if resp.GetResult() != webv1.GuildEmblemResult_GUILD_EMBLEM_RESULT_NOT_LEADER {
			t.Errorf("id %d: result = %v", id, resp.GetResult())
		}
		if f.guildaPedida != 0 || f.contaPedida != 0 {
			t.Errorf("id %d chegou ao store como %d", id, f.guildaPedida)
		}
	}
}

// TestImagemVoltaComOQuando: o Last-Modified da rota de imagem do site sai daqui.
func TestImagemVoltaComOQuando(t *testing.T) {
	quando := time.Date(2026, 9, 25, 12, 0, 0, 0, time.UTC)
	f := &emblemasFake{tem: true, imagem: []byte("bmp"), trocadoEm: quando}
	s := servidorDeEmblema(f)
	resp, err := s.GetGuildEmblem(context.Background(), &webv1.GetGuildEmblemRequest{GuildId: 12})
	if err != nil {
		t.Fatal(err)
	}
	if !resp.GetHasEmblem() || string(resp.GetImage()) != "bmp" {
		t.Fatalf("resposta = %+v", resp)
	}
	if resp.GetUpdatedAtUnix() != quando.Unix() {
		t.Errorf("updated_at = %d, queria %d", resp.GetUpdatedAtUnix(), quando.Unix())
	}
}

// TestSemEmblemaNaoMandaImagem: o booleano separado da imagem existe para a página
// distinguir "não tem" de "falhei ao ler". Mandar bytes vazios com has_emblem falso
// é o contrato.
func TestSemEmblemaNaoMandaImagem(t *testing.T) {
	s := servidorDeEmblema(&emblemasFake{tem: false})
	resp, err := s.GetGuildEmblem(context.Background(), &webv1.GetGuildEmblemRequest{GuildId: 12})
	if err != nil {
		t.Fatal(err)
	}
	if resp.GetHasEmblem() || len(resp.GetImage()) != 0 || resp.GetUpdatedAtUnix() != 0 {
		t.Errorf("resposta = %+v, queria tudo vazio", resp)
	}
}

// TestGuildaInvalidaNaLeituraNaoEErro: uma rota de imagem não pode virar 500 por um
// número torto na URL.
func TestGuildaInvalidaNaLeituraNaoEErro(t *testing.T) {
	f := &emblemasFake{tem: true, imagem: []byte("bmp")}
	s := servidorDeEmblema(f)
	resp, err := s.GetGuildEmblem(context.Background(), &webv1.GetGuildEmblemRequest{GuildId: 99999})
	if err != nil {
		t.Fatalf("erro = %v, queria nenhum", err)
	}
	if resp.GetHasEmblem() {
		t.Error("id fora da faixa devolveu emblema")
	}
	if f.guildaPedida != 0 {
		t.Errorf("chegou ao store como %d", f.guildaPedida)
	}
}

// TestListaDasGuildasQueLidero traz o nome e se já tem emblema, que é o que a tela
// precisa para montar a escolha.
func TestListaDasGuildasQueLidero(t *testing.T) {
	f := &emblemasFake{guildas: []store.GuildaQueLidero{
		{ID: 12, Nome: "Alfa", TemBrasao: true},
		{ID: 40, Nome: "Beta"},
	}}
	s := servidorDeEmblema(f)
	resp, err := s.ListMyLedGuilds(context.Background(), &webv1.ListMyLedGuildsRequest{AccountId: 7})
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.GetGuilds()) != 2 {
		t.Fatalf("guildas = %d", len(resp.GetGuilds()))
	}
	g := resp.GetGuilds()[0]
	if g.GetGuildId() != 12 || g.GetName() != "Alfa" || !g.GetHasEmblem() {
		t.Errorf("primeira = %+v", g)
	}
	if resp.GetGuilds()[1].GetHasEmblem() {
		t.Error("a segunda veio marcada com emblema")
	}
	if f.contaPedida != 7 {
		t.Errorf("a conta da sessão não chegou ao store: %d", f.contaPedida)
	}
}

// TestSemEmblemaLigadoRespondeUnimplemented: um servidor montado sem o emblema tem de
// DIZER isso. Responder "guilda nenhuma" faria o site desenhar uma tela vazia como
// se fosse verdade.
func TestSemEmblemaLigadoRespondeUnimplemented(t *testing.T) {
	s := NewCharacters(nil)
	if _, err := s.ListMyLedGuilds(context.Background(), &webv1.ListMyLedGuildsRequest{}); status.Code(err) != codes.Unimplemented {
		t.Errorf("listar: código = %v", status.Code(err))
	}
	if _, err := s.SetMyGuildEmblem(context.Background(), &webv1.SetMyGuildEmblemRequest{}); status.Code(err) != codes.Unimplemented {
		t.Errorf("gravar: código = %v", status.Code(err))
	}
	if _, err := s.ClearMyGuildEmblem(context.Background(), &webv1.ClearMyGuildEmblemRequest{}); status.Code(err) != codes.Unimplemented {
		t.Errorf("limpar: código = %v", status.Code(err))
	}
	if _, err := s.GetGuildEmblem(context.Background(), &webv1.GetGuildEmblemRequest{}); status.Code(err) != codes.Unimplemented {
		t.Errorf("ler: código = %v", status.Code(err))
	}
}
