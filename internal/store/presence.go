package store

import (
	"context"
	"fmt"
)

// Character presence (character.online_since, 0024_admin_editor).
//
// This exists for one reader: the staff panel, which must know whether the
// database is the authority for a character's items. While someone is in-play
// the tmServer owns their inventory and rewrites it wholesale on every save
// (SaveCharacter), so an edit written underneath them is lost — the panel needs
// to refuse rather than pretend it worked.
//
// Nothing in the game reads these marks. They are deliberately a column on
// character rather than a session table: presence is a property of the
// character, there is at most one, and a column cannot leak rows.

// SetCharacterPresence marks a character in-play or out. Returns false when the
// name does not resolve, which the caller may ignore — presence is bookkeeping,
// and failing to record it must never interfere with a login or a logout.
func (s *Store) SetCharacterPresence(ctx context.Context, name string, online bool) (bool, error) {
	// now() rather than a value from the caller: the timestamp is only ever
	// compared against other database time, and the tmServer's clock may drift.
	tag, err := s.pool.Exec(ctx, `
		UPDATE character
		   SET online_since = CASE WHEN $2 THEN now() ELSE NULL END
		 WHERE name = $1`, name, online)
	if err != nil {
		return false, fmt.Errorf("store: presence %q: %w", name, err)
	}
	return tag.RowsAffected() > 0, nil
}

// ClearAllPresence drops every presence mark and reports how many it dropped.
//
// The tmServer calls this once at boot. Without it an unclean shutdown strands
// characters marked online forever, and the panel would refuse to edit them for
// good — the failure mode of a presence flag that only one process can clear.
func (s *Store) ClearAllPresence(ctx context.Context) (int64, error) {
	tag, err := s.pool.Exec(ctx,
		`UPDATE character SET online_since = NULL WHERE online_since IS NOT NULL`)
	if err != nil {
		return 0, fmt.Errorf("store: clear presence: %w", err)
	}
	return tag.RowsAffected(), nil
}
