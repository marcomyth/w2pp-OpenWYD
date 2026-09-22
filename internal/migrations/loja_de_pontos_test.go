package migrations_test

import (
	"strings"
	"testing"

	"github.com/jeanluca/w2pp-openwyd/internal/migrations"
)

// A 0096 tira a Loja de Pontos do mundo, e precisa de DUAS instruções para isso.
//
// A Loja de Pontos não está no seed da 0006: a linha dela nasce da reconciliação
// do catálogo de conteúdo, que o dbServer roda no boot DEPOIS de store.Migrate.
// Numa base nova o UPDATE não acha linha nenhuma, a seed cria o NPC com enabled
// no padrão TRUE e a limpeza some sem aviso — num Postgres embutido, tirar o
// INSERT faz exatamente isso acontecer. O INSERT deixa a linha já desativada, e
// o upsert da seed não toca em `enabled`.
//
// Este teste existe porque a segunda instrução PARECE redundante ao lado da
// primeira, e é a que alguém tiraria numa limpeza.
func TestLojaDePontosDesativaNosDoisCaminhos(t *testing.T) {
	b, err := migrations.FS.ReadFile("0096_loja_de_pontos_fora_do_mundo.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := semComentariosSQL(string(b))

	for _, quer := range []string{
		// A base que já está no ar: pega a linha qualquer que seja o slug dela.
		"UPDATE npc_definition SET enabled = FALSE WHERE template_name = 'Loja_de_Pontos'",
		// A base nova: a linha nasce desativada, antes de a seed chegar.
		"INSERT INTO npc_definition",
		"'Loja_de_Pontos-6144'",
		"ON CONFLICT (slug) DO UPDATE SET enabled = FALSE",
		// E o jogo relê a configuração.
		"UPDATE npc_config_meta SET version = version + 1",
	} {
		if !strings.Contains(sql, quer) {
			t.Errorf("a 0096 não tem %q", quer)
		}
	}

	// Desativa, não apaga: o slug carrega a posição do bloco no NPCGener, e
	// apagar a linha desloca o índice de todo NPC seguinte (mesma razão da 0083).
	if strings.Contains(sql, "DELETE FROM npc_definition") {
		t.Error("a 0096 apaga a definição; ela só pode desativar")
	}

	volta, err := migrations.FS.ReadFile("0096_loja_de_pontos_fora_do_mundo.down.sql")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(semComentariosSQL(string(volta)), "SET enabled = TRUE WHERE template_name = 'Loja_de_Pontos'") {
		t.Error("a volta da 0096 não devolve a Loja de Pontos ao mundo")
	}
}
