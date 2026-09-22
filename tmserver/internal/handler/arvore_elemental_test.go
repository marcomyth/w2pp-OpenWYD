package handler

import (
	"log/slog"
	"testing"

	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/content"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

// bmDaElemental monta um Beast Master com a 8ª da árvore (Espírito Vingador).
func bmDaElemental(learned int32) *world.Entity {
	return &world.Entity{ID: 1, Class: 2, Level: 399, LearnedSkill: learned,
		MaxMP: 10_000, MP: 1_000, MaxHP: 10_000, HP: 1_000, Int: 2500, BaseInt: 2500}
}

func TestArmaDaElemental(t *testing.T) {
	const cajado2, cajado1, escudo, lanca, machado = 910, 904, 920, 855, 700
	ability := armasNasMaos(map[int16]int{
		cajado2: wtypeCajadoDuasMaos, cajado1: wtypeCajadoUmaMao,
		escudo: 51, lanca: wtypeLanca, machado: wtypeMachadoUmaMao,
	})
	for _, c := range []struct {
		nome        string
		dir, esquer int16
		want        int
	}{
		{"lança", lanca, 0, elementalLancaPct},
		{"cajado de 2 mãos", cajado2, 0, elementalCajado2MaosPct},
		{"cajado de 1 mão com escudo não ganha bônus", cajado1, escudo, elementalArmaPct},
		{"cajado de 1 mão sozinho", cajado1, 0, elementalArmaPct},
		{"machado", machado, 0, elementalArmaPct},
		{"sem arma", 0, 0, elementalArmaPct},
	} {
		e := bmDaElemental(learnedEspiritoVingador)
		e.Equip[weaponSlotR], e.Equip[weaponSlotL] = world.Item{Index: c.dir}, world.Item{Index: c.esquer}
		if got := armaPctElemental(e, ability); got != c.want {
			t.Errorf("%s: %d%%, want %d%%", c.nome, got, c.want)
		}
	}
	// Sem itemAbility o servidor não sabe qual arma está na mão: neutro, nunca o
	// bônus — um nil aqui não pode virar 140% de graça.
	e := bmDaElemental(learnedEspiritoVingador)
	e.Equip[weaponSlotR] = world.Item{Index: cajado2}
	if got := armaPctElemental(e, nil); got != elementalArmaPct {
		t.Errorf("sem itemAbility = %d%%, want %d%%", got, elementalArmaPct)
	}
}

// A árvore só vale para BM com a 8ª. Sem ela, ou em outra classe, a arma não
// multiplica nada.
func TestElementalExigeClasseEOitava(t *testing.T) {
	if !bmElemental(bmDaElemental(learnedEspiritoVingador)) {
		t.Fatal("BM com o Espírito Vingador tinha de valer")
	}
	if bmElemental(bmDaElemental(0)) {
		t.Error("sem a 8ª a árvore não pode valer")
	}
	outraClasse := bmDaElemental(learnedEspiritoVingador)
	outraClasse.Class = 0
	if bmElemental(outraClasse) {
		t.Error("TK não pode entrar na árvore do BM")
	}
	mob := bmDaElemental(learnedEspiritoVingador)
	mob.ID = world.MaxUser + 1
	if bmElemental(mob) {
		t.Error("monstro não pode entrar na árvore")
	}
	if bmElemental(nil) {
		t.Error("nil não pode entrar na árvore")
	}
}

// As skills de dano são as da faixa 48-55, menos as três que não ferem.
func TestSkillDeDanoDaElemental(t *testing.T) {
	for _, sk := range []int{48, 49, 50, 52, 55} {
		if !skillDeDanoDaElemental(sk) {
			t.Errorf("skill %d tinha de ser de dano", sk)
		}
	}
	// 51 Enfraquecer (debuff), 53 Proteção Elemental e 54 Aura Bestial (buff).
	for _, sk := range []int{51, 53, 54} {
		if skillDeDanoDaElemental(sk) {
			t.Errorf("skill %d NÃO é de dano", sk)
		}
	}
	// Fora da faixa: 47 é o Cancelamento da FM, 56 a primeira evocação.
	for _, sk := range []int{0, 47, 56, 63, 71} {
		if skillDeDanoDaElemental(sk) {
			t.Errorf("skill %d está fora da árvore Elemental", sk)
		}
	}
}

// A ARMA escolhe a barra, e nunca as duas ao mesmo tempo.
func TestRouboDaElementalSegueAArma(t *testing.T) {
	const cajado2, lanca, machado = 910, 855, 700
	d := New(Config{Log: slog.New(slog.DiscardHandler), ItemEffects: map[int][]content.BaseEffect{
		cajado2: {{Eff: efWType, Val: wtypeCajadoDuasMaos}},
		lanca:   {{Eff: efWType, Val: wtypeLanca}},
		machado: {{Eff: efWType, Val: wtypeMachadoUmaMao}},
	}})
	// Um rand que sempre tira 0 faz os DOIS sorteios passarem: se a regra
	// deixasse, vida e mana viriam juntas, e é isso que se está provando que não.
	sempre := randSempreZero{}
	com := func(arma int16, learned int32) *world.Entity {
		e := bmDaElemental(learned)
		e.Equip[weaponSlotR] = world.Item{Index: arma}
		return e
	}

	if vida, mana := d.rouboDaElemental(sempre, com(lanca, learnedEspiritoVingador), 1000, 10_000); vida <= 0 || mana != 0 {
		t.Errorf("lança: vida %d (quero >0), mana %d (quero 0)", vida, mana)
	}
	if vida, mana := d.rouboDaElemental(sempre, com(cajado2, learnedEspiritoVingador), 1000, 10_000); mana <= 0 || vida != 0 {
		t.Errorf("cajado de 2 mãos: mana %d (quero >0), vida %d (quero 0)", mana, vida)
	}
	if vida, mana := d.rouboDaElemental(sempre, com(machado, learnedEspiritoVingador), 1000, 10_000); vida != 0 || mana != 0 {
		t.Errorf("arma sem bônus não repõe nada: vida %d, mana %d", vida, mana)
	}
	if vida, mana := d.rouboDaElemental(sempre, com(lanca, 0), 1000, 10_000); vida != 0 || mana != 0 {
		t.Errorf("sem a 8ª não repõe nada: vida %d, mana %d", vida, mana)
	}
	if vida, mana := d.rouboDaElemental(sempre, com(lanca, learnedEspiritoVingador), 0, 10_000); vida != 0 || mana != 0 {
		t.Errorf("golpe de 0 não repõe nada: vida %d, mana %d", vida, mana)
	}
	// O teto do lançamento vale: sem espaço sobrando, a mana não vem.
	if _, mana := d.rouboDaElemental(sempre, com(cajado2, learnedEspiritoVingador), 1000, 0); mana != 0 {
		t.Errorf("com o teto esgotado a mana tem de ser 0, veio %d", mana)
	}
}

// O BM NÃO tem crítico de mago — é a diferença deliberada para o TK Espada
// Mágica e a FM Magia Negra, o preço de trazer o bando de evocações junto.
func TestElementalNaoTemCriticoDeMago(t *testing.T) {
	e := bmDaElemental(0xFFFFFF)
	for _, sk := range []int{48, 49, 50, 52, 55} {
		if magoCritico(e, sk) {
			t.Errorf("skill %d: o BM Elemental não pode ter crítico de mago", sk)
		}
	}
}

// randSempreZero faz todo sorteio de chance passar.
type randSempreZero struct{}

func (randSempreZero) Intn(int) int { return 0 }

// A absorção da Proteção Elemental exige as DUAS coisas: a 8ª aprendida e o
// buff no ar. Cada uma sozinha não dá nada.
func TestAbsorcaoDaProtecaoElemental(t *testing.T) {
	comProtecao := func(e *world.Entity) *world.Entity {
		e.Affect[0] = world.Affect{Type: protecaoElementalAfeto, Value: 10, Level: 100, Time: 500}
		return e
	}
	const golpe = 1000
	quer := golpe - golpe*protecaoElementalAbsPct/100

	if got := absorcaoDaProtecaoElemental(comProtecao(bmDaElemental(learnedEspiritoVingador)), golpe); got != quer {
		t.Errorf("com a 8ª e a proteção: %d, want %d", got, quer)
	}
	// Só o buff, sem a 8ª: nada.
	if got := absorcaoDaProtecaoElemental(comProtecao(bmDaElemental(0)), golpe); got != golpe {
		t.Errorf("sem a 8ª: %d, want %d (sem absorção)", got, golpe)
	}
	// Só a 8ª, sem o buff: nada. Ela é um buff que expira, não uma passiva.
	if got := absorcaoDaProtecaoElemental(bmDaElemental(learnedEspiritoVingador), golpe); got != golpe {
		t.Errorf("sem o buff no ar: %d, want %d (sem absorção)", got, golpe)
	}
	// Outra classe com o mesmo afeto não ganha nada.
	outra := comProtecao(bmDaElemental(learnedEspiritoVingador))
	outra.Class = 1
	if got := absorcaoDaProtecaoElemental(outra, golpe); got != golpe {
		t.Errorf("outra classe: %d, want %d", got, golpe)
	}
	// Golpe que não entrou continua não entrando, e o piso é 1.
	if got := absorcaoDaProtecaoElemental(comProtecao(bmDaElemental(learnedEspiritoVingador)), 0); got != 0 {
		t.Errorf("golpe 0: %d, want 0", got)
	}
	if got := absorcaoDaProtecaoElemental(comProtecao(bmDaElemental(learnedEspiritoVingador)), 1); got != 1 {
		t.Errorf("golpe 1: %d, want o piso 1", got)
	}
	if got := absorcaoDaProtecaoElemental(nil, golpe); got != golpe {
		t.Errorf("nil: %d, want %d", got, golpe)
	}
}

// A absorção tem de estar LIGADA no absorbBlow, que é o ponto único por onde
// todo golpe passa. A função certa existindo e ninguém a chamando é o modo mais
// comum de uma regra nova não valer nada em jogo.
func TestAbsorcaoDaProtecaoElementalEntraNoAbsorbBlow(t *testing.T) {
	log := slog.New(slog.DiscardHandler)
	d := New(Config{Log: log, CombatRules: regraSemEscala()})
	w := world.New(world.Config{GridDim: 16}, log, nil, nil)
	const golpe = 1000
	quer := golpe - golpe*protecaoElementalAbsPct/100

	protegido := bmDaElemental(learnedEspiritoVingador)
	protegido.Affect[0] = world.Affect{Type: protecaoElementalAfeto, Value: 10, Level: 100, Time: 500}
	if got := d.absorbBlow(w, protegido, golpe, true); got != quer {
		t.Errorf("golpe de jogador: absorbBlow = %d, want %d", got, quer)
	}
	// E também no golpe de MONSTRO: é defesa do personagem, não regra de arena.
	if got := d.absorbBlow(w, protegido, golpe, false); got != quer {
		t.Errorf("golpe de monstro: absorbBlow = %d, want %d", got, quer)
	}

	// Sem a proteção no ar o absorbBlow devolve o golpe inteiro.
	semBuff := bmDaElemental(learnedEspiritoVingador)
	if got := d.absorbBlow(w, semBuff, golpe, true); got != golpe {
		t.Errorf("sem o buff: absorbBlow = %d, want %d", got, golpe)
	}
}
