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

// foraDeVenda são os itens que a reforma dos acessórios (16/09/2026) tira de toda
// loja: os sete planetas e o Amuleto dos Amantes, que viram drop raro, e o
// Amuleto Arcano, que só sai da evolução do Místico +15 no Odin.
var foraDeVenda = map[int32]bool{
	762: true, 763: true, 764: true, 765: true, 766: true, 767: true, 768: true,
	1738: true, 567: true, 568: true, 569: true, 570: true,
}

// TestTemplatesDeLojaSemPlanetasAmantesNemArcano: nenhum template com byte de
// loja carrega esses itens, nem os que hoje não nascem (Lich_Mercador). O banco
// é a outra metade (migração 0070); sem os templates as vagas voltariam no boot.
func TestTemplatesDeLojaSemPlanetasAmantesNemArcano(t *testing.T) {
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
			if foraDeVenda[int32(c.Index)] {
				t.Errorf("template %s carrega o item %d no Carry[%d]", a.Name(), c.Index, slot)
			}
		}
	}
	if lojas == 0 {
		t.Fatal("nenhum template de loja lido: o teste não provou nada")
	}
}

// TestAkiVendeOsAneis confere o que o dbServer semeia para o Aki de Armia: os
// cinco anéis nas vagas 18-22, que a 0070 esvazia no banco, e nenhum dos itens
// que saíram dali.
func TestAkiVendeOsAneis(t *testing.T) {
	defs, err := buildNPCDefinitions(releaseDir(t), slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	quer := map[int16]int32{18: 501, 19: 503, 20: 502, 21: 506, 22: 505}
	sairam := map[int32]bool{646: true, 647: true, 693: true, 694: true, 695: true}
	akis := 0
	for _, d := range defs {
		if d.TemplateName != "Aki" {
			continue
		}
		akis++
		porVaga := map[int16]int32{}
		for _, s := range d.Shop {
			porVaga[s.Slot] = s.ItemIndex
			if sairam[s.ItemIndex] {
				t.Errorf("%s ainda vende o item %d na vaga %d", d.Slug, s.ItemIndex, s.Slot)
			}
		}
		for vaga, item := range quer {
			if porVaga[vaga] != item {
				t.Errorf("%s vaga %d = item %d, esperado o anel %d", d.Slug, vaga, porVaga[vaga], item)
			}
		}
	}
	if akis == 0 {
		t.Fatal("nenhum Aki no catálogo: o teste não provou nada")
	}
}
