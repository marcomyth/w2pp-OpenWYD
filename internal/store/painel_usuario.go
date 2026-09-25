package store

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/jeanluca/w2pp-openwyd/internal/secret"
)

// ErrLoginEmUso é criar um usuário de painel com login que já existe.
var ErrLoginEmUso = errors.New("store: login de painel ja existe")

// ErrLoginInvalido é um login que não serve.
var ErrLoginInvalido = errors.New("store: login de painel invalido")

// ErrPapelInvalido é um papel que o painel não conhece.
var ErrPapelInvalido = errors.New("store: papel de painel invalido")

// UsuarioDoPainel é quem administra, sem precisar de personagem.
//
// SEM O HASH DA SENHA neste tipo, de propósito: ele é lido pela tela que LISTA usuários, e
// um campo de hash que viaja junto da listagem é um campo que um dia aparece num log ou
// num JSON. Quem precisa do hash é só a verificação do login, e ela usa a função própria.
type UsuarioDoPainel struct {
	ID        int64
	Login     string
	Papel     string
	Ativo     bool
	CriadoEm  time.Time
	CriadoPor *int64
}

// papeisDoPainel são os dois que o painel conhece. É a mesma dupla do account.role, e não
// uma lista nova: o painel já decide o que cada um vê por estas duas palavras.
var papeisDoPainel = map[string]bool{"moderator": true, "admin": true}

// NormalizaLoginDoPainel deixa o login na forma gravada, ou diz por que não serve.
//
// MINÚSCULAS E SEM ESPAÇO NAS PONTAS, porque é assim que o formulário de login já trata o
// que a pessoa digita (panel.go faz ToLower+TrimSpace). Se a gravação não fizesse o mesmo,
// criar "Hanna" produziria um usuário que NUNCA consegue entrar — o login procuraria
// "hanna" e não acharia. Esse é o tipo de defeito que só aparece com a pessoa na frente da
// tela, sem erro nenhum no log.
func NormalizaLoginDoPainel(login string) (string, error) {
	v := strings.ToLower(strings.TrimSpace(login))
	switch {
	case v == "":
		return "", fmt.Errorf("%w: vazio", ErrLoginInvalido)
	case len(v) > 64:
		return "", fmt.Errorf("%w: passa de 64 caracteres", ErrLoginInvalido)
	case strings.ContainsAny(v, " \t\n\r"):
		// Espaço no meio não é recusado por gosto: o login vai em cookie, em log e em
		// URL de auditoria, e um nome com espaço é um nome que cada camada escapa de um
		// jeito diferente.
		return "", fmt.Errorf("%w: nao pode ter espaco", ErrLoginInvalido)
	}
	return v, nil
}

// CriarUsuarioDoPainel grava um usuário novo com a senha já cifrada.
//
// O HASH É O MESMO DO JOGO E DO SITE (internal/secret, argon2id). Não há algoritmo novo
// aqui e não pode haver: uma segunda implementação de senha é uma segunda chance de errar
// parâmetro, e a errada não avisa — ela só verifica menos.
//
// criadoPor é nulo APENAS para o primeiro usuário, que nasce pelo subcomando de linha em
// ambiente vazio e portanto não tem criador. Qualquer outro caminho passa quem criou, para
// a pergunta "quem deu acesso a esta pessoa" ter resposta.
func (s *Store) CriarUsuarioDoPainel(ctx context.Context, login, senha, papel string,
	criadoPor *int64,
) (UsuarioDoPainel, error) {
	nome, err := NormalizaLoginDoPainel(login)
	if err != nil {
		return UsuarioDoPainel{}, err
	}
	if !papeisDoPainel[papel] {
		return UsuarioDoPainel{}, fmt.Errorf("%w: %q; use moderator ou admin", ErrPapelInvalido, papel)
	}
	if senha == "" {
		return UsuarioDoPainel{}, errors.New("store: senha de painel vazia")
	}
	hash, err := secret.HashSecret(senha)
	if err != nil {
		return UsuarioDoPainel{}, fmt.Errorf("store: cifrando a senha do painel: %w", err)
	}

	var u UsuarioDoPainel
	err = s.pool.QueryRow(ctx, `
		INSERT INTO painel_usuario (login, senha_hash, papel, criado_por)
		VALUES ($1, $2, $3, $4)
		RETURNING id, login, papel, ativo, criado_em, criado_por`,
		nome, hash, papel, criadoPor).
		Scan(&u.ID, &u.Login, &u.Papel, &u.Ativo, &u.CriadoEm, &u.CriadoPor)
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		// 23505 é violação de unicidade. Traduzido para um erro NOSSO porque quem chama
		// é uma tela, e ela precisa dizer "esse login já existe" em vez de vazar o texto
		// do Postgres para a pessoa.
		return UsuarioDoPainel{}, fmt.Errorf("%w: %q", ErrLoginEmUso, nome)
	}
	if err != nil {
		return UsuarioDoPainel{}, fmt.Errorf("store: criando usuario de painel %q: %w", nome, err)
	}
	return u, nil
}

// AutenticacaoDoPainel é o que o login precisa saber, e nada além.
type AutenticacaoDoPainel struct {
	ID    int64
	Login string
	Papel string
	Ativo bool
	Hash  string
}

// UsuarioDoPainelPorLogin lê o necessário para verificar a senha.
//
// TRAZ O INATIVO TAMBÉM, e isso é escolha. Quem chama tem de gastar o mesmo tempo
// verificando a senha de um usuário desativado, e só então recusar: responder antes de
// verificar transformaria o formulário num revelador de quais logins existem e estão
// desligados.
func (s *Store) UsuarioDoPainelPorLogin(ctx context.Context, login string) (AutenticacaoDoPainel, error) {
	nome, err := NormalizaLoginDoPainel(login)
	if err != nil {
		// Login que nem poderia existir é tratado como inexistente, e não como erro: o
		// formulário não deve distinguir "você digitou algo impossível" de "não existe".
		return AutenticacaoDoPainel{}, ErrNotFound
	}
	var a AutenticacaoDoPainel
	err = s.pool.QueryRow(ctx, `
		SELECT id, login, papel, ativo, senha_hash
		  FROM painel_usuario WHERE login = $1`, nome).
		Scan(&a.ID, &a.Login, &a.Papel, &a.Ativo, &a.Hash)
	if errors.Is(err, pgx.ErrNoRows) {
		return AutenticacaoDoPainel{}, ErrNotFound
	}
	if err != nil {
		return AutenticacaoDoPainel{}, fmt.Errorf("store: lendo usuario de painel %q: %w", nome, err)
	}
	return a, nil
}

// ExisteUsuarioDoPainel diz se aquele login é de painel, sem ler senha nenhuma.
//
// É O QUE DECIDE A "SEM SEGUNDA CHANCE" do login: se o nome digitado é de um usuário de
// painel, o login NÃO tenta a conta de jogo depois. Sem isso, um usuário de painel com a
// senha errada cairia no caminho antigo e poderia entrar como conta de jogo homônima — e
// as duas pessoas teriam o mesmo nome no painel, com permissões diferentes.
func (s *Store) ExisteUsuarioDoPainel(ctx context.Context, login string) (bool, error) {
	nome, err := NormalizaLoginDoPainel(login)
	if err != nil {
		return false, nil
	}
	var existe bool
	if err := s.pool.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM painel_usuario WHERE login = $1)`, nome).Scan(&existe); err != nil {
		return false, fmt.Errorf("store: procurando usuario de painel %q: %w", nome, err)
	}
	return existe, nil
}

// ListarUsuariosDoPainel devolve todos, ativos e desativados, mais antigo primeiro.
//
// OS DESATIVADOS APARECEM porque a tela precisa poder reativar, e porque uma lista que
// esconde quem foi desligado faz alguém tentar criar o mesmo login de novo e bater no
// "já existe" sem entender por quê.
func (s *Store) ListarUsuariosDoPainel(ctx context.Context) ([]UsuarioDoPainel, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, login, papel, ativo, criado_em, criado_por
		  FROM painel_usuario ORDER BY criado_em, id`)
	if err != nil {
		return nil, fmt.Errorf("store: listando usuarios de painel: %w", err)
	}
	defer rows.Close()
	var out []UsuarioDoPainel
	for rows.Next() {
		var u UsuarioDoPainel
		if err := rows.Scan(&u.ID, &u.Login, &u.Papel, &u.Ativo, &u.CriadoEm, &u.CriadoPor); err != nil {
			return nil, fmt.Errorf("store: lendo usuario de painel: %w", err)
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

// ContarUsuariosDoPainel conta quantos existem, para o subcomando saber se o ambiente está
// vazio e para a tela avisar quando o último admin ativo for o que está sendo desligado.
func (s *Store) ContarUsuariosDoPainel(ctx context.Context) (total, adminsAtivos int, err error) {
	if err := s.pool.QueryRow(ctx, `
		SELECT count(*), count(*) FILTER (WHERE ativo AND papel = 'admin')
		  FROM painel_usuario`).Scan(&total, &adminsAtivos); err != nil {
		return 0, 0, fmt.Errorf("store: contando usuarios de painel: %w", err)
	}
	return total, adminsAtivos, nil
}

// ErrUltimoAdmin é desligar ou rebaixar o último admin ativo.
var ErrUltimoAdmin = errors.New("store: seria o ultimo admin ativo do painel")

// DefinirAtivoDoPainel liga ou desliga um usuário.
//
// RECUSA DESLIGAR O ÚLTIMO ADMIN ATIVO, e a trava mora no banco e não só na tela. Sem ela,
// um clique deixa o painel sem NINGUÉM que possa criar usuário — e a saída seria entrar na
// máquina e rodar o subcomando de linha, que é exatamente o aperto que esta separação
// existe para acabar.
//
// A contagem e a escrita vão na MESMA transação, com a linha travada: contar fora dela
// deixaria dois cliques simultâneos desligarem os dois últimos admins, cada um vendo o
// outro ainda ativo.
func (s *Store) DefinirAtivoDoPainel(ctx context.Context, id int64, ativo bool) error {
	return s.inTx(ctx, func(tx pgx.Tx) error {
		var papel string
		var eraAtivo bool
		err := tx.QueryRow(ctx,
			`SELECT papel, ativo FROM painel_usuario WHERE id = $1 FOR UPDATE`, id).
			Scan(&papel, &eraAtivo)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return fmt.Errorf("store: lendo usuario de painel %d: %w", id, err)
		}
		if !ativo && eraAtivo && papel == "admin" {
			var outros int
			if err := tx.QueryRow(ctx, `
				SELECT count(*) FROM painel_usuario
				 WHERE ativo AND papel = 'admin' AND id <> $1`, id).Scan(&outros); err != nil {
				return fmt.Errorf("store: contando os outros admins: %w", err)
			}
			if outros == 0 {
				return ErrUltimoAdmin
			}
		}
		if _, err := tx.Exec(ctx,
			`UPDATE painel_usuario SET ativo = $2 WHERE id = $1`, id, ativo); err != nil {
			return fmt.Errorf("store: mudando o ativo do usuario %d: %w", id, err)
		}
		return nil
	})
}

// TrocarSenhaDoPainel grava uma senha nova.
func (s *Store) TrocarSenhaDoPainel(ctx context.Context, id int64, senha string) error {
	if senha == "" {
		return errors.New("store: senha de painel vazia")
	}
	hash, err := secret.HashSecret(senha)
	if err != nil {
		return fmt.Errorf("store: cifrando a senha do painel: %w", err)
	}
	tag, err := s.pool.Exec(ctx,
		`UPDATE painel_usuario SET senha_hash = $2 WHERE id = $1`, id, hash)
	if err != nil {
		return fmt.Errorf("store: trocando a senha do usuario %d: %w", id, err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}
