package level

import "testing"

// entradaDoDeserto é uma morte no deserto, do jeito que a Hanna mede: Mortal,
// Tauron, Kefra vivo (que no código é KefraLive=false).
func entradaDoDeserto(nivelTela int32, bonus, fada int32) ExpRewardInput {
	return ExpRewardInput{
		Zone:         ZoneDesertoLugefer,
		MobExp:       1_150_000,
		KillerLevel:  nivelTela - 1, // o servidor guarda o nível interno
		MobLevel:     351,
		Tier:         Tier{ClassMaster: classMortal},
		ExpBonus:     bonus,
		FairyContent: fada,
		Events:       ExpEvents{KefraLive: false},
	}
}

// O RASTRO NÃO PODE MUDAR O QUE ELE MEDE. É a regra inteira deste arquivo: uma
// ferramenta de diagnóstico que altera o resultado transforma a investigação em
// perseguição do próprio instrumento — e hoje foi justamente o instrumento errado
// que custou a tarde.
func TestORastroNaoMudaOResultado(t *testing.T) {
	for _, tela := range []int32{202, 264, 270, 282, 350} {
		semRastro := ExpReward(entradaDoDeserto(tela, 100, 32))

		in := entradaDoDeserto(tela, 100, 32)
		in.Trace = &ExpTrace{}
		comRastro := ExpReward(in)

		if semRastro != comRastro {
			t.Errorf("tela %d: sem rastro %d, com rastro %d", tela, semRastro, comRastro)
		}
		if in.Trace.Final != comRastro {
			t.Errorf("tela %d: o rastro diz %d e a funcao devolveu %d",
				tela, in.Trace.Final, comRastro)
		}
	}
}

// AS ETAPAS SAÍDAS TÊM DE SER A CONTA DE VERDADE, e não campos preenchidos por
// educação: cada uma é conferida contra a etapa seguinte.
func TestORastroContaAHistoriaInteira(t *testing.T) {
	in := entradaDoDeserto(282, 100, 32)
	in.Trace = &ExpTrace{}
	ExpReward(in)
	tr := in.Trace

	if tr.MobExp != 1_150_000 || tr.NivelMolde != 351 {
		t.Errorf("o rastro nao guardou o molde: exp=%d nivel=%d", tr.MobExp, tr.NivelMolde)
	}
	if tr.IsExp <= 0 {
		t.Fatalf("isExp = %d", tr.IsExp)
	}
	if !tr.PassouOGate {
		t.Fatalf("no 282 a recompensa nao deveria estourar o teto; bruto=%d", tr.Bruto)
	}
	// O 6/10 é a única etapa cujo valor exato dá para afirmar sem repetir a conta.
	if querSeis := 6 * tr.DepoisCortes / 10; tr.DepoisSeis != querSeis {
		t.Errorf("depois do 6/10 = %d, a conta dos cortes dá %d", tr.DepoisSeis, querSeis)
	}
	// O bônus somado tem de ser o item mais a fada, porque o deserto conta a fada.
	if tr.BonusPercent != 132 {
		t.Errorf("bonus aplicado = %d%%, queria 132%% (100 do item + 32 da fada)", tr.BonusPercent)
	}
	// Kefra vivo corta pela metade, e é a etapa que mais confunde quem lê o código
	// (a flag se chama KefraLive e vale false quando ele está VIVO).
	if tr.DepoisKefra != tr.DepoisDobro/2 {
		t.Errorf("depois do Kefra = %d, queria a metade de %d", tr.DepoisKefra, tr.DepoisDobro)
	}
}

// E O RASTRO NIL CONTINUA SENDO O CAMINHO NORMAL: nada estoura sem ele.
func TestSemRastroNaoEstoura(t *testing.T) {
	if got := ExpReward(entradaDoDeserto(282, 100, 32)); got <= 0 {
		t.Errorf("sem rastro a recompensa saiu %d", got)
	}
}
