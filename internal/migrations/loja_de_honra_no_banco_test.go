package migrations_test

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jeanluca/w2pp-openwyd/internal/migrations"
)

// A 0164 passa o estoque da Loja de Honra do código para as vagas do God of War.
// O que este teste segura é a vitrine de 25/09 — a mesma que estava no código —,
// com as pilhas de três na coluna quantity e a Fada Azul com o prazo de 24 horas.
func TestLojaDeHonraNoBanco(t *testing.T) {
	b, err := migrations.FS.ReadFile("0164_loja_de_honra_no_banco.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := semComentariosSQL(string(b))
	const doGodOfWar = "(SELECT id FROM npc_definition WHERE lower(btrim(template_name)) = 'god_of_war')"
	for _, quer := range []string{
		"DELETE FROM npc_shop_item WHERE npc_id IN " + doGodOfWar,
		"DELETE FROM npc_shop_slot_cleared WHERE npc_id IN " + doGodOfWar,
		"INSERT INTO npc_shop_item (npc_id, slot, item_index, quantity, eff1, effv1, price_points)",
		"WHERE lower(btrim(d.template_name)) = 'god_of_war'",
		"UPDATE npc_config_meta SET version = version + 1",
		// vaga, item, quantidade, efeito, valor, pontos
		"(0::smallint, 413, 1::smallint, 0::smallint, 0::smallint, 100)",
		"(1::smallint, 3438, 1::smallint, 0::smallint, 0::smallint, 360)",
		"(2::smallint, 412, 3::smallint, 0::smallint, 0::smallint, 480)",
		"(3::smallint, 3901, 1::smallint, 106::smallint, 1::smallint, 960)", // Fada Azul 24 h
		"(4::smallint, 4140, 1::smallint, 0::smallint, 0::smallint, 1440)",
		"(5::smallint, 3173, 3::smallint, 0::smallint, 0::smallint, 1440)",
		"(6::smallint, 3467, 1::smallint, 0::smallint, 0::smallint, 2400)",
	} {
		if !strings.Contains(sql, quer) {
			t.Errorf("a 0164 não tem %q", quer)
		}
	}
	if strings.Contains(sql, "DELETE FROM npc_definition") {
		t.Error("a 0164 apaga a definição do NPC")
	}
}

// O template do God of War tem de ficar SEM estoque. A seed do dbServer recoloca
// o item do template em toda vaga livre a cada boot, sem price_points: um item
// que voltasse por ali ficaria em ouro, fora da Loja de Honra, e o log de cada
// recarga reclamaria dele.
func TestGodOfWarTemplateSemEstoque(t *testing.T) {
	arquivo := filepath.Join("..", "..", "Release", "TMsrv", "run", "npc", "God_of_War")
	b, err := os.ReadFile(arquivo)
	if err != nil {
		t.Fatal(err)
	}
	const structMob, carry, slots, item = 816, 268, 64, 8
	if len(b) != structMob {
		t.Fatalf("template tem %d bytes, want %d", len(b), structMob)
	}
	for i := 0; i < slots; i++ {
		off := carry + i*item
		if idx := int16(binary.LittleEndian.Uint16(b[off:])); idx != 0 {
			t.Errorf("Carry[%d] do template tem o item %d; o estoque vive só no banco", i, idx)
		}
	}
}

// A 0165 é a vitrine pedida em 26/09/2026: os sete itens de antes com 30% a
// menos, e três novos — a Chave da Caçada Orc (465), a Repletion D (4019) em
// pilha de cinco e o Ovo de Dente de Sabre (2305), o mais caro da loja.
func TestLojaDeHonraVitrineNova(t *testing.T) {
	b, err := migrations.FS.ReadFile("0165_loja_de_honra_vitrine_nova.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := semComentariosSQL(string(b))
	for _, quer := range []string{
		"DELETE FROM npc_shop_item WHERE npc_id IN (SELECT id FROM npc_definition WHERE lower(btrim(template_name)) = 'god_of_war')",
		"WHERE lower(btrim(d.template_name)) = 'god_of_war'",
		"UPDATE npc_config_meta SET version = version + 1",
		// vaga, item, quantidade, efeito, valor, pontos
		"(0::smallint, 413, 1::smallint, 0::smallint, 0::smallint, 70)",
		"(1::smallint, 3438, 1::smallint, 0::smallint, 0::smallint, 252)",
		"(2::smallint, 412, 3::smallint, 0::smallint, 0::smallint, 336)",
		"(3::smallint, 465, 1::smallint, 0::smallint, 0::smallint, 480)",
		"(4::smallint, 4019, 5::smallint, 0::smallint, 0::smallint, 500)",
		"(5::smallint, 3901, 1::smallint, 106::smallint, 1::smallint, 672)", // Fada Azul 24 h
		"(6::smallint, 4140, 1::smallint, 0::smallint, 0::smallint, 1008)",
		"(7::smallint, 3173, 3::smallint, 0::smallint, 0::smallint, 1008)",
		"(8::smallint, 3467, 1::smallint, 0::smallint, 0::smallint, 1680)",
		"(9::smallint, 2305, 1::smallint, 0::smallint, 0::smallint, 3600)",
	} {
		if !strings.Contains(sql, quer) {
			t.Errorf("a 0165 não tem %q", quer)
		}
	}
	if strings.Contains(sql, "DELETE FROM npc_definition") {
		t.Error("a 0165 apaga a definição do NPC")
	}
}
