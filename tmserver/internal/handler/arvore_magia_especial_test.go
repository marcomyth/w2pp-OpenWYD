package handler

import (
	"log/slog"
	"testing"

	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/content"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/protocol"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

// fmCancel monta a Foema física: INT e Destreza, com o Cancelamento aprendido.
func fmCancel(intel, dex int16, learned int32, tier uint8) *world.Entity {
	e := &world.Entity{ID: 1, Class: 1, ClassMaster: tier, Int: intel, BaseInt: intel, Dex: dex, BaseDex: dex,
		Str: 12, BaseStr: 12, LearnedSkill: learned}
	e.Special[3] = 255
	return e
}

func TestFmCancelamentoExigeAOitava(t *testing.T) {
	tests := []struct {
		name string
		e    *world.Entity
		want bool
	}{
		{"FM com o Cancelamento", fmCancel(2147, 712, learnedCancelamento, classMasterMortal), true},
		{"FM sem o Cancelamento", fmCancel(2147, 712, 1<<22, classMasterMortal), false},
		{"Huntress com o bit 23", &world.Entity{ID: 1, Class: 3, LearnedSkill: learnedCancelamento}, false},
		{"monstro com o bit 23", &world.Entity{ID: world.MaxUser, Class: 1, LearnedSkill: learnedCancelamento}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := fmCancelamento(tt.e); got != tt.want {
				t.Fatalf("fmCancelamento = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAlvosDoCancelamento(t *testing.T) {
	const espada, machado, garra, cajado, escudo = 101, 102, 103, 104, 105
	ability := armasNasMaos(map[int16]int{
		espada: wtypeUmaMao, machado: wtypeMachadoUmaMaoFM, garra: wtypeGarra, cajado: wtypeCajadoDuasMaos,
	})
	tests := []struct {
		nome        string
		dir, esquer int16
		intel, dex  int16
		tier        uint8
		want        int
	}{
		{"duas espadas", espada, espada, 2147, 712, classMasterMortal, 3},
		{"dois machados", machado, machado, 2147, 712, classMasterMortal, 3},
		{"garra no Arch", garra, garra, 2147, 712, classMasterArch, 3},
		{"garra no Mortal não conta", garra, garra, 2147, 712, classMasterMortal, 2},
		{"espada e escudo", espada, escudo, 2147, 712, classMasterMortal, 1},
		{"cajado de 2 mãos", cajado, 0, 2147, 712, classMasterMortal, 2},
		{"black-cancel: duas espadas com pouca Destreza", espada, espada, 3148, 12, classMasterMortal, 2},
		{"black-cancel: cajado com pouca Destreza", cajado, 0, 3148, 12, classMasterMortal, 1},
		{"black-cancel: escudo com pouca Destreza nunca zera", espada, escudo, 3148, 12, classMasterMortal, 1},
	}
	for _, tt := range tests {
		t.Run(tt.nome, func(t *testing.T) {
			e := fmCancel(tt.intel, tt.dex, learnedCancelamento, tt.tier)
			e.Equip[weaponSlotR], e.Equip[weaponSlotL] = world.Item{Index: tt.dir}, world.Item{Index: tt.esquer}
			if got := alvosDoCancelamento(e, ability); got != tt.want {
				t.Errorf("alvos = %d, want %d", got, tt.want)
			}
		})
	}
}

// O limite de alvos chega ao cast: o servidor recusa o slot acima da conta.
func TestCancelamentoRecusaAlvoAcimaDaConta(t *testing.T) {
	const espada = 101
	d := New(Config{ItemEffects: map[int][]content.BaseEffect{espada: {{Eff: efWType, Val: wtypeUmaMao}}}})
	w := world.New(world.Config{GridDim: 16}, slog.Default(), nil, nil)
	s := &world.Session{Conn: 1}
	e := fmCancel(2147, 712, learnedCancelamento, classMasterMortal)
	e.Equip[weaponSlotR], e.Equip[weaponSlotL] = world.Item{Index: espada}, world.Item{Index: espada}
	alvo := &world.Entity{ID: 2, Class: 0, HP: 100}
	cast := castInfo{isSkill: true, spell: content.Spell{Index: skillCancelamento, TargetType: 5, MaxTarget: 5, Range: 5}}

	for slot := range 5 {
		got := d.validateSkillTarget(w, s, e, alvo, slot, cast, protocol.SkipCheckTick)
		if want := slot < 3; got != want {
			t.Errorf("slot %d aceito = %v, want %v (duas espadas cancelam 3)", slot, got, want)
		}
	}
}

// O dano dela sai da INT no lugar da Força.
func TestDanoDaFmCancelamentoVemDaInt(t *testing.T) {
	com := fmCancel(2000, 600, learnedCancelamento, classMasterMortal)
	sem := fmCancel(2000, 600, 1<<22, classMasterMortal)
	// Com: 2000/2 + 600/3 = 1200; sem: 12/2 + 600/3 = 206.
	if got := attributeDamageBonus(com, true) - attributeDamageLevelTerm(com); got != 1200 {
		t.Errorf("com o Cancelamento: atributos = %d, want 1200 (INT/2 + DES/3)", got)
	}
	if got := attributeDamageBonus(sem, true) - attributeDamageLevelTerm(sem); got != 206 {
		t.Errorf("sem o Cancelamento: atributos = %d, want 206 (FOR/2 + DES/3)", got)
	}
}

// Os buffs valem em dobro nela e o normal em quem não é ela.
func TestBuffsEmDobroNaFmCancelamento(t *testing.T) {
	buffs := func(e *world.Entity) *world.Entity {
		e.Affect[0] = world.Affect{Type: affectVelocidade, Value: 20, Time: 500}
		e.Affect[1] = world.Affect{Type: affectArmaMagica, Value: 90, Level: 200, Time: 500}
		e.Affect[2] = world.Affect{Type: affectEscudoMagico, Value: 15, Level: 150, Time: 500}
		e.Affect[3] = world.Affect{Type: affectToqueAthena, Value: 7, Level: 100, Time: 500}
		applyAffectScoreWithItemAbility(e, nil)
		return e
	}
	com := buffs(fmCancel(2147, 712, learnedCancelamento, classMasterMortal))
	sem := buffs(fmCancel(2147, 712, 1<<22, classMasterMortal))

	if com.AffDamage != 2*sem.AffDamage || com.AffAC != 2*sem.AffAC || com.AffRunSpeed != 2*sem.AffRunSpeed {
		t.Errorf("dano %d/%d, defesa %d/%d, movimento %d/%d — want o dobro em cada",
			com.AffDamage, sem.AffDamage, com.AffAC, sem.AffAC, com.AffRunSpeed, sem.AffRunSpeed)
	}
	if com.AffSpecial[0] != 2*sem.AffSpecial[0] {
		t.Errorf("maestria %d/%d, want o dobro", com.AffSpecial[0], sem.AffSpecial[0])
	}
	// Velocidade de ataque e crítico só nela.
	if com.AffAttackSpeed != velocidadeAtaque || com.AffCritical != velocidadeCritico {
		t.Errorf("velocidade %d e crítico %d, want %d e %d", com.AffAttackSpeed, com.AffCritical, velocidadeAtaque, velocidadeCritico)
	}
	if sem.AffAttackSpeed != 0 || sem.AffCritical != 0 {
		t.Errorf("quem não tem a 8ª: velocidade %d e crítico %d, want 0 e 0", sem.AffAttackSpeed, sem.AffCritical)
	}
}

// Com o Cancelamento, o Controle de Mana deixa passar menos.
func TestControleDeManaDaFmCancelamento(t *testing.T) {
	monta := func(learned int32) *world.Entity {
		e := fmCancel(2147, 712, learned, classMasterMortal)
		e.ID = 2
		e.MaxMP, e.MP = 10_000, 20_000
		e.Affect[0] = world.Affect{Type: 18, Time: 500}
		return e
	}
	com, _, ok := manaControlDamage(monta(learnedCancelamento), 1000, false)
	if !ok {
		t.Fatal("o Controle de Mana tem de pegar")
	}
	sem, _, _ := manaControlDamage(monta(1<<22), 1000, false)
	if com != 201 || sem != 300 {
		t.Errorf("dano que passa = %d com a 8ª e %d sem ela, want 201 e 300", com, sem)
	}
}

// cancelComArmas monta a Foema física com o que estiver nas duas mãos, num
// Dispatcher que conhece o tipo e o dano de cada arma.
func cancelComArmas(learned int32, dir, esq int16) (*Dispatcher, *world.Entity) {
	const espada, machado, escudo = 101, 102, 105
	d := New(Config{ItemEffects: map[int][]content.BaseEffect{
		espada:  {{Eff: efWType, Val: wtypeUmaMao}, {Eff: efDamage, Val: 300}},
		machado: {{Eff: efWType, Val: wtypeMachadoUmaMaoFM}, {Eff: efDamage, Val: 300}},
		escudo:  {{Eff: efDamage, Val: 300}},
	}})
	e := fmCancel(2147, 712, learned, classMasterMortal)
	e.Equip[weaponSlotR], e.Equip[weaponSlotL] = world.Item{Index: dir}, world.Item{Index: esq}
	return d, e
}

// A mão esquerda vale INTEIRA nela, como o Mestre das Armas do TK.
func TestMaoEsquerdaInteiraNoCancelamento(t *testing.T) {
	const espada, machado, escudo = 101, 102, 105
	tests := []struct {
		nome     string
		learned  int32
		dir, esq int16
		want     int32
	}{
		{"duas espadas com a 8ª", learnedCancelamento, espada, espada, 600},
		{"dois machados com a 8ª", learnedCancelamento, machado, machado, 600},
		{"duas espadas sem a 8ª", 1 << 22, espada, espada, 450},
		{"espada e escudo com a 8ª", learnedCancelamento, espada, escudo, 450},
		{"espadas diferentes do par", learnedCancelamento, espada, machado, 450},
	}
	for _, tt := range tests {
		t.Run(tt.nome, func(t *testing.T) {
			d, e := cancelComArmas(tt.learned, tt.dir, tt.esq)
			if got := d.weaponDamage(e); got != tt.want {
				t.Errorf("weaponDamage = %d, want %d", got, tt.want)
			}
		})
	}
}

// A perfuração só existe com duas armas e com a 8ª.
func TestPerfuracaoDoCancelamento(t *testing.T) {
	const espada, escudo = 101, 105
	tests := []struct {
		nome     string
		learned  int32
		dir, esq int16
		want     int
	}{
		{"duas espadas com a 8ª", learnedCancelamento, espada, espada, cancelPerfuracaoPct},
		{"duas espadas sem a 8ª", 1 << 22, espada, espada, 0},
		{"espada e escudo com a 8ª", learnedCancelamento, espada, escudo, 0},
	}
	for _, tt := range tests {
		t.Run(tt.nome, func(t *testing.T) {
			d, e := cancelComArmas(tt.learned, tt.dir, tt.esq)
			if got := perfuracaoDoCancelamento(e, d.itemAbility); got != tt.want {
				t.Errorf("perfuracao = %d, want %d", got, tt.want)
			}
			// E chega à defesa que o golpe enfrenta.
			wantDef := 1000 * (100 - tt.want) / 100
			if got := d.defesaPerfurada(e, 1000); got != wantDef {
				t.Errorf("defesaPerfurada = %d, want %d", got, wantDef)
			}
		})
	}
}

// O bônus de dano com duas armas entra no score, e só com duas armas.
func TestDanoComDuasArmasDoCancelamento(t *testing.T) {
	d, com := cancelComArmas(learnedCancelamento, 101, 101)
	applyAffectScoreWithItemAbility(com, d.itemAbility)
	_, escudo := cancelComArmas(learnedCancelamento, 101, 105)
	applyAffectScoreWithItemAbility(escudo, d.itemAbility)
	if com.AffDamageMultiPct != int32(100+cancelDanoDuasArmas) {
		t.Errorf("com duas armas = %d, want %d", com.AffDamageMultiPct, 100+cancelDanoDuasArmas)
	}
	if escudo.AffDamageMultiPct != 100 {
		t.Errorf("com escudo = %d, want 100", escudo.AffDamageMultiPct)
	}
}

// O Cancelamento tranca a poção por 20 s, e o Escudo de Habilidade da HT come o
// primeiro cancel.
func TestCancelamentoTrancaAPocao(t *testing.T) {
	const agora = 100_000
	tests := []struct {
		nome    string
		escudo  bool
		imune   bool
		trancou bool
	}{
		{"alvo sem escudo", false, false, true},
		{"HT com Escudo de Habilidade", true, false, false},
		{"alvo com o Desintoxicar valendo", false, true, false},
	}
	for _, tt := range tests {
		t.Run(tt.nome, func(t *testing.T) {
			d := New(Config{})
			w := world.New(world.Config{GridDim: 16, Now: func() uint32 { return agora }}, slog.Default(), nil, nil)
			alvo := &world.Entity{ID: 2, HP: 1000}
			if tt.escudo {
				alvo.Affect[0] = world.Affect{Type: 19, Time: 500}
			}
			if tt.imune {
				alvo.ImuneDebuffAte = agora + 1000
			}
			dmg := 0
			d.applySkillSpecial(w, &world.Session{}, &world.Entity{ID: 1}, alvo, 2, 47, castInfo{}, nil, &dmg)
			if got := semPocao(alvo, agora); got != tt.trancou {
				t.Errorf("trancou = %v, want %v", got, tt.trancou)
			}
		})
	}
	// A trava vence: 20 s depois ele bebe de novo.
	alvo := &world.Entity{ID: 2, SemPocaoAte: agora + cancelSemPocaoMs}
	if semPocao(alvo, agora+cancelSemPocaoMs) {
		t.Error("a trava tem de vencer em 20 s")
	}
	// Monstro não bebe: não trancar.
	mob := &world.Entity{ID: world.MaxUser + 1}
	trancarAPocao(mob, world.MaxUser+1, agora)
	if mob.SemPocaoAte != 0 {
		t.Error("monstro não usa poção; não faz sentido trancar")
	}
}

// Trancada, a poção é RECUSADA no caminho real do item: o alvo da barra não
// sobe e a pilha não é consumida.
func TestPocaoRecusadaComATranca(t *testing.T) {
	const pocao, agora = 404, 100_000 // Ultra Poção de Cura: EF_HP 500, EF_MP 500
	monta := func(trancado bool) (*Dispatcher, *world.World, *world.Session, *world.Entity) {
		d := New(Config{ItemEffects: map[int][]content.BaseEffect{
			pocao: {{Eff: efVolatile, Val: volHpMpPotion}, {Eff: efHp, Val: 500}, {Eff: efMp, Val: 500}},
		}})
		w := world.New(world.Config{GridDim: 16, Now: func() uint32 { return agora }}, slog.Default(), nil, nil)
		s := &world.Session{Conn: 1}
		// Vida baixa de propósito: o ReqHp nunca fica abaixo do HP atual (hpmp.go),
		// e com a barra cheia o piso engoliria o ganho da poção.
		e := &world.Entity{ID: 1, HP: 100, MaxHP: 10_000, MP: 100, MaxMP: 10_000}
		e.Carry[0] = world.Item{Index: pocao, Effects: [3]world.Effect{{Effect: efAmount, Value: 10}}}
		if trancado {
			e.SemPocaoAte = agora + cancelSemPocaoMs
		}
		return d, w, s, e
	}

	d, w, s, e := monta(false)
	d.useHealPotion(w, s, e, 0)
	if s.ReqHp != 500 || s.ReqMp != 500 {
		t.Fatalf("sem tranca: ReqHp %d ReqMp %d, want 500 e 500 (o cenário não prova nada)", s.ReqHp, s.ReqMp)
	}

	d, w, s, e = monta(true)
	d.useHealPotion(w, s, e, 0)
	if s.ReqHp != 0 || s.ReqMp != 0 {
		t.Errorf("trancado: ReqHp %d ReqMp %d, want 0 e 0 — a poção não pode valer", s.ReqHp, s.ReqMp)
	}
	if got := int(e.Carry[0].Effects[0].Value); got != 10 {
		t.Errorf("pilha = %d, want 10 (nada consumido)", got)
	}
}

// O ARCO da FM Cancelamento (20/09/2026). Ele não era penalizado — só não
// ganhava nada, e valia o mesmo que uma espada sozinha. Agora tem bônus próprio,
// menor que o de duas armas.
func TestArcoDoCancelamento(t *testing.T) {
	const espada, arco, dardo, escudo = 201, 202, 203, 204
	ability := armasNasMaos(map[int16]int{
		espada: wtypeUmaMao, arco: wtypeArco, dardo: 102,
	})
	fm := func(dir, esq int16, learned int32) *world.Entity {
		e := &world.Entity{ID: 1, Class: 1, Level: 399, LearnedSkill: learned, Int: 2147, Dex: 712}
		e.Equip[weaponSlotR], e.Equip[weaponSlotL] = world.Item{Index: dir}, world.Item{Index: esq}
		return e
	}
	for _, c := range []struct {
		nome        string
		dir, esquer int16
		want        bool
	}{
		{"arco", arco, 0, true},
		{"espada", espada, 0, false},
		{"duas espadas", espada, espada, false},
		{"dardo NÃO é arco", dardo, 0, false},
		{"arma e escudo", espada, escudo, false},
		{"sem arma", 0, 0, false},
	} {
		if got := arcoDoCancelamento(fm(c.dir, c.esquer, learnedCancelamento), ability); got != c.want {
			t.Errorf("%s: %v, want %v", c.nome, got, c.want)
		}
	}
	// Sem a 8ª a árvore não vale, e sem itemAbility o servidor não sabe a arma.
	if arcoDoCancelamento(fm(arco, 0, 0), ability) {
		t.Error("sem o Cancelamento a regra não pode valer")
	}
	if arcoDoCancelamento(fm(arco, 0, learnedCancelamento), nil) {
		t.Error("sem itemAbility não se dá bônus de graça")
	}
	if arcoDoCancelamento(nil, ability) {
		t.Error("nil não entra na árvore")
	}
}

// O bônus entra no SCORE, e a empunhadura dá UM só: duas armas OU arco, nunca
// os dois somados.
func TestBonusDeDanoPorEmpunhaduraDaCancel(t *testing.T) {
	const espada, arco = 201, 202
	ability := armasNasMaos(map[int16]int{espada: wtypeUmaMao, arco: wtypeArco})
	com := func(dir, esq int16) int32 {
		e := &world.Entity{ID: 1, Class: 1, Level: 399, LearnedSkill: learnedCancelamento, Int: 2147, Dex: 712}
		e.Equip[weaponSlotR], e.Equip[weaponSlotL] = world.Item{Index: dir}, world.Item{Index: esq}
		applyAffectScoreWithItemAbility(e, ability)
		return e.AffDamageMultiPct
	}
	if got := com(espada, espada); got != int32(100+cancelDanoDuasArmas) {
		t.Errorf("duas espadas: %d%%, want %d%%", got, 100+cancelDanoDuasArmas)
	}
	if got := com(arco, 0); got != int32(100+cancelDanoArco) {
		t.Errorf("arco: %d%%, want %d%%", got, 100+cancelDanoArco)
	}
	if got := com(espada, 0); got != 100 {
		t.Errorf("uma espada só: %d%%, want 100%%", got)
	}
	// O arco tem de valer MENOS que duas armas: são elas que levam também o alvo
	// a mais no Cancelamento e a perfuração de armadura.
	if cancelDanoArco >= cancelDanoDuasArmas {
		t.Errorf("o arco (%d) não pode alcançar as duas armas (%d)", cancelDanoArco, cancelDanoDuasArmas)
	}
	// E tem de valer mais que nada: era esse o problema.
	if cancelDanoArco <= 0 {
		t.Errorf("o arco precisa de bônus, veio %d", cancelDanoArco)
	}
}
