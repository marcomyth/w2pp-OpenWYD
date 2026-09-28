package level

// O RASTRO DA XP: cada etapa do pagamento de uma morte, em números.
//
// POR QUE ELE EXISTE. Em 24/09/2026 a Hanna mediu, em jogo, três valores de XP por
// morte no deserto. Reconstruí o cálculo inteiro a partir do código e NENHUM dos
// três fechava — nem variando o monstro entre os seis moldes Tauron, nem varrendo o
// bônus de 0 a 400%, nem com Kefra dos dois jeitos, nem com o dobro ligado. Também
// não era a Mesa: invertendo duas medições, uma exigiria divisor 3,49 e a outra
// 6,70, quando as duas caem na mesma faixa.
//
// Ou seja: o modelo estava errado em algum ponto, e descobrir ONDE por tentativa e
// erro custaria uma rodada de perguntas por hipótese. O servidor não registrava nada
// sobre a conta que ele mesmo faz — o único jeito de saber quanto ele pagou era
// alguém ler o número na tela do jogo e contar.
//
// Com o rastro, uma morte responde tudo: qual molde, qual MobExp ele carregou de
// verdade, qual divisor pegou, qual bônus entrou, e o valor em cada etapa.
//
// ELE É TEMPORÁRIO. Sai assim que a calibração da Mesa fechar — está escrito no PR
// que o trouxe. Fica até lá porque a pergunta "por que este monstro pagou isto?"
// vai se repetir a cada recalibração, e hoje ela não tem resposta barata.
//
// CUSTO ZERO QUANDO DESLIGADO: o campo Trace do ExpRewardInput é um ponteiro, e nil
// é o caminho normal. Quem não pede rastro não paga nada além de um teste de nil.
type ExpTrace struct {
	// De onde veio a conta.
	Zona       Zone
	Tier       uint8
	NivelMatou int32 // nível do personagem, como o servidor o guarda (interno)
	NivelMolde int32
	MobExp     int64

	// Cada etapa, na ordem em que o servidor as aplica.
	IsExp        int64   // depois do ExpApply (a escala por diferença de nível)
	EMob         int64   // o teto do golpe final
	Bruto        int64   // 450*isExp/(30+nivel), ou o próprio isExp nas zonas de base identidade
	PassouOGate  bool    // false = o bruto estourou os 10 mi e a recompensa virou zero
	DivisorUsado float64 // o divisor da Mesa que pegou (0 = nenhum corte casou)
	DivisorAte   int32   // o teto da faixa que casou, para achar a linha na Mesa
	DepoisCortes int64
	DepoisSeis   int64 // depois do 6/10
	TetouNoEMob  bool
	BonusPercent int32 // o bônus efetivamente aplicado (já somada a fada)
	DepoisBonus  int64
	DepoisDobro  int64
	DepoisKefra  int64
	DepoisNovato int64 // o ±15%
	TaxaPercent  int32
	Final        int64
}
