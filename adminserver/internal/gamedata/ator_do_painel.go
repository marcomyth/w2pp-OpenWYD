package gamedata

import (
	"context"
	"strconv"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

// O ATOR DO PAINEL, viajando por METADADO e não pelo proto.
//
// O PROBLEMA: os serviços de administração do webServer autorizam por
// `moderator_id`, que é uma conta DE JOGO. Desde o #130 a staff entra no painel como
// usuário do painel, sem conta de jogo, e o painel manda 0 — o webServer recusa antes de
// olhar qualquer coisa, e as páginas de NPC, Monstros, Atributos, Recompensa Diária,
// Loja de Donate, Receita, Eventos do Mundo e Preços de item param todas.
//
// POR QUE METADADO E NÃO UM CAMPO NO web.proto, que seria o lugar natural: mexer em
// `api/` faz o tmServer reconstruir, e reconstruir o tmServer DERRUBA QUEM ESTÁ JOGANDO.
// Este conserto precisa subir sem reiniciar o jogo. O metadado atravessa só o
// adminServer e o webServer, que é exatamente o alcance do problema.
//
// O campo no proto continua sendo o certo, e vem na próxima atualização, junto com o
// conserto das escritas que auditam dentro do internal/store.

// HeaderPainelUsuario é o cabeçalho gRPC que leva o id do usuário do painel.
//
// Nome longo e com prefixo próprio de propósito: metadado é um espaço plano e
// compartilhado, e um "usuario" solto colidiria com o primeiro proxy que resolvesse
// acrescentar o dele.
const HeaderPainelUsuario = "x-w2pp-painel-usuario"

type chaveDoAtor struct{}

// ComAtorDoPainel marca o contexto com quem está agindo pelo painel.
//
// Chamado UMA VEZ, no meio do caminho da requisição HTTP, e não em cada ponto que fala
// com o webServer: são mais de trinta chamadas, e a que alguém esquecer de marcar é
// justamente a que volta a recusar — sem ninguém ligar a falha a este commit.
func ComAtorDoPainel(ctx context.Context, painelUsuarioID int64) context.Context {
	if painelUsuarioID <= 0 {
		return ctx
	}
	return context.WithValue(ctx, chaveDoAtor{}, painelUsuarioID)
}

// AtorDoPainel lê o que o ComAtorDoPainel marcou. Zero quer dizer "não é usuário do
// painel" — pode ser uma conta de jogo, que segue pelo caminho de sempre.
func AtorDoPainel(ctx context.Context) int64 {
	id, _ := ctx.Value(chaveDoAtor{}).(int64)
	return id
}

// MandaAtorDoPainel põe o id no metadado de toda chamada que sair com o contexto
// marcado.
//
// UM INTERCEPTADOR E NÃO TRINTA CHAMADAS ALTERADAS: é a mesma razão do ComAtorDoPainel,
// e aqui ela é mais forte ainda, porque o esquecimento seria numa chamada específica de
// uma página específica.
func MandaAtorDoPainel() grpc.UnaryClientInterceptor {
	return func(ctx context.Context, method string, req, reply any,
		cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption,
	) error {
		if id := AtorDoPainel(ctx); id > 0 {
			ctx = metadata.AppendToOutgoingContext(ctx, HeaderPainelUsuario,
				strconv.FormatInt(id, 10))
		}
		return invoker(ctx, method, req, reply, cc, opts...)
	}
}
