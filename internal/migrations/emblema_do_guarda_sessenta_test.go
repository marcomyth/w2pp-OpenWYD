package migrations_test

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"testing"

	"github.com/jeanluca/w2pp-openwyd/internal/droprate"
	"github.com/jeanluca/w2pp-openwyd/internal/migrations"
)

const itemEmblemaDoGuarda = 4042

// regrasDoEmblema lê as tuplas (mob, 4042, chance) de uma migração.
func regrasDoEmblema(t *testing.T, arquivo string) map[string]int {
	t.Helper()
	b, err := migrations.FS.ReadFile(arquivo)
	if err != nil {
		t.Fatal(err)
	}
	re := regexp.MustCompile(`\('([^']+)',\s*4042,\s*(\d+)\)`)
	out := map[string]int{}
	for _, m := range re.FindAllStringSubmatch(semComentariosSQL(string(b)), -1) {
		chance, _ := strconv.Atoi(m[2])
		out[m[1]] = chance
	}
	return out
}

// pagaMesa é a chance real de uma regra: a Mesa sorteia rand() % 10000 sobre o
// rand() de 15 bits do MSVC.
func pagaMesa(chance int) float64 {
	n := 0
	for r := 0; r < 32768; r++ {
		if r%10000 < chance {
			n++
		}
	}
	return float64(n) / 32768
}

// pagaTemplate é a chance real de o template soltar o 4042 por alguma das
// vagas, sem bônus de drop: cada vaga rola rand() % taxa == 0.
func pagaTemplate(t *testing.T, mob string) float64 {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "Release", "TMsrv", "run", "npc", mob))
	if err != nil {
		t.Skipf("sem o template em Release/: %v", err)
	}
	const carry, baseLevel = 268, 44 // STRUCT_MOB.Carry[64] e BaseScore.Level
	level := int(int32(binary.LittleEndian.Uint32(b[baseLevel:])))
	nenhuma, vagas := 1.0, 0
	for slot := 0; slot < 64; slot++ {
		if binary.LittleEndian.Uint16(b[carry+slot*8:]) != itemEmblemaDoGuarda {
			continue
		}
		vagas++
		taxa := droprate.EffectiveDropRate(slot, 0, level)
		nenhuma *= 1 - float64(32767/taxa+1)/32768
	}
	if vagas == 0 {
		t.Fatalf("%s não tem o 4042 no Carry; a regra da 0179 não substitui nada", mob)
	}
	return 1 - nenhuma
}

// A 0179 deixa o Emblema do Guarda a 60% do que caía em cada monstro que nasce.
func TestEmblemaDoGuardaSessenta(t *testing.T) {
	depois := regrasDoEmblema(t, "0179_emblema_do_guarda_sessenta.up.sql")

	// Regras que já existiam: 60% do número antigo, exato.
	antes := map[string]int{
		"Aqua_Golem":    regrasDoEmblema(t, "0169_aqua_golem_submundo.up.sql")["Aqua_Golem"],
		"Argos":         50, // painel, 28/09/2026
		"Argos_Errante": 50, // painel, 28/09/2026
	}
	for mob, era := range antes {
		if era == 0 {
			t.Fatalf("%s: sem número antigo; o teste perdeu a referência", mob)
		}
		if got, want := depois[mob], era*6/10; got != want {
			t.Errorf("%s: grava %d, 60%% de %d é %d", mob, got, era, want)
		}
	}

	// Templates: a regra paga entre 50% e 70% do que as vagas pagavam. O piso de
	// 1 da Mesa deixa os Trolls em 67%.
	for _, mob := range []string{"Elfo_Negro_Abj", "Cav._Elfo_Negro", "CH_Troll_Ghoul", "Troll_Ghoul"} {
		chance, ok := depois[mob]
		if !ok {
			t.Errorf("%s ficou fora da 0179", mob)
			continue
		}
		razao := pagaMesa(chance) / pagaTemplate(t, mob)
		if razao < 0.5 || razao > 0.7 {
			t.Errorf("%s: a regra %d paga %.0f%% do template, esperava ~60%%", mob, chance, razao*100)
		}
	}

	if len(depois) != len(antes)+4 {
		t.Errorf("a 0179 tem %d regras do 4042, esperava %d", len(depois), len(antes)+4)
	}

	baixo := regrasDoEmblema(t, "0179_emblema_do_guarda_sessenta.down.sql")
	for mob, era := range antes {
		if baixo[mob] != era {
			t.Errorf("down: %s volta a %d, era %d", mob, baixo[mob], era)
		}
	}
}
