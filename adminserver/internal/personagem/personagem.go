// Package personagem is the staff panel's read/write access to one character's
// items and attributes — the editor's data layer.
//
// # Why editing is conditional
//
// The tmServer is the single owner of a character that is in play: its save
// deletes every row for that character and reinserts the list it holds in
// memory. An edit written underneath a live character is therefore lost at the
// next save, and a moderator who believes an item was delivered when it was not
// is how a duplicate gets created — by hand, on the second attempt.
//
// So every write here checks presence first (character.online_since, marked by
// the tmServer) and refuses with ErrEmJogo rather than writing hopefully.
//
// For a character in play there are two legitimate paths, and neither is this
// one. To hand over an item, the panel's existing delivery queue
// (adminserver/internal/entrega) enqueues it and the tmServer applies it inside
// its loop. To edit for real, the operator kicks the account through the game
// control API and waits for this mark to drop — which happens only once the
// leaving save has committed, and is exactly why the mark exists on top of the
// control API's ListOnline.
package personagem

import (
	"context"

	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrNaoEncontrado is returned when the character (or slot) does not exist.
var ErrNaoEncontrado = errors.New("personagem: não encontrado")

// ErrEmJogo refuses a write to a character the tmServer currently owns.
var ErrEmJogo = errors.New("personagem: em jogo")

// ErrSlotInvalido rejects a slot outside its container, or one the character
// cannot reach (a Bolsa do Andarilho band with no bag).
var ErrSlotInvalido = errors.New("personagem: slot inválido")

// Container sizes, mirroring the world constants (tmserver/internal/world:
// MaxEquip/MaxCarry/MaxCargo). Duplicated rather than imported: the panel must
// not depend on the game server's packages, and these are wire-frozen anyway.
const (
	MaxEquip = 16
	MaxCarry = 64
	MaxCargo = 128
)

// The Bolsa do Andarilho geometry (tmserver/internal/handler/carry.go). Carry is
// 64 slots but only 30 are reachable by default: each bag item held in slot 60
// or 61 unlocks a 15-slot band. Editing must respect it — an item placed in a
// locked band exists in the database and is unreachable in game, which reads to
// the player exactly like the item was stolen.
const (
	ItemBolsaAndarilho = 3467
	SlotsBase          = 30
	SlotsPorBolsa      = 15
	MaxCarryLiberado   = 60
	SlotMarcadorBolsa1 = 60
	SlotMarcadorBolsa2 = 61
)

// Store reads and writes the character editor's data.
type Store struct{ pool *pgxpool.Pool }

// New builds a Store over the shared pool.
func New(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }

// Item is one stored item, with its slot.
type Item struct {
	Slot  int
	Index int16
	Eff1  uint8
	EffV1 uint8
	Eff2  uint8
	EffV2 uint8
	Eff3  uint8
	EffV3 uint8
}

// Vazio reports whether the slot holds nothing.
func (i Item) Vazio() bool { return i.Index == 0 }

// Ficha is everything the editor shows for one character.
type Ficha struct {
	ID          int64
	AccountID   int64
	AccountName string
	Slot        int
	Nome        string
	Classe      uint8
	ClassMaster uint8
	GuildID     uint16
	Level       int32
	Exp         int64
	Coin        int32
	Str         int16
	Int         int16
	Dex         int16
	Con         int16
	Hp, MaxHp   int32
	Mp, MaxMp   int32
	// OnlineDesde is nil when the character is not in play. When it is set the
	// tmServer owns this character's items and every write here is refused.
	OnlineDesde *time.Time

	Equip []Item // MaxEquip, positional (empty slots included)
	Carry []Item // MaxCarry, positional
	Cargo []Item // MaxCargo, positional; belongs to the ACCOUNT, not this character
}

// EmJogo reports whether the tmServer currently owns this character.
func (f Ficha) EmJogo() bool { return f.OnlineDesde != nil }

// LimiteCarry is how many inventory slots this character can actually reach,
// counting the Bolsa do Andarilho markers. Mirrors carryLimit in the tmServer.
func (f Ficha) LimiteCarry() int {
	limite := SlotsBase
	if len(f.Carry) > SlotMarcadorBolsa1 && f.Carry[SlotMarcadorBolsa1].Index == ItemBolsaAndarilho {
		limite += SlotsPorBolsa
	}
	if len(f.Carry) > SlotMarcadorBolsa2 && f.Carry[SlotMarcadorBolsa2].Index == ItemBolsaAndarilho {
		limite += SlotsPorBolsa
	}
	if limite > MaxCarryLiberado {
		return MaxCarryLiberado
	}
	return limite
}

// fichaCols is the character projection, matched positionally by scanFicha.
const fichaCols = `c.id, c.account_id, a.name, c.slot, c.name, c.class, c.class_master,
	c.guild_id, c.level, c.exp, c.coin, c.str, c.int, c.dex, c.con,
	c.hp, c.max_hp, c.mp, c.max_mp, c.online_since`

// Carregar loads one character by name, with all three item containers.
func (s *Store) Carregar(ctx context.Context, nome string) (Ficha, error) {
	var f Ficha
	err := s.pool.QueryRow(ctx, `SELECT `+fichaCols+`
		  FROM character c JOIN account a ON a.id = c.account_id
		 WHERE c.name = $1`, nome).
		Scan(&f.ID, &f.AccountID, &f.AccountName, &f.Slot, &f.Nome, &f.Classe, &f.ClassMaster,
			&f.GuildID, &f.Level, &f.Exp, &f.Coin, &f.Str, &f.Int, &f.Dex, &f.Con,
			&f.Hp, &f.MaxHp, &f.Mp, &f.MaxMp, &f.OnlineDesde)
	if errors.Is(err, pgx.ErrNoRows) {
		return Ficha{}, ErrNaoEncontrado
	}
	if err != nil {
		return Ficha{}, fmt.Errorf("personagem: carregar %q: %w", nome, err)
	}

	if f.Equip, err = s.itens(ctx, `owner_kind = 'char_equip' AND character_id = $1`, MaxEquip, f.ID); err != nil {
		return Ficha{}, err
	}
	if f.Carry, err = s.itens(ctx, `owner_kind = 'char_carry' AND character_id = $1`, MaxCarry, f.ID); err != nil {
		return Ficha{}, err
	}
	// The warehouse hangs off the account: all four characters share it, and it
	// is where a granted item lands.
	if f.Cargo, err = s.itens(ctx, `owner_kind = 'account_cargo' AND account_id = $1`, MaxCargo, f.AccountID); err != nil {
		return Ficha{}, err
	}
	return f, nil
}

// itens returns a positional slice of size n: empty slots are not stored, so the
// query result is scattered back into its slots rather than appended.
func (s *Store) itens(ctx context.Context, where string, n int, id int64) ([]Item, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT slot, item_index, eff1, effv1, eff2, effv2, eff3, effv3 FROM item WHERE `+where, id)
	if err != nil {
		return nil, fmt.Errorf("personagem: itens: %w", err)
	}
	defer rows.Close()

	out := make([]Item, n)
	for i := range out {
		out[i].Slot = i
	}
	for rows.Next() {
		var it Item
		if err := rows.Scan(&it.Slot, &it.Index, &it.Eff1, &it.EffV1, &it.Eff2, &it.EffV2, &it.Eff3, &it.EffV3); err != nil {
			return nil, fmt.Errorf("personagem: itens: scan: %w", err)
		}
		// A slot outside the container means corrupt data, not a reason to fail
		// the page: skipping it shows the moderator everything that IS valid.
		if it.Slot < 0 || it.Slot >= n {
			continue
		}
		out[it.Slot] = it
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("personagem: itens: rows: %w", err)
	}
	return out, nil
}

// Destino names an item container for a write.
type Destino string

// The three containers an item can live in, spelled as the item.owner_kind enum.
const (
	DestinoEquip Destino = "char_equip"
	DestinoCarry Destino = "char_carry"
	DestinoCargo Destino = "account_cargo"
)

// tamanho returns the container's slot count, and whether the destination is one
// this package knows.
func (d Destino) tamanho() (int, bool) {
	switch d {
	case DestinoEquip:
		return MaxEquip, true
	case DestinoCarry:
		return MaxCarry, true
	case DestinoCargo:
		return MaxCargo, true
	default:
		return 0, false
	}
}

// GravarSlot writes one item into one slot, replacing whatever was there.
//
// It re-reads presence inside the transaction rather than trusting the Ficha the
// screen was rendered from: a moderator can sit on an open page while the player
// logs in, and the check has to be against the state at write time.
func (s *Store) GravarSlot(ctx context.Context, characterID int64, dest Destino, slot int, it Item) error {
	return s.escrever(ctx, characterID, dest, slot, func(ctx context.Context, tx pgx.Tx, accountID int64) error {
		if err := apagarSlot(ctx, tx, characterID, accountID, dest, slot); err != nil {
			return err
		}
		if it.Vazio() {
			return nil // writing an empty item is a removal
		}
		charRef, accRef := donos(characterID, accountID, dest)
		_, err := tx.Exec(ctx, `
			INSERT INTO item (owner_kind, account_id, character_id, slot, item_index,
			                  eff1, effv1, eff2, effv2, eff3, effv3)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`,
			string(dest), accRef, charRef, slot, it.Index,
			it.Eff1, it.EffV1, it.Eff2, it.EffV2, it.Eff3, it.EffV3)
		if err != nil {
			return fmt.Errorf("personagem: gravar slot: %w", err)
		}
		return nil
	})
}

// LimparSlot empties one slot.
func (s *Store) LimparSlot(ctx context.Context, characterID int64, dest Destino, slot int) error {
	return s.escrever(ctx, characterID, dest, slot, func(ctx context.Context, tx pgx.Tx, accountID int64) error {
		return apagarSlot(ctx, tx, characterID, accountID, dest, slot)
	})
}

// escrever is the guard every item write shares: validate the slot, lock the
// character row, refuse if it is in play, then run the write.
func (s *Store) escrever(ctx context.Context, characterID int64, dest Destino, slot int,
	fn func(context.Context, pgx.Tx, int64) error) error {

	n, ok := dest.tamanho()
	if !ok || slot < 0 || slot >= n {
		return ErrSlotInvalido
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("personagem: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// FOR UPDATE holds the row for the whole transaction, so a login that lands
	// mid-write waits and then sees a consistent character rather than half an
	// edit.
	var accountID int64
	var online *time.Time
	err = tx.QueryRow(ctx,
		`SELECT account_id, online_since FROM character WHERE id = $1 FOR UPDATE`, characterID).
		Scan(&accountID, &online)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNaoEncontrado
	}
	if err != nil {
		return fmt.Errorf("personagem: travar personagem: %w", err)
	}
	if online != nil {
		return ErrEmJogo
	}
	if err := fn(ctx, tx, accountID); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("personagem: commit: %w", err)
	}
	return nil
}

// donos returns the owner columns for a destination: the item table keeps a
// character reference for worn/carried items and an account reference for the
// warehouse, enforced by a CHECK constraint.
func donos(characterID, accountID int64, dest Destino) (charRef, accRef any) {
	if dest == DestinoCargo {
		return nil, accountID
	}
	return characterID, nil
}

func apagarSlot(ctx context.Context, tx pgx.Tx, characterID, accountID int64, dest Destino, slot int) error {
	var err error
	if dest == DestinoCargo {
		_, err = tx.Exec(ctx,
			`DELETE FROM item WHERE owner_kind = $1 AND account_id = $2 AND slot = $3`,
			string(dest), accountID, slot)
	} else {
		_, err = tx.Exec(ctx,
			`DELETE FROM item WHERE owner_kind = $1 AND character_id = $2 AND slot = $3`,
			string(dest), characterID, slot)
	}
	if err != nil {
		return fmt.Errorf("personagem: limpar slot: %w", err)
	}
	return nil
}

// Atributos is the editable attribute block.
type Atributos struct {
	Level int32
	Exp   int64
	Coin  int32
	Str   int16
	Int   int16
	Dex   int16
	Con   int16
}

// GravarAtributos writes the attribute block, refusing a character in play for
// the same reason item writes are refused.
func (s *Store) GravarAtributos(ctx context.Context, characterID int64, a Atributos) (Atributos, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Atributos{}, fmt.Errorf("personagem: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// Read the old values before writing: the audit entry has to record what was
	// actually there, and a subquery in the UPDATE may or may not see the row
	// pre-update depending on the statement snapshot.
	var antes Atributos
	var online *time.Time
	err = tx.QueryRow(ctx, `
		SELECT level, exp, coin, str, int, dex, con, online_since
		  FROM character WHERE id = $1 FOR UPDATE`, characterID).
		Scan(&antes.Level, &antes.Exp, &antes.Coin, &antes.Str, &antes.Int, &antes.Dex, &antes.Con, &online)
	if errors.Is(err, pgx.ErrNoRows) {
		return Atributos{}, ErrNaoEncontrado
	}
	if err != nil {
		return Atributos{}, fmt.Errorf("personagem: ler atributos: %w", err)
	}
	if online != nil {
		return Atributos{}, ErrEmJogo
	}

	if _, err := tx.Exec(ctx, `
		UPDATE character SET level = $2, exp = $3, coin = $4, str = $5, int = $6, dex = $7, con = $8
		 WHERE id = $1`,
		characterID, a.Level, a.Exp, a.Coin, a.Str, a.Int, a.Dex, a.Con); err != nil {
		return Atributos{}, fmt.Errorf("personagem: gravar atributos: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return Atributos{}, fmt.Errorf("personagem: commit: %w", err)
	}
	return antes, nil
}
