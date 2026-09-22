package handler

import (
	"testing"

	"github.com/jeanluca/w2pp-openwyd/internal/level"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

// Um chefe de guilda tem vida grande, e a conta do máximo efetivo é em int32:
// `pool × (pct+100)/100` estourava a partir de 21.474.836 e devolvia lixo — 0 com
// 25 milhões, 18 milhões com 1,35 bilhão. Como refreshScore prende o HP ao máximo
// efetivo, e ele roda toda vez que um afeto expira no monstro (processMobAffect),
// o chefe morria sozinho no primeiro veneno que acabasse.
func TestVidaDeChefeNaoEstoura(t *testing.T) {
	casos := []struct {
		nome string
		pool int32
		pct  int32
		want int32
	}{
		{"mob comum", 32_000, 0, 32_000},
		{"Lich Crunt", 4_000_000, 0, 4_000_000},
		{"último valor que já cabia em int32", 21_000_000, 0, 21_000_000},
		{"acima do estouro antigo", 25_000_000, 0, 25_000_000},
		{"chefe de guilda", 100_000_000, 0, 100_000_000},
		{"no teto do legado", level.MaxHPCap, 0, level.MaxHPCap},
		{"acima do teto, preso nele", 1_350_000_000, 0, level.MaxHPCap},
		{"com EF_HPADD de 20%", 100_000_000, 20, 120_000_000},
		{"EF_HPADD que passaria do teto", 900_000_000, 50, level.MaxHPCap},
	}
	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			e := &world.Entity{ID: world.MaxUser + 1, MaxHP: c.pool, HP: c.pool, HpAddPct: c.pct}
			if got := effectiveMaxHP(e); got != c.want {
				t.Errorf("effectiveMaxHP(%d, +%d%%) = %d, want %d", c.pool, c.pct, got, c.want)
			}
		})
	}
}

// O mesmo para a mana, que usa a mesma conta.
func TestManaDeChefeNaoEstoura(t *testing.T) {
	e := &world.Entity{ID: world.MaxUser + 1, MaxMP: 100_000_000, MP: 100_000_000}
	if got := effectiveMaxMP(e); got != 100_000_000 {
		t.Errorf("effectiveMaxMP = %d, want 100000000", got)
	}
}
