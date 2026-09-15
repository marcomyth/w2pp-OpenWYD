package main

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/jeanluca/w2pp-openwyd/internal/npctemplate"
	"github.com/jeanluca/w2pp-openwyd/internal/savefmt"
)

// poeirasDeFada são os três índices que saem de toda venda e todo drop que o
// jogador alcança sozinho (decisão de 15/09/2026): 414 e 4142 Poeira_de_Fada,
// 5600 Poeira_de_Fada_Avançada.
var poeirasDeFada = map[int32]bool{414: true, 4142: true, 5600: true}

func releaseDir(t *testing.T) string {
	t.Helper()
	dir := filepath.Join("..", "..", "..", "Release")
	if _, err := os.Stat(dir); err != nil {
		t.Skip("Release content unavailable")
	}
	return dir
}

// TestCatalogoDeNPCSemPoeiraDeFada: o catálogo que o dbServer monta do Release e
// ressemeia em todo boot (SeedNPCDefinitions, INSERT ... ON CONFLICT DO NOTHING)
// não pode trazer as poeiras. Sem isto, apagar a vaga da Evolução no banco não
// adianta: ela voltaria no boot seguinte.
func TestCatalogoDeNPCSemPoeiraDeFada(t *testing.T) {
	defs, err := buildNPCDefinitions(releaseDir(t), slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range defs {
		for _, s := range d.Shop {
			if poeirasDeFada[s.ItemIndex] {
				t.Errorf("%s vende o item %d na vaga %d", d.Slug, s.ItemIndex, s.Slot)
			}
		}
	}
}

// TestTemplatesDeLojaSemPoeiraDeFada: nenhum template de NPC com byte de loja
// (Merchant no 17 ou no 104) carrega as poeiras, nem os que hoje não nascem, como
// a Nordic_Store___. Os monstros (Merchant 0) ficam de fora: o drop deles é tirado
// pela regra '*' da Mesa de Drops (migração 0066), que ganha do Carry do template.
func TestTemplatesDeLojaSemPoeiraDeFada(t *testing.T) {
	dir := releaseDir(t)
	arquivos, err := os.ReadDir(filepath.Join(dir, "TMsrv", "run", "npc"))
	if err != nil {
		t.Fatal(err)
	}
	lojas := 0
	for _, a := range arquivos {
		if a.IsDir() {
			continue
		}
		b, _, err := npctemplate.Load(dir, a.Name())
		if err != nil {
			continue
		}
		mob, err := savefmt.DecodeMob(b)
		if err != nil {
			continue
		}
		if mob.Merchant == 0 && mob.CurrentScore.Merchant == 0 {
			continue
		}
		lojas++
		for slot, c := range mob.Carry {
			if poeirasDeFada[int32(c.Index)] {
				t.Errorf("template %s carrega o item %d no Carry[%d]", a.Name(), c.Index, slot)
			}
		}
	}
	if lojas == 0 {
		t.Fatal("nenhum template de loja lido: o teste não provou nada")
	}
}
