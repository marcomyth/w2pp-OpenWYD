package migrations_test

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jeanluca/w2pp-openwyd/internal/migrations"
)

// A 0168 devolve a Safira (697) e o Pacote de Safiras (4131) ao Bardes, e só a
// ele: a 0086 os tirou de todas as lojas. Uma unidade de cada — na loja de NPC o
// preço é por compra, e dez Safiras numa vaga sairiam pelo preço de uma.
func TestBardesVendeSafira(t *testing.T) {
	b, err := migrations.FS.ReadFile("0168_bardes_vende_safira.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := semComentariosSQL(string(b))
	for _, quer := range []string{
		"SELECT id FROM npc_definition WHERE lower(btrim(template_name)) = 'bardes'",
		"(VALUES (1, 697), (2, 4131))",
		"INSERT INTO npc_shop_item (npc_id, slot, item_index, quantity)",
		"1::smallint",
		"DELETE FROM npc_shop_slot_cleared",
		"UPDATE npc_config_meta SET version = version + 1",
	} {
		if !strings.Contains(sql, quer) {
			t.Errorf("a 0168 não tem %q", quer)
		}
	}
	// Só o Bardes: nenhuma instrução pode alcançar outra loja.
	if strings.Contains(sql, "DELETE FROM npc_shop_item") || strings.Contains(sql, "item_price") {
		t.Error("a 0168 mexe em outra loja ou no preço do item")
	}
}

// O template do Bardes continua SEM a Safira e o Pacote (a 0086 os tirou). Se
// eles voltassem ao template, a seed do boot os recolocaria sozinha — e numa
// vaga que a 0168 não escolheu.
func TestBardesTemplateSemSafira(t *testing.T) {
	arquivo := filepath.Join("..", "..", "Release", "TMsrv", "run", "npc", "Bardes")
	b, err := os.ReadFile(arquivo)
	if err != nil {
		t.Fatal(err)
	}
	const carry, slots, item = 268, 64, 8
	for i := 0; i < slots; i++ {
		switch idx := binary.LittleEndian.Uint16(b[carry+i*item:]); idx {
		case 697, 4131:
			t.Errorf("Carry[%d] do template do Bardes tem o item %d", i, idx)
		}
	}
}
