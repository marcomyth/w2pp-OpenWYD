// Package authz decides who may call what on the web-api.
//
// The web-api carries eighteen services, and most of them administer the game:
// create and delete NPCs, set item prices and item stats, move donate balances,
// read revenue. Until this package existed it had one gate — mutual TLS — and
// that gate answers the wrong question. It says "is the caller one of our
// services", not "which one, and may it do this". With a single caller (the
// staff panel) the difference did not matter. It stops being free the moment a
// player-facing site becomes a second caller, because then the site's credential
// opens the game's administration too.
//
// Worse, the TLS gate degrades silently: secure.ServerCreds falls back to an
// insecure listener when no certificate is configured, so an unconfigured
// deployment serves every administrative call to anyone who can reach the port.
// The only thing standing in the way is the hosting's private network.
//
// The rule here is the one tmserver/internal/control already applies to the
// control API, quoted from its own doc: a service that starts without its
// credentials looks healthy and is not.
package authz

import (
	"context"
	"crypto/subtle"
	"errors"
	"strconv"
	"strings"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	webv1 "github.com/jeanluca/w2pp-openwyd/api/web/v1"
	"github.com/jeanluca/w2pp-openwyd/webserver/internal/painelator"
)

// TokenHeader is the metadata key carrying the caller's key. Re-exported from
// the proto package, where it has to live so the panel can name it too.
const TokenHeader = webv1.TokenHeader

// servicosDoJogador are the services a player-facing site legitimately calls:
// sign-up and login, its own characters, rankings, the item catalog it renders,
// the three that move a player's own donate balance, and the seller's own Pix
// receiving key.
//
// This is an ALLOWLIST, and that is the point. Everything not named here needs
// the panel's key, so a service somebody adds next year is closed from birth and
// has to be opened deliberately. The failure of forgetting is a caller getting a
// loud Unauthenticated, never a door left open quietly.
//
// Names are the proto service names, without the package: the full method looks
// like /web.v1.NpcAdminService/UpsertNpc.
var servicosDoJogador = map[string]bool{
	"AccountWebService":   true,
	"RankingWebService":   true,
	"CharacterWebService": true,
	"ItemCatalogService":  true,
	"DonateShopService":   true,
	"DailyRewardService":  true,
	"DonateTopupService":  true,
	// RmtWebService: o vendedor cadastra a chave Pix DELE, na conta DELE, pelo
	// site. É dado do próprio jogador, como o saldo de doação que já está nesta
	// lista, e não há caminho por onde a chave de outra conta apareça — a leitura
	// devolve sempre mascarada e a escrita recusa com cobrança aberta, as duas
	// travas no servidor e não na tela. Exigir a chave do painel aqui obrigaria o
	// site a passar pela staff para o jogador preencher o próprio formulário.
	"RmtWebService": true,
}

// servicosDeSistema are the services one machine calls on another machine's
// behalf, with no person behind the request.
//
// A THIRD LIST, and not a corner of servicosDoJogador, because the caller is
// different in kind. The player list is "things a browser-driven request can
// cause"; this one is "things the site's server does when the payment processor
// talks to it". They happen to live in the same process today and that is an
// accident of deployment, not a reason to share a key.
//
// The concrete benefit is small and real: if a bug in the player-facing side ever
// let somebody drive arbitrary web-api calls with the site key — an SSRF, a
// confused path, a leaked header — the payment-notification door would still be
// shut. A key that opens everything the site can reach is not a key.
var servicosDeSistema = map[string]bool{
	// RmtSystemService: o webhook da processadora é da CONTA e não da cobrança,
	// então o aviso de uma venda em dinheiro real chega ao SITE. Ele repassa, e
	// este servidor não acredita no repasse — vai perguntar à processadora. Não há
	// pessoa nenhuma nesta chamada.
	"RmtSystemService": true,
}

// DoSistema reports whether a proto service name is one the site's server-side
// system key may reach.
func DoSistema(servico string) bool { return servicosDeSistema[servico] }

// DoJogador reports whether a proto service name is one a player-facing caller
// may reach. Exported for the guard test that keeps this list and the .proto
// from drifting apart.
func DoJogador(servico string) bool { return servicosDoJogador[servico] }

// Chaves are the keys the web-api accepts, one per caller.
//
// Two callers and not one shared secret, because the whole point is that the
// site's key must not open the administration. A key that opens everything is
// not a key, it is the absence of one.
type Chaves struct {
	// Painel opens every service. Held by the staff panel.
	Painel string
	// Site opens only servicosDoJogador. Held by the player-facing site, which
	// does not exist yet — an empty value simply matches nobody.
	Site string
	// Sistema opens only servicosDeSistema. Held by the site's SERVER side, for
	// the calls that no person triggers.
	//
	// Separate from Site even though the same deployment holds both: see
	// servicosDeSistema. Empty matches nobody, which is what a server without the
	// payment path configured should be.
	Sistema string
}

// Configurada reports whether any key was set. A web-api with none is the
// pre-existing behaviour: it serves everything to whoever reaches the port.
func (c Chaves) Configurada() bool {
	return strings.TrimSpace(c.Painel) != "" || strings.TrimSpace(c.Site) != "" ||
		strings.TrimSpace(c.Sistema) != ""
}

// Interceptor authenticates every call against the keys.
//
// With NO key configured it lets every call through. That is the migration step
// and nothing else: this has to be deployable to a running server before the
// keys exist on either side, so the panel does not break in the window between
// the two deploys. Once the keys are set, the follow-up change makes an
// unconfigured web-api refuse to start at all, which is the end state — the
// pass-through above is precisely the behaviour being removed.
func Interceptor(c Chaves, painel *painelator.Leitor) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (any, error) {
		if !c.Configurada() {
			return handler(ctx, req)
		}
		token, ok := tokenDe(ctx)
		if !ok {
			return nil, status.Error(codes.Unauthenticated, "missing web-api key")
		}
		switch {
		case confere(token, c.Painel):
			// SÓ NO RAMO DO PAINEL se lê o ator, e isso é a metade que importa da
			// segurança: o cabeçalho é um texto que qualquer chamador pode inventar.
			// Aqui ele só é olhado DEPOIS de a chamada já estar autenticada com a
			// chave do painel, que é um segredo. Lido no ramo do site, ele seria uma
			// porta para o site se dizer admin.
			ctx2, err := comAtorDoPainel(ctx, painel)
			if err != nil {
				return nil, err
			}
			// ENQUANTO A AUDITORIA NÃO CONHECE O USUÁRIO DO PAINEL, ele só lê.
			//
			// As escritas gravam o autor no internal/store com a conta de jogo, e as
			// tabelas de auditoria guardam esse número SEM chave estrangeira — a
			// escrita passaria e registraria "conta 0" como autor. Edição que funciona
			// e mente sobre quem a fez é pior que edição recusada.
			if _, doPainel := painelator.Do(ctx2); doPainel && !PainelPodeChamar(info.FullMethod) {
				return nil, status.Error(codes.PermissionDenied, MsgEscritaAindaNao)
			}
			return handler(ctx2, req)
		case confere(token, c.Site):
			if !DoJogador(servicoDe(info.FullMethod)) {
				// Named in the message on purpose: this is the line that says
				// the split is working, and the person reading the log is
				// usually somebody wondering why the site cannot do something.
				return nil, status.Errorf(codes.PermissionDenied,
					"the site key does not open %s", servicoDe(info.FullMethod))
			}
			return handler(ctx, req)
		case confere(token, c.Sistema):
			if !DoSistema(servicoDe(info.FullMethod)) {
				return nil, status.Errorf(codes.PermissionDenied,
					"the system key does not open %s", servicoDe(info.FullMethod))
			}
			return handler(ctx, req)
		default:
			return nil, status.Error(codes.Unauthenticated, "bad web-api key")
		}
	}
}

// StreamInterceptor is the same rule for streaming calls. None exist today, and
// one added later must not arrive unguarded.
func StreamInterceptor(c Chaves) grpc.StreamServerInterceptor {
	return func(srv any, ss grpc.ServerStream, info *grpc.StreamServerInfo,
		handler grpc.StreamHandler,
	) error {
		if !c.Configurada() {
			return handler(srv, ss)
		}
		token, ok := tokenDe(ss.Context())
		if !ok {
			return status.Error(codes.Unauthenticated, "missing web-api key")
		}
		switch {
		case confere(token, c.Painel):
			return handler(srv, ss)
		case confere(token, c.Site):
			if !DoJogador(servicoDe(info.FullMethod)) {
				return status.Errorf(codes.PermissionDenied,
					"the site key does not open %s", servicoDe(info.FullMethod))
			}
			return handler(srv, ss)
		case confere(token, c.Sistema):
			if !DoSistema(servicoDe(info.FullMethod)) {
				return status.Errorf(codes.PermissionDenied,
					"the system key does not open %s", servicoDe(info.FullMethod))
			}
			return handler(srv, ss)
		default:
			return status.Error(codes.Unauthenticated, "bad web-api key")
		}
	}
}

func tokenDe(ctx context.Context) (string, bool) {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return "", false
	}
	vals := md.Get(TokenHeader)
	if len(vals) != 1 {
		return "", false
	}
	return vals[0], true
}

// confere compares in constant time, and never matches an unset key.
//
// The empty check is load-bearing: Site is empty until the player site exists,
// and without it a caller sending an empty key would be granted the site's
// access. Constant time because the alternative leaks the key one byte at a
// time to anybody who can measure, and this endpoint edits the whole game.
func confere(recebido, esperado string) bool {
	if strings.TrimSpace(esperado) == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(recebido), []byte(esperado)) == 1
}

// servicoDe pulls the service name out of a full method
// (/web.v1.NpcAdminService/UpsertNpc → NpcAdminService). An unparseable one
// returns the empty string, which is in no allowlist — closed by default, here
// too.
func servicoDe(fullMethod string) string {
	partes := strings.Split(strings.TrimPrefix(fullMethod, "/"), "/")
	if len(partes) != 2 {
		return ""
	}
	if i := strings.LastIndex(partes[0], "."); i >= 0 {
		return partes[0][i+1:]
	}
	return partes[0]
}

// comAtorDoPainel lê o cabeçalho do usuário do painel e o confere no banco.
//
// SEM CABEÇALHO NÃO É ERRO: continua existindo quem entra no painel com conta de jogo, e
// essas chamadas seguem pelo caminho do moderator_id. O cabeçalho é um acréscimo, não uma
// exigência — exigi-lo quebraria o login por conta de jogo no mesmo dia.
//
// COM CABEÇALHO INVÁLIDO É ERRO, e na hora. Um id que não existe, ou de alguém
// DESATIVADO, não pode simplesmente "seguir sem ator": seguiria como conta zero e cairia
// na mesma recusa de hoje, com uma mensagem que não explica nada. Recusar aqui diz a
// verdade — quem foi desativado para de editar o jogo no próximo clique.
func comAtorDoPainel(ctx context.Context, leitor *painelator.Leitor) (context.Context, error) {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return ctx, nil
	}
	vals := md.Get(painelator.HeaderPainelUsuario)
	if len(vals) == 0 {
		return ctx, nil
	}
	id, err := strconv.ParseInt(vals[0], 10, 64)
	if err != nil || id <= 0 {
		return nil, status.Error(codes.Unauthenticated, "painel: id de usuario invalido")
	}
	if leitor == nil {
		// Sem banco ligado não há como conferir, e seguir sem conferir seria aceitar
		// um "sou admin" escrito pelo chamador.
		return nil, status.Error(codes.Unauthenticated, "painel: sem como conferir o usuario")
	}
	ator, err := leitor.Confere(ctx, id)
	if errors.Is(err, painelator.ErrNaoServe) {
		return nil, status.Error(codes.PermissionDenied,
			"painel: usuario invalido ou desativado")
	}
	if err != nil {
		return nil, status.Error(codes.Internal, "painel: nao consegui conferir o usuario")
	}
	return painelator.NoContexto(ctx, ator), nil
}
