package migrations_test

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jeanluca/w2pp-openwyd/internal/migrations"
)

// A 0166 passa o estoque da Loja de Honra do código para as vagas do God of War.
// O que este teste segura é a vitrine de 25/09 — a mesma que estava no código —,
// com as pilhas de três na coluna quantity e a Fada Azul com o prazo de 24 horas.
func TestLojaDeHonraNoBanco(t *testing.T) {
	b, err := migrations.FS.ReadFile("0166_loja_de_honra_no_banco.up.sql")
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
			t.Errorf("a 0166 não tem %q", quer)
		}
	}
	if strings.Contains(sql, "DELETE FROM npc_definition") {
		t.Error("a 0166 apaga a definição do NPC")
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

// A 0167 é a vitrine pedida em 26/09/2026: os sete itens de antes com 30% a
// menos, e três novos — a Chave da Caçada Orc (465), a Repletion D (4019) em
// pilha de cinco e o Ovo de Dente de Sabre (2305), o mais caro da loja.
func TestLojaDeHonraVitrineNova(t *testing.T) {
	b, err := migrations.FS.ReadFile("0167_loja_de_honra_vitrine_nova.up.sql")
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
			t.Errorf("a 0167 não tem %q", quer)
		}
	}
	if strings.Contains(sql, "DELETE FROM npc_definition") {
		t.Error("a 0167 apaga a definição do NPC")
	}
}

// A 0171 põe os três Círculos Divinos Puros (448, 449, 450) a 100 pontos, logo
// depois da Poeira de Lactolerium, e empurra os outros dez itens três vagas sem
// mexer em preço nem quantidade. A volta devolve a vitrine da 0167.
func TestLojaDeHonraCirculos(t *testing.T) {
	b, err := migrations.FS.ReadFile("0171_loja_de_honra_circulos.up.sql")
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
		"(1::smallint, 448, 1::smallint, 0::smallint, 0::smallint, 100)",
		"(2::smallint, 449, 1::smallint, 0::smallint, 0::smallint, 100)",
		"(3::smallint, 450, 1::smallint, 0::smallint, 0::smallint, 100)",
		"(4::smallint, 3438, 1::smallint, 0::smallint, 0::smallint, 252)",
		"(5::smallint, 412, 3::smallint, 0::smallint, 0::smallint, 336)",
		"(6::smallint, 465, 1::smallint, 0::smallint, 0::smallint, 480)",
		"(7::smallint, 4019, 5::smallint, 0::smallint, 0::smallint, 500)",
		"(8::smallint, 3901, 1::smallint, 106::smallint, 1::smallint, 672)", // Fada Azul 24 h
		"(9::smallint, 4140, 1::smallint, 0::smallint, 0::smallint, 1008)",
		"(10::smallint, 3173, 3::smallint, 0::smallint, 0::smallint, 1008)",
		"(11::smallint, 3467, 1::smallint, 0::smallint, 0::smallint, 1680)",
		"(12::smallint, 2305, 1::smallint, 0::smallint, 0::smallint, 3600)",
	} {
		if !strings.Contains(sql, quer) {
			t.Errorf("a 0171 não tem %q", quer)
		}
	}
	if n := strings.Count(sql, "::smallint, 0::smallint, 0::smallint,") + strings.Count(sql, "::smallint, 106::smallint,"); n != 13 {
		t.Errorf("a 0171 tem %d vagas, want 13", n)
	}
	if strings.Contains(sql, "DELETE FROM npc_definition") {
		t.Error("a 0171 apaga a definição do NPC")
	}

	volta, err := migrations.FS.ReadFile("0171_loja_de_honra_circulos.down.sql")
	if err != nil {
		t.Fatal(err)
	}
	ida, err := migrations.FS.ReadFile("0167_loja_de_honra_vitrine_nova.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	if semComentariosSQL(string(volta)) != semComentariosSQL(string(ida)) {
		t.Error("a volta da 0171 não é a vitrine da 0167")
	}
}

// A 0173 baixa em 25% todo preço da vitrine da 0171, arredondando para baixo, e
// mantém vagas, itens e quantidades. A volta devolve a vitrine da 0171.
func TestLojaDeHonraMenos25(t *testing.T) {
	b, err := migrations.FS.ReadFile("0173_loja_de_honra_menos_25.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := semComentariosSQL(string(b))
	for _, quer := range []string{
		"DELETE FROM npc_shop_item WHERE npc_id IN (SELECT id FROM npc_definition WHERE lower(btrim(template_name)) = 'god_of_war')",
		"WHERE lower(btrim(d.template_name)) = 'god_of_war'",
		"UPDATE npc_config_meta SET version = version + 1",
		// vaga, item, quantidade, efeito, valor, pontos: os da 0171 x 0,75
		"(0::smallint, 413, 1::smallint, 0::smallint, 0::smallint, 52)",
		"(1::smallint, 448, 1::smallint, 0::smallint, 0::smallint, 75)",
		"(2::smallint, 449, 1::smallint, 0::smallint, 0::smallint, 75)",
		"(3::smallint, 450, 1::smallint, 0::smallint, 0::smallint, 75)",
		"(4::smallint, 3438, 1::smallint, 0::smallint, 0::smallint, 189)",
		"(5::smallint, 412, 3::smallint, 0::smallint, 0::smallint, 252)",
		"(6::smallint, 465, 1::smallint, 0::smallint, 0::smallint, 360)",
		"(7::smallint, 4019, 5::smallint, 0::smallint, 0::smallint, 375)",
		"(8::smallint, 3901, 1::smallint, 106::smallint, 1::smallint, 504)", // Fada Azul 24 h
		"(9::smallint, 4140, 1::smallint, 0::smallint, 0::smallint, 756)",
		"(10::smallint, 3173, 3::smallint, 0::smallint, 0::smallint, 756)",
		"(11::smallint, 3467, 1::smallint, 0::smallint, 0::smallint, 1260)",
		"(12::smallint, 2305, 1::smallint, 0::smallint, 0::smallint, 2700)",
	} {
		if !strings.Contains(sql, quer) {
			t.Errorf("a 0173 não tem %q", quer)
		}
	}
	if n := strings.Count(sql, "::smallint, 0::smallint, 0::smallint,") + strings.Count(sql, "::smallint, 106::smallint,"); n != 13 {
		t.Errorf("a 0173 tem %d vagas, want 13", n)
	}

	volta, err := migrations.FS.ReadFile("0173_loja_de_honra_menos_25.down.sql")
	if err != nil {
		t.Fatal(err)
	}
	ida, err := migrations.FS.ReadFile("0171_loja_de_honra_circulos.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	if semComentariosSQL(string(volta)) != semComentariosSQL(string(ida)) {
		t.Error("a volta da 0173 não é a vitrine da 0171")
	}
}
