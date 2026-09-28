package content

import "testing"

// Os Aqua Golem do Submundo nascem em trio desde 26/09/2026: pedido de ao menos
// três vezes mais golems na área. Os 35 blocos ativos passam de 1 golem para um
// líder com dois seguidores (MaxNumMob 3). Os sete blocos desativados no arquivo
// original (MinuteGenerate -1) continuam desativados.
//
// Lido do arquivo de verdade, e com o número exato de blocos: o teste tem de
// falhar se alguém tirar um trio, e também se o parser deixar de achar os
// blocos — passar por não ter olhado nada é o modo de falha a evitar.
func TestAquaGolemDoSubmundoNasceEmTrio(t *testing.T) {
	gens, err := LoadNPCGenerators(release(t, "TMsrv", "run", "NPCGener.txt"))
	if err != nil {
		t.Skipf("Release content unavailable: %v", err)
	}
	ativos, desativados := 0, 0
	for i, g := range gens {
		if g.Leader != "Aqua_Golem" {
			continue
		}
		if g.MinuteGenerate < 0 {
			desativados++
			continue
		}
		ativos++
		if g.MaxNumMob != 3 || g.MinGroup != 2 || g.MaxGroup != 2 || g.Follower != "Aqua_Golem" {
			t.Errorf("bloco %d: MaxNumMob %d, grupo %d-%d, seguidor %q; quer trio de Aqua_Golem (3, 2-2)",
				i, g.MaxNumMob, g.MinGroup, g.MaxGroup, g.Follower)
		}
	}
	if ativos != 35 || desativados != 7 {
		t.Errorf("blocos de Aqua_Golem: %d ativos e %d desativados; quer 35 e 7", ativos, desativados)
	}
}
