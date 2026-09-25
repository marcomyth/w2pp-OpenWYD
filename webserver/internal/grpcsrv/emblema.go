package grpcsrv

import (
	"context"
	"errors"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	webv1 "github.com/jeanluca/w2pp-openwyd/api/web/v1"
	"github.com/jeanluca/w2pp-openwyd/internal/store"
)

// Emblemas é a superfície do emblema de guilda que o servidor usa.
type Emblemas interface {
	GuildasQueLidero(ctx context.Context, accountID int64) ([]store.GuildaQueLidero, error)
	SalvarEmblemaDaGuilda(ctx context.Context, accountID int64, guildID uint16, imagem []byte) error
	LimparEmblemaDaGuilda(ctx context.Context, accountID int64, guildID uint16) error
	EmblemaDaGuilda(ctx context.Context, guildID uint16) ([]byte, time.Time, bool, error)
}

// ListMyLedGuilds lista as guildas lideradas por algum personagem da conta.
//
// Quem está logado no site é a CONTA, e a liderança é do PERSONAGEM: o servidor
// resolve isso, e o site não precisa saber qual personagem lidera qual guilda.
func (s *CharacterServer) ListMyLedGuilds(ctx context.Context, req *webv1.ListMyLedGuildsRequest) (*webv1.ListMyLedGuildsResponse, error) {
	if s.emblemas == nil {
		return nil, status.Error(codes.Unimplemented, "emblema nao ligado neste servidor")
	}
	guildas, err := s.emblemas.GuildasQueLidero(ctx, req.GetAccountId())
	if err != nil {
		return nil, status.Errorf(codes.Internal, "listar guildas que lidero: %v", err)
	}
	out := make([]*webv1.LedGuild, 0, len(guildas))
	for _, g := range guildas {
		out = append(out, &webv1.LedGuild{
			GuildId:   int32(g.ID),
			Name:      g.Nome,
			HasEmblem: g.TemBrasao,
		})
	}
	return &webv1.ListMyLedGuildsResponse{Guilds: out}, nil
}

// SetMyGuildEmblem troca o emblema da guilda.
func (s *CharacterServer) SetMyGuildEmblem(ctx context.Context, req *webv1.SetMyGuildEmblemRequest) (*webv1.SetMyGuildEmblemResponse, error) {
	if s.emblemas == nil {
		return nil, status.Error(codes.Unimplemented, "emblema nao ligado neste servidor")
	}
	id, ok := guildaDoPedido(req.GetGuildId())
	if !ok {
		// Número fora da faixa de guilda não é erro de sistema: é pedido inválido, e
		// a resposta é a mesma de "não é sua" para não contar o que existe.
		return &webv1.SetMyGuildEmblemResponse{Result: webv1.GuildEmblemResult_GUILD_EMBLEM_RESULT_NOT_LEADER}, nil
	}
	err := s.emblemas.SalvarEmblemaDaGuilda(ctx, req.GetAccountId(), id, req.GetImage())
	if r, trata := resultadoDoEmblema(err); trata {
		return &webv1.SetMyGuildEmblemResponse{Result: r}, nil
	}
	return nil, status.Errorf(codes.Internal, "gravar emblema: %v", err)
}

// ClearMyGuildEmblem tira o emblema da guilda.
func (s *CharacterServer) ClearMyGuildEmblem(ctx context.Context, req *webv1.ClearMyGuildEmblemRequest) (*webv1.ClearMyGuildEmblemResponse, error) {
	if s.emblemas == nil {
		return nil, status.Error(codes.Unimplemented, "emblema nao ligado neste servidor")
	}
	id, ok := guildaDoPedido(req.GetGuildId())
	if !ok {
		return &webv1.ClearMyGuildEmblemResponse{Result: webv1.GuildEmblemResult_GUILD_EMBLEM_RESULT_NOT_LEADER}, nil
	}
	err := s.emblemas.LimparEmblemaDaGuilda(ctx, req.GetAccountId(), id)
	if r, trata := resultadoDoEmblema(err); trata {
		return &webv1.ClearMyGuildEmblemResponse{Result: r}, nil
	}
	return nil, status.Errorf(codes.Internal, "limpar emblema: %v", err)
}

// GetGuildEmblem devolve a imagem. É público: é por ele que a página da guilda
// mostra o emblema.
func (s *CharacterServer) GetGuildEmblem(ctx context.Context, req *webv1.GetGuildEmblemRequest) (*webv1.GetGuildEmblemResponse, error) {
	if s.emblemas == nil {
		return nil, status.Error(codes.Unimplemented, "emblema nao ligado neste servidor")
	}
	id, ok := guildaDoPedido(req.GetGuildId())
	if !ok {
		// Número inválido devolve "não tem", e não erro: quem pede a imagem trata as
		// duas coisas do mesmo jeito, e um erro aqui viraria 500 numa rota de imagem.
		return &webv1.GetGuildEmblemResponse{}, nil
	}
	img, em, tem, err := s.emblemas.EmblemaDaGuilda(ctx, id)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "ler emblema: %v", err)
	}
	if !tem {
		return &webv1.GetGuildEmblemResponse{}, nil
	}
	var quando int64
	if !em.IsZero() {
		quando = em.Unix()
	}
	return &webv1.GetGuildEmblemResponse{HasEmblem: true, Image: img, UpdatedAtUnix: quando}, nil
}

// guildaDoPedido estreita o número do site para o ushort do jogo.
//
// A FAIXA É DO LEGADO: o id de guilda cabe num ushort e nunca é zero (a migração
// 0012 tem o CHECK). Um int32 que não cabe ali não é uma guilda, e mandá-lo para o
// banco como uint16 estouraria em silêncio, virando OUTRA guilda.
func guildaDoPedido(id int32) (uint16, bool) {
	if id <= 0 || id >= 65536 {
		return 0, false
	}
	return uint16(id), true
}

// resultadoDoEmblema traduz as recusas previstas do store.
//
// Devolve DOIS valores para que um erro desconhecido continue virando Internal: com
// um só, um erro novo do banco viraria silenciosamente "você não lidera", e a pessoa
// procuraria o problema na própria conta.
func resultadoDoEmblema(err error) (webv1.GuildEmblemResult, bool) {
	switch {
	case err == nil:
		return webv1.GuildEmblemResult_GUILD_EMBLEM_RESULT_OK, true
	case errors.Is(err, store.ErrNaoLideraAGuilda):
		return webv1.GuildEmblemResult_GUILD_EMBLEM_RESULT_NOT_LEADER, true
	case errors.Is(err, store.ErrEmblemaInvalido):
		return webv1.GuildEmblemResult_GUILD_EMBLEM_RESULT_INVALID_IMAGE, true
	case errors.Is(err, store.ErrEmblemaMuitoCedo):
		return webv1.GuildEmblemResult_GUILD_EMBLEM_RESULT_RATE_LIMITED, true
	}
	return webv1.GuildEmblemResult_GUILD_EMBLEM_RESULT_UNSPECIFIED, false
}
