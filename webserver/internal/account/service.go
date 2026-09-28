// Package account holds the web platform's account logic: sign-up and the
// credential check the BFF uses to mint a session cookie. It writes only the
// `account` row (cold storage) — never live character state — so it is safe to
// run outside tmServer's single-owner game loop (web-platform-plan.md).
package account

import (
	"context"
	"errors"
	"fmt"
	"net/mail"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/jeanluca/w2pp-openwyd/internal/domain"
	"github.com/jeanluca/w2pp-openwyd/internal/secret"
	"github.com/jeanluca/w2pp-openwyd/internal/store"
)

// Store is the persistence surface the service needs (satisfied by *store.Store).
// Kept as an interface so the service is unit-testable without a live database.
type Store interface {
	AccountByName(ctx context.Context, name string) (store.AccountAuth, error)
	SaveAccount(ctx context.Context, acc domain.Account) (int64, error)
	// VincularDiscord guarda o Discord da conta. As recusas previstas vêm como
	// erros próprios do store (ver store.ErrDiscordEmOutraConta e irmãos).
	VincularDiscord(ctx context.Context, accountID int64, discordID string) error
}

// Service creates and authenticates web accounts.
type Service struct {
	store Store
	// cadastroFechado tranca a criação de conta. Ver SemCadastro.
	cadastroFechado bool
}

// New builds the account service over the given store.
func New(s Store) *Service { return &Service{store: s} }

// SemCadastro desliga a criação de conta neste servidor.
//
// É para o AMBIENTE DE TESTE: o cliente já está na mão de gente, e um cadastro
// aberto faz do teste um segundo servidor público sem ninguém ter decidido isso.
//
// Só o CADASTRO fecha. O login continua igual, porque quem já tem conta de staff
// precisa entrar — e é justamente para essas pessoas que o ambiente existe.
func (s *Service) SemCadastro() *Service {
	s.cadastroFechado = true
	return s
}

// CreateResult is the business outcome of a sign-up attempt.
type CreateResult int

const (
	// CreateOK means the account was created; the returned id is valid.
	CreateOK CreateResult = iota
	// CreateNameTaken means the canonical name already exists.
	CreateNameTaken
	// CreateInvalid means name/password/email failed validation.
	CreateInvalid
	// CreateFechado é o cadastro DESLIGADO neste servidor.
	//
	// Valor próprio e não CreateInvalid: nada no pedido estava errado, e dizer que
	// estava manda a pessoa tentar de novo com outra senha, e de novo, e depois
	// procurar o suporte.
	CreateFechado
)

// Account-name rules: 4–12 ASCII alphanumerics (the legacy login field is short
// and case-insensitive). Passwords are at least 4 chars. These are intentionally
// conservative; tighten as product needs dictate.
const (
	minNameLen = 4
	maxNameLen = 12
	minPassLen = 4
)

// Create registers a new account. The name is canonicalized to lowercase; the
// password is stored only as an argon2id hash (never plaintext). Email is
// optional but, when present, must parse as an address.
//
// Business outcomes (invalid input, name taken) are returned as a CreateResult,
// not an error — error is reserved for infrastructure failures. A concurrent
// sign-up that wins the unique-name race is reported as CreateNameTaken via the
// Postgres unique violation, so the DB constraint is the final arbiter.
func (s *Service) Create(ctx context.Context, name, password, email string) (CreateResult, int64, error) {
	// A PORTA FECHADA É CONFERIDA ANTES DA VALIDAÇÃO, e a ordem é o que torna a
	// resposta honesta: validando primeiro, um pedido malformado receberia
	// "inválido" num servidor onde NADA seria aceito, e a pessoa passaria a tarde
	// corrigindo um formulário que não tinha problema nenhum.
	if s.cadastroFechado {
		return CreateFechado, 0, nil
	}
	canonical := strings.ToLower(strings.TrimSpace(name))
	if !validName(canonical) || len(password) < minPassLen || !validEmail(email) {
		return CreateInvalid, 0, nil
	}

	switch _, err := s.store.AccountByName(ctx, canonical); {
	case err == nil:
		return CreateNameTaken, 0, nil
	case !errors.Is(err, store.ErrNotFound):
		return CreateInvalid, 0, fmt.Errorf("account: lookup %q: %w", canonical, err)
	}

	passHash, err := secret.HashSecret(password)
	if err != nil {
		return CreateInvalid, 0, fmt.Errorf("account: hash password: %w", err)
	}

	id, err := s.store.SaveAccount(ctx, domain.Account{
		Name:     canonical,
		PassHash: passHash,
		Email:    email,
	})
	if err != nil {
		if isUniqueViolation(err) {
			return CreateNameTaken, 0, nil
		}
		return CreateInvalid, 0, fmt.Errorf("account: save %q: %w", canonical, err)
	}
	return CreateOK, id, nil
}

// Verify reports whether name+password match a stored account. A missing account
// or wrong password both return ok=false (no account enumeration via timing is
// attempted here beyond the constant-time hash compare). blocked reflects
// account.is_blocked; role is account.role ('player'/'moderator'/'admin') so the
// BFF can gate the moderator UI — authorization itself stays server-side in the
// NpcAdminService, this is only a UI hint.
func (s *Service) Verify(ctx context.Context, name, password string) (ok bool, accountID int64, blocked bool, role, discordID string, err error) {
	canonical := strings.ToLower(strings.TrimSpace(name))
	auth, err := s.store.AccountByName(ctx, canonical)
	if errors.Is(err, store.ErrNotFound) {
		return false, 0, false, "", "", nil
	}
	if err != nil {
		return false, 0, false, "", "", fmt.Errorf("account: lookup %q: %w", canonical, err)
	}
	match, err := secret.VerifySecret(password, auth.PassHash)
	if err != nil {
		return false, 0, false, "", "", fmt.Errorf("account: verify password: %w", err)
	}
	if !match {
		return false, 0, false, "", "", nil
	}
	return true, auth.ID, auth.IsBlocked, auth.Role, auth.DiscordID, nil
}

// validName enforces the 4–12 ASCII-alphanumeric login rule on an already
// lowercased name.
func validName(name string) bool {
	if len(name) < minNameLen || len(name) > maxNameLen {
		return false
	}
	for i := 0; i < len(name); i++ {
		c := name[i]
		if (c < 'a' || c > 'z') && (c < '0' || c > '9') {
			return false
		}
	}
	return true
}

// validEmail accepts an empty address (email is optional) or any RFC 5322
// address that net/mail can parse.
func validEmail(email string) bool {
	if email == "" {
		return true
	}
	_, err := mail.ParseAddress(email)
	return err == nil
}

// isUniqueViolation reports whether err is a Postgres unique-constraint
// violation (SQLSTATE 23505), used to turn a lost name race into CreateNameTaken.
func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

// VincularDiscord repassa o vínculo ao store.
//
// SEM REGRA NOVA AQUI de propósito: a validação do snowflake e as duas recusas de
// unicidade moram no store, junto da transação e do índice que as garante. Uma
// segunda conferência nesta camada seria uma regra em dois lugares, e o dia em que as
// duas discordassem ninguém saberia qual vale.
func (s *Service) VincularDiscord(ctx context.Context, accountID int64, discordID string) error {
	return s.store.VincularDiscord(ctx, accountID, discordID)
}
