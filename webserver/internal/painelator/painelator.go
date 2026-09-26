// Package painelator identifica quem está agindo pelo painel administrativo.
//
// POR QUE ISTO EXISTE: os serviços de administração do webServer autorizam pelo
// `moderator_id`, que é uma conta DE JOGO. Desde o #130 a staff entra no painel como
// USUÁRIO DO PAINEL, sem conta de jogo, e o painel manda zero — o `authorize` recusa na
// primeira linha e as oito páginas de administração param.
//
// O id do usuário do painel chega por METADADO gRPC, e não por campo no proto, porque
// mexer em `api/` faria o tmServer reconstruir e derrubaria quem está jogando. O campo no
// proto continua sendo o certo e vem na próxima atualização.
//
// POR QUE A CONSULTA MORA AQUI e não no internal/store, que seria o lugar natural: o
// internal/store é compartilhado com o tmServer, e tocá-lo reconstrói o jogo pelo mesmo
// motivo. Uma consulta de dez linhas neste pacote é o preço de o conserto subir sem
// reinício.
package painelator

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// HeaderPainelUsuario é o cabeçalho que o adminServer manda. Tem de casar, letra por
// letra, com o gamedata.HeaderPainelUsuario do outro lado — são dois processos, e não há
// compilador que ligue os dois.
const HeaderPainelUsuario = "x-w2pp-painel-usuario"

// Ator é quem está agindo pelo painel, já conferido contra o banco.
type Ator struct {
	ID    int64
	Login string
	Papel string // 'admin' ou 'moderator'
}

// Leitor confere um usuário do painel no banco.
type Leitor struct{ pool *pgxpool.Pool }

// Novo monta o leitor sobre o pool que o webServer já tem.
func Novo(pool *pgxpool.Pool) *Leitor { return &Leitor{pool: pool} }

// ErrNaoServe é o usuário que não existe, ou que existe e está DESATIVADO.
//
// UM ERRO SÓ PARA OS DOIS CASOS, de propósito: quem chama não precisa distinguir, e
// distinguir na resposta contaria a um chamador não autenticado quais ids existem.
var ErrNaoServe = errors.New("painelator: usuario do painel invalido ou desativado")

// Confere lê o usuário AGORA, a cada chamada.
//
// A CADA CHAMADA E SEM CACHE, e isto é escolha: desativar alguém no painel tem de valer
// no próximo clique dela, não no próximo reinício. É uma consulta por chave primária, e o
// custo dela é menor que o de descobrir, depois, que alguém demitido continuou editando o
// jogo por dez minutos.
//
// O `ativo` é conferido AQUI e não por quem chama: é a única coisa que separa um usuário
// demitido de um em exercício, e deixá-la para o chamador é como um caminho esquece.
func (l *Leitor) Confere(ctx context.Context, id int64) (Ator, error) {
	if l == nil || l.pool == nil || id <= 0 {
		return Ator{}, ErrNaoServe
	}
	var a Ator
	var ativo bool
	err := l.pool.QueryRow(ctx, `
		SELECT id, login, papel, ativo FROM painel_usuario WHERE id = $1`, id).
		Scan(&a.ID, &a.Login, &a.Papel, &ativo)
	if errors.Is(err, pgx.ErrNoRows) {
		return Ator{}, ErrNaoServe
	}
	if err != nil {
		return Ator{}, fmt.Errorf("painelator: lendo o usuario %d: %w", id, err)
	}
	if !ativo {
		return Ator{}, ErrNaoServe
	}
	return a, nil
}

type chave struct{}

// NoContexto guarda o ator conferido, para os serviços lerem.
func NoContexto(ctx context.Context, a Ator) context.Context {
	return context.WithValue(ctx, chave{}, a)
}

// Do devolve o ator do painel, se houver. O segundo valor é falso quando a chamada não
// veio de um usuário do painel — pode ser uma conta de jogo, que segue pelo caminho de
// sempre.
func Do(ctx context.Context) (Ator, bool) {
	a, ok := ctx.Value(chave{}).(Ator)
	return a, ok
}

// PapelValido diz se o papel deixa administrar. Os dois que o painel emite.
func PapelValido(papel string) bool { return papel == "admin" || papel == "moderator" }

// AutorizaPeloPainel responde "esta chamada veio de um usuário do painel em exercício?".
//
// Devolve (podeAdministrar, veioDoPainel). O segundo valor é o que importa para quem
// chama: FALSO quer dizer "não é usuário do painel", e aí o caminho de sempre — a conta
// de jogo — continua valendo. Sem esse segundo valor, quem chama não distinguiria
// "recusado" de "não é do painel", e trataria toda conta de jogo como recusa.
//
// O ativo já foi conferido no interceptador, contra o banco, nesta mesma chamada: o que
// está no contexto é um ator em exercício ou não é nada.
func AutorizaPeloPainel(ctx context.Context) (pode, veioDoPainel bool) {
	a, ok := Do(ctx)
	if !ok {
		return false, false
	}
	return PapelValido(a.Papel), true
}
