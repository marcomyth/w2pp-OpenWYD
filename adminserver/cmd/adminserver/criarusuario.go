package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/jeanluca/w2pp-openwyd/adminserver/internal/audit"
	"github.com/jeanluca/w2pp-openwyd/internal/store"
)

// O SUBCOMANDO QUE TIRA O PAINEL DO OVO E DA GALINHA.
//
// Num ambiente novo — o servidor de teste, o LOTM — ninguém consegue abrir o painel: o
// login antigo exige uma conta de jogo com CARGO, e dar cargo exige o painel. Foi isso que
// travou o LOTM.
//
// A senha vem pela ENTRADA PADRÃO e nunca por argumento. Argumento aparece no histórico do
// shell, na lista de processos (`ps` mostra a linha de comando inteira para qualquer
// usuário da máquina) e no log do terminal de quem estava assistindo. A senha do painel é a
// que abre tudo; ela não pode passar por nenhum desses.

// criarUsuarioCmd é o verbo que main() reconhece antes de qualquer flag do servidor.
const criarUsuarioCmd = "criar-usuario"

// criarUsuario roda o subcomando. Recebe args JÁ sem o verbo.
func criarUsuario(logger *slog.Logger, args []string, entrada io.Reader) error {
	fs := flag.NewFlagSet(criarUsuarioCmd, flag.ContinueOnError)
	dsn := fs.String("dsn", envOr("DATABASE_URL", os.Getenv("W2PP_DB_DSN")), "PostgreSQL DSN (ou DATABASE_URL)")
	login := fs.String("login", "", "login do usuário do painel (minúsculas, sem espaço)")
	papel := fs.String("papel", "admin", "moderator ou admin")
	fs.Usage = func() {
		fmt.Fprintf(fs.Output(), `adminserver %s -login NOME [-papel admin|moderator]

Cria um usuário do PAINEL — quem administra, sem personagem no jogo.

A SENHA VEM PELA ENTRADA PADRÃO, nunca por argumento:

    printf '%%s' 'a-senha-aqui' | adminserver %s -login hanna

Argumento apareceria no histórico do shell e na lista de processos da máquina.

Serve para o primeiro acesso de um ambiente novo, onde ninguém consegue abrir o painel
ainda. Depois disso, use a tela Usuários do painel.

`, criarUsuarioCmd, criarUsuarioCmd)
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

	senha, err := leSenhaDaEntrada(entrada)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool, err := store.Pool(ctx, *dsn)
	if err != nil {
		return fmt.Errorf("abrindo o banco: %w", err)
	}
	defer pool.Close()
	st := store.New(pool)

	// AS MIGRAÇÕES RODAM AQUI TAMBÉM, e não é zelo: num ambiente novo a tabela
	// painel_usuario pode nem existir, e este comando é justamente o primeiro a rodar. Sem
	// isto, o primeiro acesso exigiria subir um serviço antes — e subir o painel é o que
	// ainda não dá.
	if err := store.Migrate(ctx, pool); err != nil {
		return fmt.Errorf("aplicando as migracoes: %w", err)
	}

	total, adminsAtivos, err := st.ContarUsuariosDoPainel(ctx)
	if err != nil {
		return err
	}
	// AVISA, E NÃO RECUSA, quando já existe painel montado.
	//
	// Recusar transformaria este comando numa porta que fecha sozinha: no dia em que o
	// último admin perder a senha, a saída seria mexer no banco à mão. E quem roda isto já
	// tem shell na máquina e o DSN, ou seja, já pode tudo — a recusa não protegeria nada,
	// só atrapalharia no pior momento.
	painelJaMontado := total > 0
	if painelJaMontado {
		logger.Warn("já existem usuários do painel; este comando é para ambiente novo",
			"total", total, "admins_ativos", adminsAtivos)
	}

	// Quem criou fica NULO: este caminho não tem criador, e é o único que pode não ter.
	u, err := st.CriarUsuarioDoPainel(ctx, *login, senha, *papel, nil)
	switch {
	case errors.Is(err, store.ErrLoginEmUso):
		return fmt.Errorf("esse login já existe; use a tela do painel para trocar a senha dele: %w", err)
	case err != nil:
		return err
	}
	// O LOG NÃO DIZ A SENHA nem o tamanho dela. Diz o que a pessoa precisa para conferir
	// que deu certo, e nada que ajude quem estiver lendo o terminal por cima do ombro.
	logger.Info("usuário do painel criado", "id", u.ID, "login", u.Login, "papel", u.Papel)

	// COM O PAINEL JÁ MONTADO, ISTO VAI PARA A AUDITORIA, e não só para o aviso.
	//
	// Exigência da planejadora, e ela está certa: enquanto o ambiente está vazio este
	// comando é o único caminho e não há painel para auditar. Depois disso, ele passa a ser
	// uma porta lateral — alguém com shell na máquina dando acesso ao painel sem clicar em
	// nada. Sem registro, essa porta é SILENCIOSA, e a auditoria estaria mentindo por
	// omissão para quem fosse conferir depois quem deu acesso a quem.
	//
	// O ATOR É O USUÁRIO CRIADO, e não quem rodou o comando: a linha de comando não tem ator
	// de painel para apontar, e o CHECK do banco exige exatamente um. Apontar para o
	// recém-criado é o registro honesto do que aconteceu — "este usuário nasceu por fora da
	// tela" —, e o nome da ação diz o resto.
	if painelJaMontado {
		if err := audit.New(pool).Write(ctx, audit.Record{
			AtorPainelID: u.ID, ActorRole: u.Papel,
			Action: audit.ActionPainelUsuarioCriadoPorLinhaDeComando,
			New: map[string]any{
				"painel_usuario": u.ID, "login": u.Login, "papel": u.Papel,
				"usuarios_antes": total,
			},
		}); err != nil {
			// O USUÁRIO JÁ EXISTE e a auditoria não. Erro visível, e não aviso de rodapé:
			// num comando que dá acesso ao painel, a criação sem registro é justamente a que
			// ninguém consegue explicar depois. Quem rodar vê o erro e pode registrar à mão;
			// o silêncio não daria essa chance.
			return fmt.Errorf("o usuário %q foi criado, mas a auditoria FALHOU — "+
				"registre isto à mão e avise quem cuida do servidor: %w", u.Login, err)
		}
	}
	fmt.Printf("Usuário %q criado como %s. Entre no painel e troque a senha.\n", u.Login, u.Papel)
	return nil
}

// senhaMinimaDoPainel é o mesmo piso da tela. Repetido aqui como constante própria porque
// o pacote main não importa o pacote panel — e um número diferente nos dois lugares faria
// o comando aceitar uma senha que a tela recusaria.
const senhaMinimaDoPainel = 12

// leSenhaDaEntrada lê a senha da entrada padrão.
//
// LÊ TUDO e tira só o \n e \r do FIM, porque senha pode ter espaço no meio de propósito —
// frase-senha é mais forte do que palavra, e um TrimSpace inteiro apagaria o começo e o fim
// de uma. O \r entra na conta porque quem colar de um arquivo do Windows manda \r\n.
func leSenhaDaEntrada(entrada io.Reader) (string, error) {
	b, err := io.ReadAll(bufio.NewReader(entrada))
	if err != nil {
		return "", fmt.Errorf("lendo a senha da entrada padrão: %w", err)
	}
	senha := strings.TrimRight(string(b), "\r\n")
	if len(senha) < senhaMinimaDoPainel {
		return "", fmt.Errorf("a senha precisa de pelo menos %d caracteres; "+
			"mande pela entrada padrão, por exemplo: printf '%%s' 'sua-senha' | adminserver %s -login NOME",
			senhaMinimaDoPainel, criarUsuarioCmd)
	}
	return senha, nil
}
