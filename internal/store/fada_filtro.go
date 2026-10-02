package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// FadaFiltroMax é o teto da lista de Itens Protegidos (o CHECK da 0183).
const FadaFiltroMax = 60

// loadFadaFiltro lê o filtro de drop das fadas de um personagem. Sem linha:
// desligado e vazio.
func (s *Store) loadFadaFiltro(ctx context.Context, charID int64) (bool, []int16, error) {
	var ligado bool
	var itens []int16
	err := s.pool.QueryRow(ctx,
		`SELECT ligado, itens FROM fada_filtro WHERE character_id = $1`, charID).Scan(&ligado, &itens)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil, nil
	}
	if err != nil {
		return false, nil, fmt.Errorf("store: load fada_filtro c=%d: %w", charID, err)
	}
	return ligado, itens, nil
}

// SaveFadaFiltro grava o filtro do personagem (conta, slot): a lista inteira
// substitui a anterior. Ligado com lista vazia e lista acima do teto são recusados
// aqui, antes do CHECK, para o erro dizer o que foi.
func (s *Store) SaveFadaFiltro(ctx context.Context, accountID int64, slot int, ligado bool, itens []int16) error {
	if len(itens) > FadaFiltroMax {
		return fmt.Errorf("store: fada_filtro com %d itens, teto %d", len(itens), FadaFiltroMax)
	}
	if ligado && len(itens) == 0 {
		return errors.New("store: fada_filtro ligado com a lista vazia")
	}
	if itens == nil {
		itens = []int16{}
	}
	tag, err := s.pool.Exec(ctx, `
		INSERT INTO fada_filtro (character_id, ligado, itens, updated_at)
		SELECT id, $3, $4, now() FROM character WHERE account_id = $1 AND slot = $2
		ON CONFLICT (character_id) DO UPDATE
		   SET ligado = EXCLUDED.ligado, itens = EXCLUDED.itens, updated_at = now()`,
		accountID, slot, ligado, itens)
	if err != nil {
		return fmt.Errorf("store: save fada_filtro a=%d slot=%d: %w", accountID, slot, err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}
