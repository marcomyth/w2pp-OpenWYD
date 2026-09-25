// Package entrega puts items into the delivery_queue mailbox, so a moderator
// can hand something to a player from the panel.
//
// Nothing new had to be built in the game for this. delivery_queue already
// exists for the donate shop, and the tmServer drains it at login into the
// account cargo — so a grant made here arrives the next time the player logs
// in, through a path that has been in production and needs no restart.
//
// That timing is the one thing a moderator has to understand: the item does not
// appear in the hands of somebody already playing. The UI says so.
//
// The queries live here rather than in internal/store for the same reason the
// account ones do: every service embeds internal/, so adding there would
// redeploy the game to ship a panel change.
package entrega

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/jeanluca/w2pp-openwyd/internal/store"

	"github.com/jeanluca/w2pp-openwyd/internal/pilha"
)

// Refusals the caller can tell apart.
var (
	ErrNotFound   = errors.New("entrega: delivery not found")
	ErrJaEntregue = errors.New("entrega: already delivered")
	ErrDias       = errors.New("entrega: expiry out of range")
	// ErrQuantidade: menos de 1, ou mais do que cabe num baú vazio.
	ErrQuantidade = errors.New("entrega: quantity out of range")
	// ErrEfeitos: uma pilha precisa de um espaço de efeito livre para EF_AMOUNT,
	// e a quantidade não pode vir também escrita à mão num efeito.
	ErrEfeitos = errors.New("entrega: no effect slot for the stack amount")
)

// MaxDias bounds a timed grant. Longer than this is a typo, not an intention,
// and the stored value is an absolute timestamp nobody would notice was wrong.
const MaxDias = 3650

// Item is what to hand over: a catalog index and up to three effect pairs, the
// same shape the donate shop enqueues.
type Item struct {
	Index int32
	Eff   [3][2]uint8 // {effect, value} pairs; a zero effect is an empty slot
	Dias  int         // 0 = permanent
}

// payload is the delivery_queue.payload shape for kind='item'. It mirrors the
// struct internal/store writes for the donate shop; the field names are the
// contract the tmServer drain reads, so they are spelled out here rather than
// shared, and a test pins them against a round trip.
type payload struct {
	ItemIndex int32 `json:"item_index"`
	Eff1      uint8 `json:"eff1"`
	EffV1     uint8 `json:"effv1"`
	Eff2      uint8 `json:"eff2"`
	EffV2     uint8 `json:"effv2"`
	Eff3      uint8 `json:"eff3"`
	EffV3     uint8 `json:"effv3"`
	ExpiresAt int64 `json:"expires_at"`
}

// Pendente is one grant still waiting for the player to log in.
type Pendente struct {
	ID        int64
	ItemIndex int32
	Eff       [3][2]uint8
	ExpiresAt int64 // absolute Unix seconds; 0 = permanent
	Origem    string
	CriadoEm  time.Time
}

// Expira reports whether the grant is timed.
func (p Pendente) Expira() bool { return p.ExpiresAt > 0 }

// Quando returns the expiry as a time. Only meaningful when Expira.
func (p Pendente) Quando() time.Time { return time.Unix(p.ExpiresAt, 0) }

// Store performs the writes.
type Store struct{ pool *pgxpool.Pool }

// New wraps a pool.
func New(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }

// Lote reparte uma quantidade do item pelos espaços do baú: pilhas de até
// pilha.MaxPorPilha com EF_AMOUNT para o que empilha, um item por espaço para o
// que não empilha (internal/pilha decide). Cada elemento vira uma linha da fila e
// ocupa um espaço, então um baú cheio segura as linhas que sobraram para o
// próximo login, como qualquer entrega.
//
// Uma pilha de 1 sai sem EF_AMOUNT, igual a antes da quantidade existir. A
// quantidade de uma pilha maior vai no primeiro efeito vazio; sem efeito vazio,
// ou com EF_AMOUNT já escrito à mão, é ErrEfeitos.
func Lote(it Item, quantidade int) ([]Item, error) {
	if quantidade < 1 || quantidade > pilha.MaxPorEnvio(int16(it.Index)) {
		return nil, ErrQuantidade
	}
	partes := pilha.Divide(int16(it.Index), quantidade)
	out := make([]Item, 0, len(partes))
	for _, n := range partes {
		cada := it
		if n > 1 {
			livre := -1
			for i := range cada.Eff {
				if cada.Eff[i][0] == pilha.EfAmount {
					return nil, ErrEfeitos
				}
				if livre < 0 && cada.Eff[i][0] == 0 {
					livre = i
				}
			}
			if livre < 0 {
				return nil, ErrEfeitos
			}
			cada.Eff[livre] = [2]uint8{pilha.EfAmount, uint8(n)}
		}
		out = append(out, cada)
	}
	return out, nil
}

// EnfileirarLote grava as linhas de um Lote numa transação só e devolve os ids,
// na ordem. Ou entram todas, ou nenhuma: meia entrega enfileirada seria uma
// quantidade que ninguém escolheu.
func (s *Store) EnfileirarLote(ctx context.Context, actorID, contaID int64, itens []Item) ([]int64, error) {
	if len(itens) == 0 || len(itens) > pilha.EspacosDoBau {
		return nil, ErrQuantidade
	}
	for _, it := range itens {
		if it.Dias < 0 || it.Dias > MaxDias {
			return nil, ErrDias
		}
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("entrega: begin batch for %d: %w", contaID, err)
	}
	defer func() { _ = tx.Rollback(ctx) }() // no-op after a commit
	ids := make([]int64, 0, len(itens))
	for _, it := range itens {
		id, err := enfileirar(ctx, tx, actorID, contaID, it)
		if err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("entrega: commit batch for %d: %w", contaID, err)
	}
	return ids, nil
}

// Enfileirar queues an item for an account and returns the delivery id.
//
// source records who did it, so a row in the mailbox can be traced back to a
// person without joining the audit log — the donate shop writes its own id
// there for the same reason.
func (s *Store) Enfileirar(ctx context.Context, actorID, contaID int64, it Item) (int64, error) {
	if it.Dias < 0 || it.Dias > MaxDias {
		return 0, ErrDias
	}
	return enfileirar(ctx, s.pool, actorID, contaID, it)
}

// querier is what one insert needs: the pool, or a transaction.
type querier interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

func enfileirar(ctx context.Context, q querier, actorID, contaID int64, it Item) (int64, error) {
	var expira int64
	if it.Dias > 0 {
		expira = time.Now().Add(time.Duration(it.Dias) * 24 * time.Hour).Unix()
	}

	body, err := json.Marshal(payload{
		ItemIndex: it.Index,
		Eff1:      it.Eff[0][0], EffV1: it.Eff[0][1],
		Eff2: it.Eff[1][0], EffV2: it.Eff[1][1],
		Eff3: it.Eff[2][0], EffV3: it.Eff[2][1],
		ExpiresAt: expira,
	})
	if err != nil {
		return 0, fmt.Errorf("entrega: marshal payload: %w", err)
	}

	var id int64
	err = q.QueryRow(ctx, `
		INSERT INTO delivery_queue (account_id, kind, payload, source)
		VALUES ($1, 'item', $2, $3)
		RETURNING id`,
		contaID, body, fmt.Sprintf("painel:%d", actorID)).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("entrega: enqueue for %d: %w", contaID, err)
	}
	return id, nil
}

// Pendentes lists what is still waiting for one account, newest last.
func (s *Store) Pendentes(ctx context.Context, contaID int64) ([]Pendente, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, payload, coalesce(source, ''), created_at
		  FROM delivery_queue
		 WHERE account_id = $1 AND status = 'pending' AND kind = 'item'
		 ORDER BY id`, contaID)
	if err != nil {
		return nil, fmt.Errorf("entrega: list pending for %d: %w", contaID, err)
	}
	defer rows.Close()

	var out []Pendente
	for rows.Next() {
		var p Pendente
		var body []byte
		if err := rows.Scan(&p.ID, &body, &p.Origem, &p.CriadoEm); err != nil {
			return nil, fmt.Errorf("entrega: scan pending: %w", err)
		}
		var pl payload
		if err := json.Unmarshal(body, &pl); err != nil {
			// A row the panel cannot read is still a row the game will deliver,
			// so it is listed with what could be recovered rather than hidden.
			out = append(out, p)
			continue
		}
		p.ItemIndex = pl.ItemIndex
		p.Eff = [3][2]uint8{{pl.Eff1, pl.EffV1}, {pl.Eff2, pl.EffV2}, {pl.Eff3, pl.EffV3}}
		p.ExpiresAt = pl.ExpiresAt
		out = append(out, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("entrega: iterate pending: %w", err)
	}
	return out, nil
}

// Cancelar removes a grant that has not been collected yet.
//
// It deletes rather than marking 'lost': 'lost' means the game tried to deliver
// and could not (a full cargo), and it is what an operator reads when chasing a
// complaint. A moderator undoing their own typo is not that, and blurring the
// two would cost the more useful meaning.
//
// Only a pending row can go. Once the player has it, taking it back is a
// different act with different consequences, and the panel does not pretend it
// is the same button.
func (s *Store) Cancelar(ctx context.Context, contaID, entregaID int64) error {
	tag, err := s.pool.Exec(ctx, `
		DELETE FROM delivery_queue
		 WHERE id = $1 AND account_id = $2 AND status = 'pending' AND kind = 'item'`,
		entregaID, contaID)
	if err != nil {
		return fmt.Errorf("entrega: cancel %d: %w", entregaID, err)
	}
	if tag.RowsAffected() == 0 {
		// Either it never existed for this account, or it has already been
		// collected. Telling those apart needs a second read, and the answer to
		// both is the same: there is nothing here to cancel.
		var status string
		err := s.pool.QueryRow(ctx,
			`SELECT status FROM delivery_queue WHERE id = $1 AND account_id = $2`,
			entregaID, contaID).Scan(&status)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return fmt.Errorf("entrega: read status %d: %w", entregaID, err)
		}
		return ErrJaEntregue
	}
	return nil
}

// BauSemEspaco responde se a conta está com o baú lotado.
//
// DELEGA para store.BauSemEspaco em vez de repetir a consulta: a regra do segurado é lida
// em dois lugares — aqui, na lista de itens a caminho, e no web-api, na compra do mercado
// —, e duas consultas iguais escritas em dois arquivos é como uma delas fica para trás.
func (s *Store) BauSemEspaco(ctx context.Context, accountID int64) (bool, error) {
	return store.BauSemEspaco(ctx, s.pool, accountID)
}
