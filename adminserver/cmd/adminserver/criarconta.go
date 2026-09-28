package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/jeanluca/w2pp-openwyd/adminserver/internal/accounts"
	"github.com/jeanluca/w2pp-openwyd/internal/secret"
	"github.com/jeanluca/w2pp-openwyd/internal/store"
)

// O SUBCOMANDO QUE O AMBIENTE TRANCADO EXIGE, E QUE NÃO EXISTIA.
//
// Com W2PP_ACESSO_RESTRITO ligado, o painel some com a rota de criar conta de jogo.
// Isso é deliberado e está explicado no painel: uma sessão esquecida aberta criaria
// conta de jogador num servidor que deveria estar trancado. O comentário de lá
// encaminha quem precisar para "a linha de comando, que exige a máquina".
//
// ESSA LINHA DE COMANDO NÃO EXISTIA. Só havia a de usuário do painel, que é outra
// coisa: usuário do painel administra e não entra no jogo. Ou seja, no ambiente
// trancado não havia caminho nenhum para a primeira conta de jogo da staff — nem
// pelo painel, que esconde a rota, nem pela linha, que não tinha o verbo. Este
// arquivo fecha esse buraco.
//
// TROCAR O CARGO CONTINUA SENDO PELO PAINEL, e de propósito: o painel mantém a
// troca de cargo funcionando mesmo trancado, justamente para alguém virar staff lá
// dentro. Este comando aceita -cargo só para não exigir duas ferramentas na primeira
// conta, quando ainda não há ninguém que possa clicar.
//
// A SENHA VEM PELA ENTRADA PADRÃO e nunca por argumento, pela mesma razão do
// criar-usuario: argumento aparece no histórico do shell e na lista de processos,
// que qualquer usuário da máquina lê.

// criarContaCmd é o verbo que main() reconhece antes de qualquer flag do servidor.
const criarContaCmd = "criar-conta"

// criarConta roda o subcomando. Recebe args JÁ sem o verbo.
func criarConta(logger *slog.Logger, args []string, entrada io.Reader) error {
	fs := flag.NewFlagSet(criarContaCmd, flag.ContinueOnError)
	dsn := fs.String("dsn", envOr("DATABASE_URL", os.Getenv("W2PP_DB_DSN")), "PostgreSQL DSN (ou DATABASE_URL)")
	login := fs.String("login", "", "nome da conta de jogo")
	cargo := fs.String("cargo", accounts.RoleModerator, "player, moderator ou admin")
	prazo := fs.Duration("prazo", 5*time.Minute, "quanto esperar pelo banco, migracoes incluidas")
	conferir := fs.Bool("conferir", false, "so confere: banco responde e o nome esta livre; nao cria nada")
	fs.Usage = func() {
		fmt.Fprintf(fs.Output(), `adminserver %s -login NOME [-cargo player|moderator|admin]

Cria uma conta de JOGO — a que entra no jogo com personagem. Para quem administra
sem personagem, use %s.

A SENHA VEM PELA ENTRADA PADRÃO, nunca por argumento:

    printf '%%s' 'a-senha-aqui' | adminserver %s -login marco -cargo moderator

A senha de JOGO tem de %d a %d caracteres, sem espaço, em ASCII visível. Ela é
digitada num cliente de 2003 e não é a mesma coisa que a senha do painel, que é
mais longa.

Serve para o ambiente trancado (W2PP_ACESSO_RESTRITO), onde o painel esconde a
criação de conta de propósito. Fora dele, use a tela Contas do painel.

`, criarContaCmd, criarUsuarioCmd, criarContaCmd, accounts.MinSenhaBytes, accounts.MaxSenhaBytes)
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *login == "" {
		fs.Usage()
		return errors.New("falta -login")
	}
	if *dsn == "" {
		return errors.New("falta o DSN: passe -dsn ou ponha DATABASE_URL no ambiente")
	}
	// O CARGO É CONFERIDO ANTES DE PEDIR A SENHA. Quem digitou o cargo errado descobre
	// agora, e não depois de digitar uma senha duas vezes.
	if !accounts.ValidRole(*cargo) {
		return fmt.Errorf("cargo %q nao existe; use %s, %s ou %s",
			*cargo, accounts.RolePlayer, accounts.RoleModerator, accounts.RoleAdmin)
	}
	if err := accounts.ValidarNome(*login); err != nil {
		return fmt.Errorf("nome de conta: %w", err)
	}

	senha, err := leSenhaDeJogoDaEntrada(entrada)
	if err != nil {
		return err
	}
	hash, err := secret.HashSecret(senha)
	if err != nil {
		return fmt.Errorf("preparando a senha: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), *prazo)
	defer cancel()
	pool, err := store.Pool(ctx, *dsn)
	if err != nil {
		return fmt.Errorf("abrindo o banco: %w", err)
	}
	defer pool.Close()

	// As migrações rodam aqui pela mesma razão do criar-usuario: num ambiente novo a
	// tabela pode não existir, e este comando é dos primeiros a rodar.
	if err := store.Migrate(ctx, pool); err != nil {
		return fmt.Errorf("aplicando as migracoes: %w", err)
	}

	st := accounts.New(pool)

	// -CONFERIR EXISTE PARA NAO ENTREGAR CAMINHO NAO TESTADO.
	//
	// Sem ele, a unica forma de saber que este comando funciona contra um ambiente de
	// verdade e criar uma conta de verdade — ou seja, inventar uma senha para alguem.
	// Com ele, da para provar o caminho inteiro (tunel, banco, migracoes, nome livre)
	// sem criar nada e sem ninguem digitar senha.
	//
	// E serve depois do teste tambem: conferir que o nome esta livre ANTES de digitar a
	// senha duas vezes e melhor que descobrir "nome em uso" no fim.
	if *conferir {
		// Buscar pelo nome exato: uma leitura de verdade no banco, que prova a
		// ligacao e as migracoes, e ainda responde se o nome esta livre.
		achados, err := st.Buscar(ctx, *login, 5)
		if err != nil {
			return fmt.Errorf("conferindo o nome: %w", err)
		}
		for _, a := range achados {
			if strings.EqualFold(a.Name, *login) {
				return fmt.Errorf("o nome %q ja esta em uso", *login)
			}
		}
		logger.Info("conferencia ok: o banco respondeu e o nome esta livre",
			"conta", *login, "cargo", *cargo)
		return nil
	}

	id, err := st.Criar(ctx, *login, hash, "")
	switch {
	case errors.Is(err, accounts.ErrNomeEmUso):
		return fmt.Errorf("já existe uma conta com esse nome: %w", err)
	case err != nil:
		return fmt.Errorf("criando a conta: %w", err)
	}
	// O LOG NÃO DIZ A SENHA nem o tamanho dela, igual ao criar-usuario: quem lê o
	// terminal por cima do ombro não ganha nada.
	logger.Info("conta de jogo criada", "id", id, "conta", *login)

	if *cargo != accounts.RolePlayer {
		// O ATOR VAI COMO ZERO, e aqui isso é seguro — conferido, não suposto:
		// SetRole usa o ator SÓ para recusar que alguém mude o próprio cargo, e não
		// grava esse número em lugar nenhum. Zero nunca vira "autor zero" numa linha
		// de auditoria por este caminho. A auditoria de cargo é escrita pelo painel,
		// que tem o usuário do painel de verdade para pôr nela.
		if _, err := st.SetRole(ctx, 0, id, *cargo); err != nil {
			return fmt.Errorf("a conta %q foi criada, mas o cargo não: %w", *login, err)
		}
		logger.Info("cargo da conta definido", "conta", *login, "cargo", *cargo)
	}
	return nil
}

// leSenhaDeJogoDaEntrada lê a senha da entrada padrão e a mede pela régua do JOGO.
//
// SEPARADA da do painel porque as réguas são opostas e confundi-las é fácil: a do
// painel tem MÍNIMO de 12 caracteres, e a do jogo tem MÁXIMO de 12. Um leitor único
// aceitaria no painel uma senha que o jogo recusa, e a pessoa descobriria no login.
func leSenhaDeJogoDaEntrada(entrada io.Reader) (string, error) {
	senha, err := leTextoDaEntrada(entrada)
	if err != nil {
		return "", err
	}
	if err := accounts.ValidarSenha(senha); err != nil {
		return "", fmt.Errorf("senha de jogo: %w (de %d a %d caracteres, sem espaço, ASCII visível); "+
			"mande pela entrada padrão, por exemplo: printf '%%s' 'sua-senha' | adminserver %s -login NOME",
			err, accounts.MinSenhaBytes, accounts.MaxSenhaBytes, criarContaCmd)
	}
	return senha, nil
}
