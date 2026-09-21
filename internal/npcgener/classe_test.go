package npcgener

import "testing"

// A lista do Coliseu tem de continuar com exatamente os 26 blocos de população.
// Esta conferência veio de tmserver/internal/world/coliseu_test.go junto com a
// lista: é lá que ela enxerga o mapa. O teste do world ficou com o lado dele —
// que estes 26 não nascem no boot e não voltam pela fila de 15 s.
func TestColiseuListaTem26Blocos(t *testing.T) {
	coliseu := []int{0, 1, 2, 5, 6, 7,
		4854, 4855, 4856, 4857, 4858, 4859, 4860, 4861, 4862, 4863,
		4865, 4866, 4867, 4868, 4869, 4870, 4871, 4872, 4873, 4874}
	if len(coliseu) != 26 || len(coliseuGenerators) != 26 {
		t.Fatalf("lista do teste %d, lista do código %d; want 26 e 26", len(coliseu), len(coliseuGenerators))
	}
	for _, idx := range coliseu {
		if !IsColiseuGenerator(idx) {
			t.Errorf("bloco %d saiu da lista do Coliseu", idx)
		}
	}
}

// O recorte não pode ter mudado resposta nenhuma. Os números conferidos aqui são
// os que o boot do tmserver usa para decidir o que povoar, e cada um deles já
// custou um defeito de produção quando esteve errado.
func TestClasseDoBloco(t *testing.T) {
	casos := []struct {
		nome string
		idx  int
		want Classe
	}{
		{"primeira sala da Água N", WaterGenBaseN, ClasseMasmorra},
		{"última sala numerada da Água N", WaterGenBaseN + 7, ClasseMasmorra},
		{"último bloco de chefe da Água N", WaterGenBaseN + 11, ClasseMasmorra},
		// Cuidado com a vizinhança: WaterGenBaseN+12 é 183, que é a Água A — as
		// três correntes são coladas. O primeiro bloco livre é depois da A.
		{"logo depois da Água A", WaterGenBaseA + 12, ClasseMundo},
		{"primeira sala da Água M", WaterGenBaseM, ClasseMasmorra},
		{"primeira sala da Água A", WaterGenBaseA, ClasseMasmorra},
		{"Kefra", KefraBossGenIndex, ClasseChefe},
		{"último guarda do Kefra", KefraGuardLast, ClasseChefe},
		{"logo depois dos guardas", KefraGuardLast + 1, ClasseMundo},
		{"Sala Secreta, primeiro", SecretRoomGenFirst, ClasseEvento},
		{"Sala Secreta, último", SecretRoomGenLast, ClasseEvento},
		{"Krill perdido na Sala Secreta", SecretRoomStrayGenFirst, ClasseEvento},
		{"Coliseu", 4854, ClasseEvento},
		{"torre da guerra de guilda", 1078, ClasseEvento},
		{"Castelo Orc", CasteloOrcGenFirst, ClasseEvento},
		{"Acampamento Troll", AcampamentoTrollGenLast, ClasseEvento},
		{"Tauron do Pilar", 3151, ClasseMundo},
		{"chefe sozinho do Coliseu", 4864, ClasseMundo},
	}
	for _, c := range casos {
		if got := ClasseDoBloco(c.idx); got != c.want {
			t.Errorf("%s (bloco %d): classe %v, want %v", c.nome, c.idx, got, c.want)
		}
	}
}

// NasceNoMundo é o filtro padrão da lista de monstros do painel. A masmorra e o
// chefe PASSAM: os oito moldes da perga foram onde a XP de 20/09 foi medida, e
// esconder justamente esses deixaria a tela inútil para o balanceamento.
func TestNasceNoMundoMantemMasmorraEChefe(t *testing.T) {
	passam := map[string]int{
		"campo":         3151,
		"sala da perga": WaterGenBaseN + 3,
		"Kefra":         KefraBossGenIndex,
	}
	for nome, idx := range passam {
		if !NasceNoMundo(idx) {
			t.Errorf("%s (bloco %d) sumiu do filtro padrão", nome, idx)
		}
	}
	naoPassam := map[string]int{
		"Sala Secreta":      SecretRoomGenFirst,
		"Coliseu":           0,
		"torre de guerra":   1078,
		"Castelo Orc":       CasteloOrcGenFirst,
		"Acampamento Troll": AcampamentoTrollGenFirst,
	}
	for nome, idx := range naoPassam {
		if NasceNoMundo(idx) {
			t.Errorf("%s (bloco %d) passou no filtro padrão: foi assim que a Sala Secreta"+
				" entrou no modelo de XP como campo aberto em 16/09", nome, idx)
		}
	}
}

func TestClasseNome(t *testing.T) {
	for _, c := range []struct {
		c    Classe
		want string
	}{
		{ClasseMundo, "mundo"},
		{ClasseMasmorra, "masmorra"},
		{ClasseChefe, "chefe"},
		{ClasseEvento, "evento"},
	} {
		if got := c.c.Nome(); got != c.want {
			t.Errorf("Nome(%v) = %q, want %q", c.c, got, c.want)
		}
	}
}
