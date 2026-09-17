package handler

import (
	"context"
	"io"
	"log/slog"
	"net"
	"sync/atomic"
	"testing"

	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/content"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/protocol"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

// fmBranca monta uma Foema de cura com o HP dado.
func fmBranca(maxHP int32, learned int32, tier uint8) *world.Entity {
	e := &world.Entity{ID: 1, Class: 1, ClassMaster: tier, MaxHP: maxHP / 2, HP: maxHP, LearnedSkill: learned}
	e.Special[1] = 255
	return e
}

func TestFmMagiaBrancaExigeRenascimento(t *testing.T) {
	tests := []struct {
		name string
		e    *world.Entity
		want bool
	}{
		{"FM com o Renascimento", fmBranca(30_000, learnedRenascimento, classMasterMortal), true},
		{"FM sem o Renascimento", fmBranca(30_000, 1<<6, classMasterMortal), false},
		{"TK com o bit 7 (Destino)", &world.Entity{ID: 1, Class: 0, LearnedSkill: learnedRenascimento}, false},
		{"monstro com o bit 7", &world.Entity{ID: world.MaxUser, Class: 1, LearnedSkill: learnedRenascimento}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := fmMagiaBranca(tt.e); got != tt.want {
				t.Fatalf("fmMagiaBranca = %v, want %v", got, tt.want)
			}
		})
	}
}

// A vida que conta para a cura para no teto da evolução.
func TestVidaQueConta(t *testing.T) {
	for _, c := range []struct {
		nome  string
		maxHP int32
		tier  uint8
		want  int32
	}{
		{"Mortal abaixo do teto", 20_000, classMasterMortal, 20_000},
		{"Mortal acima do teto", 50_000, classMasterMortal, 30_000},
		{"Arch acima do teto", 90_000, classMasterArch, 40_000},
		{"Celestial acima do teto", 90_000, classMasterCelestial, 50_000},
	} {
		if got := vidaQueConta(fmBranca(c.maxHP, learnedRenascimento, c.tier)); got != c.want {
			t.Errorf("%s: %d, want %d", c.nome, got, c.want)
		}
	}
}

// A Cura soma 10% da vida dela e o Recuperar 6%; o cajado de 1 mão soma 40%.
func TestCuraDaMagiaBranca(t *testing.T) {
	const cajado1, cajado2 = 904, 910
	ability := armasNasMaos(map[int16]int{cajado1: wtypeCajadoUmaMao, cajado2: wtypeCajadoDuasMaos})
	tests := []struct {
		nome    string
		skill   int
		arma    int16
		learned int32
		want    int
	}{
		{"Cura sem arma", skillCura, 0, learnedRenascimento, 500 + 3000},
		{"Cura com cajado de 1 mão", skillCura, cajado1, learnedRenascimento, (500 + 3000) * 140 / 100},
		{"Cura com cajado de 2 mãos", skillCura, cajado2, learnedRenascimento, 500 + 3000},
		{"Recuperar sem arma", skillRecuperar, 0, learnedRenascimento, 500 + 1800},
		{"sem a 8ª nada muda", skillCura, cajado1, 1 << 6, 500},
		{"outra skill nada muda", skillFlechaMagica, cajado1, learnedRenascimento, 500},
	}
	for _, tt := range tests {
		t.Run(tt.nome, func(t *testing.T) {
			e := fmBranca(30_000, tt.learned, classMasterMortal)
			e.Equip[weaponSlotR] = world.Item{Index: tt.arma}
			if got := curaDaMagiaBranca(e, tt.skill, 500, ability); got != tt.want {
				t.Errorf("cura = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestTetoDaCura(t *testing.T) {
	com := fmBranca(30_000, learnedRenascimento, classMasterMortal)
	sem := fmBranca(30_000, 1<<6, classMasterMortal)
	for _, c := range []struct {
		nome  string
		e     *world.Entity
		skill int
		want  int
	}{
		{"Cura com a 8ª", com, skillCura, 2500},
		{"Recuperar com a 8ª", com, skillRecuperar, 1800},
		{"Cura sem a 8ª", sem, skillCura, 1100},
		{"outra skill", com, skillFlechaMagica, 1100},
	} {
		if got := tetoDaCura(c.e, c.skill, 1100); got != c.want {
			t.Errorf("%s: teto = %d, want %d", c.nome, got, c.want)
		}
	}
}

// Pelo golpe real: a cura sobe com a vida da FM e para no teto da 8ª.
func TestCuraDaMagiaBrancaNoGolpe(t *testing.T) {
	const cajado = 904
	d := New(Config{CombatRules: regraSemEscala(), ItemEffects: map[int][]content.BaseEffect{
		cajado: {{Eff: efWType, Val: wtypeCajadoUmaMao}},
	}})
	w := world.New(world.Config{GridDim: 16}, slog.Default(), nil, nil)
	cast := castInfo{isSkill: true, special: 100, spell: content.Spell{Index: skillCura, InstanceType: 6, InstanceValue: 100}}
	alvo := &world.Entity{ID: 2, HP: 100, MaxHP: 50_000}

	pouca := fmBranca(10_000, learnedRenascimento, classMasterMortal)
	muita := fmBranca(30_000, learnedRenascimento, classMasterMortal)
	// 2×100 + 100 = 300, mais 10% da vida: 1.300 com 10 mil, 3.300 com 30 mil (teto 2.500).
	if got := d.resolveSkillHit(w, pouca, alvo, alvo.ID, skillCura, cast); got != -1300 {
		t.Errorf("FM de 10 mil de HP: cura = %d, want -1300", got)
	}
	if got := d.resolveSkillHit(w, muita, alvo, alvo.ID, skillCura, cast); got != -2500 {
		t.Errorf("FM de 30 mil de HP: cura = %d, want -2500 (teto da 8ª)", got)
	}
	muita.Equip[weaponSlotR] = world.Item{Index: cajado}
	pouca.Equip[weaponSlotR] = world.Item{Index: cajado}
	if got := d.resolveSkillHit(w, pouca, alvo, alvo.ID, skillCura, cast); got != -1820 {
		t.Errorf("com cajado de 1 mão: cura = %d, want -1820 (1300 × 1,40)", got)
	}
}

func TestJulgamentoDaMagiaBranca(t *testing.T) {
	e := fmBranca(30_000, learnedRenascimento, classMasterMortal)
	e.HP = 20_000
	gasto, dano := custoDoJulgamento(e)
	if gasto != 14_000 || dano != 28_000 {
		t.Errorf("gasto/dano = %d/%d, want 14000/28000", gasto, dano)
	}
	if r := (rolagens{29}); !julgamentoAcerta(&r) {
		t.Error("rolagem 29: want acerto (30%)")
	}
	if r := (rolagens{30}); julgamentoAcerta(&r) {
		t.Error("rolagem 30: want erro")
	}
}

// O Desintoxicar segura debuff novo por 4 s, e só com a 8ª.
func TestImunidadeDoDesintoxicar(t *testing.T) {
	alvo := &world.Entity{ID: 2, Class: 0}
	semOitava := fmBranca(30_000, 1<<6, classMasterMortal)
	marcarImunidadeADebuff(semOitava, alvo, 10_000)
	if imuneADebuff(alvo, 10_500) {
		t.Error("sem a 8ª não há imunidade")
	}
	com := fmBranca(30_000, learnedRenascimento, classMasterMortal)
	marcarImunidadeADebuff(com, alvo, 10_000)
	if !imuneADebuff(alvo, 13_999) {
		t.Error("dentro dos 4 s: want imune")
	}
	if imuneADebuff(alvo, 14_000) {
		t.Error("depois dos 4 s: want sem imunidade")
	}
}

// Pelo caminho do cast: um debuff hostil não pega em quem acabou de tomar
// Desintoxicar.
func TestImunidadeRecusaDebuffNoCast(t *testing.T) {
	d := New(Config{CombatRules: regraSemEscala()})
	clock := new(atomic.Uint32)
	clock.Store(10_000)
	w := world.New(world.Config{GridDim: 16, Now: clock.Load}, slog.Default(), nil, nil)
	atacante := &world.Entity{ID: 1, Class: 0, Level: 100}
	alvo := &world.Entity{ID: 2, Class: 0, Level: 100, RegenMP: 100}
	cast := castInfo{isSkill: true, special: 100, spell: content.Spell{
		Index: 34, InstanceType: 3, InstanceValue: 95, AffectType: 1, AffectValue: 2, AffectTime: 1, Aggressive: 1,
	}}

	d.applyCastAffect(w, atacante, alvo, alvo.ID, cast)
	if !alvo.HasAffect(1) {
		t.Fatal("sem imunidade o debuff tem de pegar")
	}
	alvo.Affect = [len(alvo.Affect)]world.Affect{}
	marcarImunidadeADebuff(fmBranca(30_000, learnedRenascimento, classMasterMortal), alvo, w.Now())
	d.applyCastAffect(w, atacante, alvo, alvo.ID, cast)
	if alvo.HasAffect(1) {
		t.Fatal("com a imunidade o debuff não pode pegar")
	}
}

// Pelo fio, com três jogadores: o Renascimento levanta o alvo clicado sempre e,
// de vez em quando, um morto ao lado.
func TestRenascimentoEmTresNoFio(t *testing.T) {
	db := newDB()
	db.loadResult = world.CharacterState{
		Slot: 0, Name: "Branca", Class: 1, X: 5, Y: 5,
		HP: 20_000, MaxHP: 20_000, MP: 5000, MaxMP: 5000, Level: 100, Con: 1000,
		LearnedSkill: learnedRenascimento,
	}
	spells := content.NewSkillData([]content.Spell{{
		Index: skillRenascimento, TargetType: 1, Range: 10, InstanceType: 8, MaxTarget: 1,
	}})
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	clock := new(atomic.Uint32)
	clock.Store(serverTime)
	d := New(Config{Log: log, Spells: spells})
	w := world.New(world.Config{GridDim: 16, Now: clock.Load}, log, db, d.Handle)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { _ = w.Serve(ctx, ln); close(done) }()
	defer func() { cancel(); <-done }()

	branca := enterWorld(t, ln.Addr().String())
	defer branca.Close()
	morto1 := enterWorld(t, ln.Addr().String())
	defer morto1.Close()
	morto2 := enterWorld(t, ln.Addr().String())
	defer morto2.Close()

	primeiro, segundo := 0, 0
	for i := range 60 {
		clock.Store(serverTime + uint32(i)*1000)
		w.Entity(2).HP, w.Entity(3).HP = 0, 0
		w.Entity(1).MP = 5000
		skillAttackFrame(t, branca, serverTime+uint32(i)*1000, 2, skillRenascimento, -1)
		for {
			ty, _, ok := readMaybe(t, branca)
			if !ok {
				t.Fatal("sem eco do Renascimento")
			}
			if ty == protocol.MsgAttack {
				break
			}
		}
		if w.Entity(2).HP > 0 {
			primeiro++
		}
		if w.Entity(3).HP > 0 {
			segundo++
		}
	}
	if primeiro != 60 {
		t.Errorf("o alvo clicado voltou %d de 60, want sempre", primeiro)
	}
	if segundo == 0 {
		t.Error("o segundo morto nunca voltou em 60 tentativas (10% cada)")
	}
	if segundo > 20 {
		t.Errorf("o segundo morto voltou %d de 60, want perto de 10%%", segundo)
	}
}

// Pelo golpe real: o Julgamento Divino gasta 70% da vida, soma o dobro ao golpe
// e erra a maior parte das vezes.
func TestJulgamentoDivinoNoGolpe(t *testing.T) {
	db := newDB()
	db.loadResult = world.CharacterState{
		Slot: 0, Name: "Branca", Class: 1, X: 5, Y: 5,
		HP: 20_000, MaxHP: 20_000, MP: 20_000, MaxMP: 20_000, Level: 100, Int: 500,
		LearnedSkill: 1<<(skillJulgamento%24) | learnedRenascimento,
	}
	spells := content.NewSkillData([]content.Spell{{
		Index: skillJulgamento, TargetType: 1, Range: 5, InstanceType: 4, InstanceValue: 200, Aggressive: 1, MaxTarget: 1,
	}})
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	clock := new(atomic.Uint32)
	clock.Store(serverTime)
	d := New(Config{Log: log, Spells: spells, CombatRules: regraSemEscala()})
	w := world.New(world.Config{GridDim: 16, Now: clock.Load}, log, db, d.Handle)
	mid := w.SpawnMobAt(world.MobSpawn{Template: plainMobTemplate("Alvo"), X: 6, Y: 5, GenIndex: -1})
	mob := w.Entity(mid)
	mob.HP, mob.MaxHP, mob.Damage, mob.AC = 50_000_000, 50_000_000, 0, 0
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { _ = w.Serve(ctx, ln); close(done) }()
	c := enterWorld(t, ln.Addr().String())
	defer func() { c.Close(); cancel(); <-done }()

	acertos, menorAcerto := 0, 0
	for i := range 40 {
		w.Entity(1).HP = 20_000
		clock.Store(serverTime + uint32(i)*1000)
		skillAttackFrame(t, c, serverTime+uint32(i)*1000, mid, skillJulgamento, -1)
		for {
			ty, p, ok := readMaybe(t, c)
			if !ok {
				t.Fatal("sem eco do Julgamento")
			}
			if ty != protocol.MsgAttack {
				continue
			}
			var body protocol.MsgAttackBody
			if err := body.Decode(p); err != nil {
				t.Fatal(err)
			}
			if len(body.Dam) == 1 && body.Dam[0].Damage > 0 {
				acertos++
				if menorAcerto == 0 || int(body.Dam[0].Damage) < menorAcerto {
					menorAcerto = int(body.Dam[0].Damage)
				}
			}
			break
		}
		if hp := w.Entity(1).HP; hp != 6000 {
			t.Fatalf("vida depois do Julgamento = %d, want 6000 (30%% de 20.000)", hp)
		}
	}
	if acertos == 0 || acertos > 25 {
		t.Errorf("acertos = %d de 40, want perto de 30%%", acertos)
	}
	if menorAcerto < 25_000 {
		t.Errorf("menor golpe que acertou = %d, want acima de 25.000 (o dobro dos 14.000 gastos)", menorAcerto)
	}
}

// O Desintoxicar marca a imunidade pelo caminho do cast.
func TestDesintoxicarMarcaImunidadeNoCast(t *testing.T) {
	d := New(Config{CombatRules: regraSemEscala()})
	clock := new(atomic.Uint32)
	clock.Store(10_000)
	w := world.New(world.Config{GridDim: 16, Now: clock.Load}, slog.Default(), nil, nil)
	s := &world.Session{Conn: 1}
	branca := fmBranca(30_000, learnedRenascimento, classMasterMortal)
	alvo := &world.Entity{ID: 2, Class: 0}
	cast := castInfo{isSkill: true, spell: content.Spell{Index: skillDesintoxicar, InstanceType: 8}}
	dmg := 0

	d.applySkillSpecial(w, s, branca, alvo, alvo.ID, skillDesintoxicar, cast, &protocol.MsgAttackBody{}, &dmg)

	if !imuneADebuff(alvo, w.Now()+1000) {
		t.Fatalf("ImuneDebuffAte = %d, want a janela de 4 s aberta", alvo.ImuneDebuffAte)
	}
}

// O Flash da FM Magia Branca tira o aliado da mira de quem está por perto.
func TestFlashTiraDaMiraNoFio(t *testing.T) {
	db := newDB()
	db.loadResult = world.CharacterState{
		Slot: 0, Name: "Branca", Class: 1, X: 5, Y: 5,
		HP: 20_000, MaxHP: 20_000, MP: 5000, MaxMP: 5000, Level: 100,
		LearnedSkill: 1<<(skillFlash%24) | learnedRenascimento,
	}
	spells := content.NewSkillData([]content.Spell{{
		Index: skillFlash, TargetType: 1, Range: 10, InstanceType: 7, MaxTarget: 1,
	}})
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	clock := new(atomic.Uint32)
	clock.Store(serverTime)
	d := New(Config{Log: log, Spells: spells})
	w := world.New(world.Config{GridDim: 16, Now: clock.Load}, log, db, d.Handle)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { _ = w.Serve(ctx, ln); close(done) }()
	defer func() { cancel(); <-done }()

	branca := enterWorld(t, ln.Addr().String())
	defer branca.Close()
	aliado := enterWorld(t, ln.Addr().String())
	defer aliado.Close()
	inimigo := enterWorld(t, ln.Addr().String())
	defer inimigo.Close()

	w.Entity(3).Target = 2
	skillAttackFrame(t, branca, serverTime, 2, skillFlash, -1)
	for {
		ty, _, ok := readMaybe(t, branca)
		if !ok {
			t.Fatal("sem eco do Flash")
		}
		if ty == protocol.MsgAttack {
			break
		}
	}
	if alvo := w.Entity(3).Target; alvo != 0 {
		t.Fatalf("mira do inimigo = %d, want 0 depois do Flash", alvo)
	}
}
