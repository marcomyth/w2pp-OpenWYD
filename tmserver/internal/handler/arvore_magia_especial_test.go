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
