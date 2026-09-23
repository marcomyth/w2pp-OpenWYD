package handler

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/content"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

// TestFadaSemDiasNoNomeAindaTemPrazo: desde 23/09/2026 o nome das fadas não diz
// mais os dias, e o catálogo deixa de saber a duração delas pelo nome. A loja
// entrega fada sem efeito nenhum, e uma que chega ao Equip[13] sem prazo o
// tickFairies apaga no primeiro minuto. Cada uma tem de continuar valendo os
// dias que o nome antigo dizia.
func TestFadaSemDiasNoNomeAindaTemPrazo(t *testing.T) {
	d := New(Config{}) // sem catálogo: nenhuma duração vem do nome
	casos := []struct {
		fada int16
		dias int
	}{
		{3900, 3}, {3901, 3}, {3902, 3},
		{3903, 5}, {3904, 5}, {3905, 5},
		{3906, 7}, {3907, 7}, {3908, 7},
		{3911, 7}, {3912, 15}, {3913, 30},
		{3916, 7},
	}
	for _, c := range casos {
		want := time.Duration(c.dias) * 24 * time.Hour
		if got := d.itemLifetime(world.Item{Index: c.fada}); got != want {
			t.Errorf("fada %d sem efeito dura %s, want %s", c.fada, got, want)
		}
	}
	// O tempo gravado no próprio item continua mandando: é por ele que a mesma
	// fada pode ser vendida por 24 horas ou por 15 dias.
	umDia := world.Item{Index: 3901, Effects: durationEffects(24 * time.Hour)}
	if got := d.itemLifetime(umDia); got != 24*time.Hour {
		t.Errorf("Fada Azul com 106 1 dura %s, want 24h", got)
	}
}

// TestTodaFadaDoCatalogoTemPrazo varre o catálogo real: toda fada temporária,
// com ou sem dias no nome, tem de sair com prazo de um jeito ou de outro. É o
// teste que pega a próxima fada renomeada sem entrar em defaultLifetimeDays.
func TestTodaFadaDoCatalogoTemPrazo(t *testing.T) {
	items, err := content.LoadItemList(filepath.Join(releaseDir(t), "Common", "ItemList.csv"))
	if err != nil {
		t.Fatal(err)
	}
	d := New(Config{ItemDurations: items.Durations()})
	for idx, nome := range items.Names() {
		temporaria := isFairy(int16(idx))
		if !temporaria || !strings.HasPrefix(nome, "Fada") {
			continue
		}
		if strings.Contains(nome, "dias") {
			t.Errorf("fada %d ainda tem os dias no nome: %q", idx, nome)
		}
		if d.itemLifetime(world.Item{Index: int16(idx)}) <= 0 {
			t.Errorf("fada %d (%s) sem prazo: seria apagada no primeiro minuto equipada", idx, nome)
		}
	}
}
