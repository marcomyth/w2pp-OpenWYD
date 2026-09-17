package handler

import (
	"context"
	"io"
	"log/slog"
	"net"
	"testing"
	"time"

	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/content"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/protocol"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

// tkDaConfianca monta um TK Mortal com a maestria da Confiança.
func tkDaConfianca(dex, intel int16, learned int32, maestria int16) *world.Entity {
	e := &world.Entity{ID: 1, Class: 0, ClassMaster: classMasterMortal, Dex: dex, Int: intel, LearnedSkill: learned}
	e.Special[1] = maestria
	return e
}

func TestTkConfiancaExigeDestino(t *testing.T) {
	tests := []struct {
		name string
		e    *world.Entity
		want bool
	}{
		{"TK com o Destino", tkDaConfianca(1000, 500, learnedDestino, 255), true},
		{"TK sem o Destino", tkDaConfianca(1000, 500, 1<<15|1<<23, 255), false},
		{"Huntress com o bit 7", &world.Entity{ID: 1, Class: 3, LearnedSkill: learnedDestino}, false},
		{"monstro com o bit 7", &world.Entity{ID: world.MaxUser, Class: 0, LearnedSkill: learnedDestino}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tkConfianca(tt.e); got != tt.want {
				t.Fatalf("tkConfianca = %v, want %v", got, tt.want)
			}
		})
	}
}

// armasNasMaos devolve um itemAbility que lê o EF_WTYPE pelo índice do item.
func armasNasMaos(wtypes map[int16]int) func(world.Item, uint8) int {
	return func(it world.Item, ef uint8) int {
		if ef == efWType {
			return wtypes[it.Index]
		}
		return 0
	}
}

func TestArmaDaConfianca(t *testing.T) {
	const cajado, martelo, lanca = 904, 810, 855
	ability := armasNasMaos(map[int16]int{cajado: wtypeCajadoUmaMao, martelo: wtypeMachadoUmaMao, lanca: wtypeLanca})
	tests := []struct {
		name        string
		direita     int16
		esquerda    int16
		classMaster uint8
		want        int
	}{
		{"Arch cajado + martelo", cajado, martelo, classMasterArch, 140},
		{"Arch martelo + cajado", martelo, cajado, classMasterArch, 140},
		{"Mortal cajado + martelo", cajado, martelo, classMasterMortal, 140},
		{"Mortal lança", lanca, 0, classMasterMortal, 140},
		{"Arch lança", lanca, 0, classMasterArch, 100},
		{"Celestial lança", lanca, 0, classMasterCelestial, 100},
		{"cajado sozinho", cajado, 0, classMasterArch, 100},
		{"dois martelos", martelo, martelo, classMasterArch, 100},
		{"sem arma", 0, 0, classMasterArch, 100},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := tkDaConfianca(1500, 500, learnedDestino, 255)
			e.ClassMaster = tt.classMaster
			e.Equip[weaponSlotR] = world.Item{Index: tt.direita}
			e.Equip[weaponSlotL] = world.Item{Index: tt.esquerda}
			if got := armaPctConfianca(e, ability); got != tt.want {
				t.Fatalf("armaPctConfianca = %d, want %d", got, tt.want)
			}
		})
	}
	if got := armaPctConfianca(tkDaConfianca(1500, 500, learnedDestino, 255), nil); got != 100 {
		t.Fatalf("sem catálogo = %d, want 100", got)
	}
}

// O Destino dá INT e CON pela maestria, e a esquiva sai da régua de Destreza já
// com a INT do Destino somada.
func TestPassivasDaConfianca(t *testing.T) {
	tests := []struct {
		name                      string
		learned                   int32
		maestria                  int16
		wantInt, wantCon, wantEsq int32
	}{
		// INT 500+300 → d = 500 + 1000×700/2300 = 804 → esquiva 60×0,804 = 48.
		{"maestria cheia", learnedDestino, 255, 300, 500, 48},
		// INT 500+149 → d = 500 + 1000×851/2149 = 895 → 53.
		{"meia maestria", learnedDestino, 127, 149, 249, 53},
		{"sem o Destino", 1 << 15, 255, 0, 0, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := tkDaConfianca(1500, 500, tt.learned, tt.maestria)
			applyAffectScore(e)
			if int32(e.AffInt) != tt.wantInt || int32(e.AffCon) != tt.wantCon {
				t.Fatalf("AffInt/AffCon = %d/%d, want %d/%d", e.AffInt, e.AffCon, tt.wantInt, tt.wantCon)
			}
			if e.AffMaxHP != 2*tt.wantCon || e.AffMaxMP != 2*tt.wantInt {
				t.Fatalf("AffMaxHP/AffMaxMP = %d/%d, want %d/%d", e.AffMaxHP, e.AffMaxMP, 2*tt.wantCon, 2*tt.wantInt)
			}
			if e.AffEsquivaPct != tt.wantEsq {
				t.Fatalf("AffEsquivaPct = %d, want %d", e.AffEsquivaPct, tt.wantEsq)
			}
		})
	}
}

// A esquiva da Confiança chega ao sorteio de esquiva, como a da Captura.
func TestEsquivaDaConfiancaNoParry(t *testing.T) {
	e := tkDaConfianca(1500, 500, learnedDestino, 255)
	applyAffectScore(e)
	if got, want := esquivaComMelhoria(300, e), 300*148/100; got != want {
		t.Fatalf("esquivaComMelhoria = %d, want %d", got, want)
	}
}

// A Destreza do TK Confiança não dá dano físico; a do TK comum continua dando.
func TestDestrezaDaConfiancaSemDanoFisico(t *testing.T) {
	comum := tkDaConfianca(900, 0, 1<<15, 0)
	comum.Str = 1000
	confianca := tkDaConfianca(900, 0, learnedDestino, 0)
	confianca.Str = 1000
	if got := attributeDamageBonus(comum, false); got != 1000/2+900/3 {
		t.Fatalf("TK comum = %d, want %d", got, 1000/2+900/3)
	}
	if got := attributeDamageBonus(confianca, false); got != 1000/2 {
		t.Fatalf("TK Confiança = %d, want %d (sem a DES)", got, 1000/2)
	}
}

func TestPerfuracaoDaConfianca(t *testing.T) {
	destrezaPura := tkDaConfianca(1500, 0, learnedDestino, 255)
	meioAMeio := tkDaConfianca(1000, 1000, learnedDestino, 255)
	semDestino := tkDaConfianca(1500, 0, 1<<15, 255)
	tests := []struct {
		name  string
		e     *world.Entity
		skill int
		want  int
	}{
		{"Fanatismo, DES pura", destrezaPura, skillFanatismo, 750},
		{"Destino, DES pura", destrezaPura, skillDestino, 750},
		{"Fanatismo, meio a meio", meioAMeio, skillFanatismo, 875},
		{"Giro da Fúria não perfura", destrezaPura, skillGiroDaFuria, 1000},
		{"Golpe Duplo não perfura", destrezaPura, skillGolpeDuplo, 1000},
		{"TK sem o Destino", semDestino, skillFanatismo, 1000},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := defesaPerfuradaConfianca(tt.e, tt.skill, 1000); got != tt.want {
				t.Fatalf("defesaPerfuradaConfianca = %d, want %d", got, tt.want)
			}
		})
	}
}

// O Fanatismo tira defesa do jogador e do monstro, e usar de novo renova em vez de
// acumular.
func TestDebuffDoFanatismo(t *testing.T) {
	d := New(Config{})
	w := world.New(world.Config{GridDim: 16}, slog.Default(), nil, nil)
	tk := tkDaConfianca(1500, 500, learnedDestino, 255)

	alvo := &world.Entity{ID: 2, Class: 3, BaseAC: 1000}
	d.refreshScore(alvo)
	antes := effectiveAC(alvo)
	for range 2 {
		d.aplicarDebuffDoFanatismo(w, tk, alvo, alvo.ID)
	}
	slots := 0
	for _, af := range alvo.Affect {
		if af.Type == affectDefesaPct {
			slots++
			if af.Value != 15 || af.Time != 2 {
				t.Fatalf("afeto = %+v, want Value 15 e Time 2", af)
			}
		}
	}
	if slots != 1 {
		t.Fatalf("%d slots do debuff, want 1 (renova, não acumula)", slots)
	}
	if got, want := effectiveAC(alvo), antes*85/100; got != want {
		t.Fatalf("defesa com o debuff = %d, want %d (de %d)", got, want, antes)
	}

	mob := &world.Entity{ID: world.MaxUser + 1, HP: 1000, MaxHP: 1000}
	d.aplicarDebuffDoFanatismo(w, tk, mob, mob.ID)
	if !mob.HasAnyAffect() {
		t.Fatal("o Fanatismo não pegou no monstro")
	}

	semMaestria := tkDaConfianca(1500, 500, learnedDestino, 0)
	limpo := &world.Entity{ID: 3, Class: 3}
	d.aplicarDebuffDoFanatismo(w, semMaestria, limpo, limpo.ID)
	if limpo.HasAnyAffect() {
		t.Fatal("maestria zero aplicou debuff")
	}
}

func TestCuraDaAuraConfianca(t *testing.T) {
	const agora = 50_000
	tests := []struct {
		name      string
		level     int
		hpAddPct  int32
		ultimoPvP uint32
		want      int32
	}{
		// HP bruto: 2×5000 (a dobra do score) + 2×500 da CON do Destino = 11.000.
		{"fora de PvP", 255, 0, 0, 1650},
		{"em PvP há 5 s", 255, 0, agora - 5_000, 550},
		{"PvP há 11 s já passou", 255, 0, agora - 11_000, 1650},
		{"meia maestria", 127, 0, 0, 821},
		{"EF_HPADD 10%", 255, 10, 0, 1815},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := tkDaConfianca(1500, 500, learnedDestino, 255)
			e.MaxHP = 5000
			e.HpAddPct = tt.hpAddPct
			e.UltimoPvP = tt.ultimoPvP
			// Um buff temporário de HP não entra no HP bruto.
			e.AffMaxHP = 99_999
			if got := curaDaAuraConfianca(e, tt.level, agora); got != tt.want {
				t.Fatalf("curaDaAuraConfianca = %d, want %d", got, tt.want)
			}
		})
	}
}

// Pelo fio: o TK Confiança com a Aura ativa cura a porcentagem do HP, não o
// Level/2 + Value do legado.
func TestAuraDaConfiancaCuraPeloFio(t *testing.T) {
	db := newDB()
	db.loadResult = world.CharacterState{
		Slot: 0, Name: "Paladino", X: 5, Y: 5, Class: 0,
		HP: 100, MaxHP: 20_000, MP: 100, MaxMP: 100, Level: 100,
		LearnedSkill: learnedDestino, BaseSpecial: [4]int16{0, 255, 0, 0},
		Affects: []world.Affect{{Type: affectAuraDaVida, Value: 75, Level: 255, Time: 1000}},
	}
	addr, stop := startServerAffectTick(t, db)
	defer stop()
	c := enterWorld(t, addr)
	defer c.Close()

	const legado = 255/2 + 75
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		ty, p, ok := readMaybe(t, c)
		if !ok || ty != protocol.MsgSetHpDam {
			continue
		}
		if dam := int32(lend.Uint32(p[4:])); dam <= 2*legado {
			t.Fatalf("cura = %d, want a porcentagem do HP (bem acima dos %d do legado)", dam, legado)
		}
		return
	}
	t.Fatal("a Aura da Confiança nunca curou")
}

func TestEmPvP(t *testing.T) {
	e := &world.Entity{}
	if emPvP(e, 5_000) {
		t.Fatal("nunca bateu em jogador e está em PvP")
	}
	a, b := &world.Entity{}, &world.Entity{}
	marcarPvP(a, b, 20_000)
	if !emPvP(a, 29_999) || !emPvP(b, 29_999) {
		t.Fatal("os dois lados deviam estar em PvP por 10 s")
	}
	if emPvP(a, 30_000) {
		t.Fatal("o PvP devia acabar em 10 s")
	}
}

// Pelo caminho real do ataque: o Fanatismo que acerta um jogador deixa o debuff
// de defesa nele, e os dois lados ficam marcados em PvP para a Aura.
func TestFanatismoPeloAtaque(t *testing.T) {
	db := newDB()
	db.loadResult = world.CharacterState{
		Slot: 0, Name: "Paladino", Class: 0, X: 5, Y: 5,
		HP: 50_000, MaxHP: 50_000, MP: 5000, MaxMP: 5000, Level: 100,
		Dex: 1500, Int: 500, Damage: 200,
		LearnedSkill: 1<<skillFanatismo | learnedDestino, BaseSpecial: [4]int16{0, 255, 0, 0},
	}
	spells := content.NewSkillData([]content.Spell{{
		Index: skillFanatismo, ManaSpent: 30, TargetType: 1, Range: 5,
		InstanceType: 4, InstanceValue: 75, Aggressive: 1, MaxTarget: 1, Name: "Fanatismo",
	}})
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	d := New(Config{Log: log, Spells: spells})
	w := world.New(world.Config{GridDim: 16, Now: relogioEmServerTime()}, log, db, d.Handle)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { _ = w.Serve(ctx, ln); close(done) }()
	defer func() { cancel(); <-done }()

	attacker := enterWorld(t, ln.Addr().String())
	defer attacker.Close()
	target := enterWorld(t, ln.Addr().String())
	defer target.Close()
	send(t, attacker, protocol.MsgPKMode, protocol.EncodeStandardParm(1))

	acertou := false
	for i := range 20 {
		skillAttackFrame(t, attacker, serverTime+uint32(i)*1000, 2, skillFanatismo, -1)
		for {
			ty, p, ok := readMaybe(t, attacker)
			if !ok {
				t.Fatal("sem eco do ataque")
			}
			if ty != protocol.MsgAttack {
				continue
			}
			var body protocol.MsgAttackBody
			if err := body.Decode(p); err != nil {
				t.Fatal(err)
			}
			acertou = len(body.Dam) == 1 && body.Dam[0].Damage > 0
			break
		}
		if acertou {
			break
		}
	}
	if !acertou {
		t.Fatal("o Fanatismo não acertou em 20 tentativas")
	}
	alvo, tk := w.Entity(2), w.Entity(1)
	debuff := false
	for _, af := range alvo.Affect {
		if af.Type == affectDefesaPct && af.Value == fanatismoDefesaPct {
			debuff = true
		}
	}
	if !debuff {
		t.Fatalf("afetos do alvo = %+v, want o debuff de defesa do Fanatismo", alvo.Affect)
	}
	if tk.UltimoPvP == 0 || alvo.UltimoPvP == 0 {
		t.Fatalf("UltimoPvP = %d/%d, want os dois marcados", tk.UltimoPvP, alvo.UltimoPvP)
	}
}

// O golpe real do Fanatismo sai da régua da Confiança, e não da Magia: com Magia
// zero a fórmula do legado daria menos de 800.
func TestFanatismoBatePelaDestreza(t *testing.T) {
	d := New(Config{CombatRules: regraSemEscala()})
	w := world.New(world.Config{GridDim: 16}, slog.Default(), nil, nil)
	cast := castInfo{isSkill: true, special: 255, spell: content.Spell{Index: skillFanatismo, InstanceType: 4, InstanceValue: 75}}
	// (3×(0,6×1500 + 0,4×800) + 255 + 75) × 5/4 × 1,15 = 5735, sorteio de 90% a 110%.
	const lo, hi = 5735 * 90 / 100, 5735 * 110 / 100
	for range 50 {
		tk := tkDaConfianca(1500, 800, learnedDestino, 255)
		mob := &world.Entity{ID: world.MaxUser + 1, HP: 100_000, MaxHP: 100_000}
		if got := d.resolveSkillHit(w, tk, mob, mob.ID, skillFanatismo, cast); got < lo || got > hi {
			t.Fatalf("Fanatismo = %d, want %d-%d", got, lo, hi)
		}
	}
}

// Contra defesa alta, o Fanatismo do TK de Destreza perfura: a d = 804 ignora
// 20,1% dos 8.000 de defesa, e o golpe sai de (5735 − 6392/2) × 90%-110%. Sem a
// perfuração seria (5735 − 4000) × 90%-110%, de 1561 a 1908.
func TestFanatismoPerfuraNoGolpe(t *testing.T) {
	d := New(Config{CombatRules: regraSemEscala()})
	w := world.New(world.Config{GridDim: 16}, slog.Default(), nil, nil)
	cast := castInfo{isSkill: true, special: 255, spell: content.Spell{Index: skillFanatismo, InstanceType: 4, InstanceValue: 75}}
	const lo, hi = (5735 - 3196) * 90 / 100, (5735 - 3196) * 110 / 100
	for range 50 {
		tk := tkDaConfianca(1500, 800, learnedDestino, 255)
		mob := &world.Entity{ID: world.MaxUser + 1, HP: 100_000, MaxHP: 100_000, AC: 8000}
		if got := d.resolveSkillHit(w, tk, mob, mob.ID, skillFanatismo, cast); got < lo || got > hi {
			t.Fatalf("Fanatismo contra 8.000 de defesa = %d, want %d-%d", got, lo, hi)
		}
	}
}
