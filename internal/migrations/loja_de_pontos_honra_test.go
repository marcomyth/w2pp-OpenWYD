package migrations_test

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jeanluca/w2pp-openwyd/internal/migrations"
)

// A 0150 devolve a Loja de Pontos ao mundo com sete itens em pontos. O que este
// teste segura é a tabela combinada em 25/09/2026 — cada preço é "dias de
// barraca aberta" — e as pilhas de três, que vão na coluna quantity (é ali que o
// banco guarda o EF_AMOUNT 61).
func TestLojaDePontosHonraVitrine(t *testing.T) {
	b, err := migrations.FS.ReadFile("0150_loja_de_pontos_honra.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := semComentariosSQL(string(b))

	for _, quer := range []string{
		"UPDATE npc_definition SET enabled = TRUE WHERE template_name = 'Loja_de_Pontos'",
		"DELETE FROM npc_shop_item WHERE npc_id IN (SELECT id FROM npc_definition WHERE template_name = 'Loja_de_Pontos')",
		"INSERT INTO npc_shop_item (npc_id, slot, item_index, quantity, eff1, effv1, price_points)",
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
			t.Errorf("a 0150 não tem %q", quer)
		}
	}

	// Desativa ou reativa, nunca apaga: o slug carrega a posição do bloco no
	// NPCGener (mesma razão da 0083 e da 0096).
	if strings.Contains(sql, "DELETE FROM npc_definition") {
		t.Error("a 0150 apaga a definição do NPC")
	}
}

// O template da Loja de Pontos tem de ficar SEM estoque. A seed do dbServer
// recoloca o item do template em toda vaga livre a cada boot, e ela não grava
// price_points: um item que voltasse por ali seria vendido em OURO pelo preço do
// catálogo. Foi assim que as armas Seladas da vitrine antiga voltariam.
func TestLojaDePontosTemplateSemEstoque(t *testing.T) {
	arquivo := filepath.Join("..", "..", "Release", "TMsrv", "run", "npc", "Loja_de_Pontos")
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

// A 0155 desfaz a 0150: a vitrine pedida era a da Loja de Honra (God of War), e o
// NPC Loja_de_Pontos volta a ficar fora do mundo, sem estoque. O template segue
// vazio (TestLojaDePontosTemplateSemEstoque), então o boot não recoloca nada.
func TestLojaDePontosSaiDeNovo(t *testing.T) {
	b, err := migrations.FS.ReadFile("0155_loja_de_pontos_sai_de_novo.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := semComentariosSQL(string(b))
	for _, quer := range []string{
		"UPDATE npc_definition SET enabled = FALSE WHERE template_name = 'Loja_de_Pontos'",
		"DELETE FROM npc_shop_item WHERE npc_id IN (SELECT id FROM npc_definition WHERE template_name = 'Loja_de_Pontos')",
		"UPDATE npc_config_meta SET version = version + 1",
	} {
		if !strings.Contains(sql, quer) {
			t.Errorf("a 0155 não tem %q", quer)
		}
	}
	if strings.Contains(sql, "DELETE FROM npc_definition") {
		t.Error("a 0155 apaga a definição do NPC; ela só pode desativar")
	}
}
