package handler

import (
	"log/slog"
	"testing"

	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/protocol"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

// xamaTrollTemplate é o Xamã Troll como o conteúdo o traz: Merchant 100 e grau 41
// no rosto.
func xamaTrollTemplate() []byte {
	b := expMobTemplate(80, 0, 6)
	b[17] = 100    // MOB.Merchant
	b[92+12] = 100 // CurrentScore.Merchant, o byte que o mundo lê
	b[140] = 212   // Equip[0].sIndex, byte baixo
	// Equip[0].Effects[0] = EF_GRADE0 41
	b[142], b[143] = 100, gradeAcampamentoTroll
	return b
}

// acampamentoTrollFixture é o acampamento como produção o monta: os doze blocos da
// quest, na mesma ordem (boss, seguidores, dois blocos de dois guardiões, oito de tropa).
func acampamentoTrollFixture(t *testing.T) (*Dispatcher, *world.World, *world.Session, *world.Entity) {
	t.Helper()
	log := slog.New(slog.DiscardHandler)
	d := New(Config{Log: log, AcampamentoTrollNPC: xamaTrollTemplate()})
	w := world.New(world.Config{}, log, nil, d.Handle) // grid padrão de 4096: o acampamento fica perto de (2650,1985)
	first := world.AcampamentoTrollGenFirst
	gens := make([]*world.Generator, world.AcampamentoTrollGenLast+1)
	gens[first] = casteloOrcBlock("ATroll_Enigma", 2651, 1983, 1, 0)
	gens[first+1] = casteloOrcBlock("ATroll_Mago", 2660, 1985, 4, 3)
	gens[first+2] = casteloOrcBlock("ATroll_Caos", 2658, 1973, 2, 1)
	gens[first+3] = casteloOrcBlock("ATroll_Caos", 2662, 1994, 2, 1)
	for gen := first + 4; gen <= world.AcampamentoTrollGenLast; gen++ {
		gens[gen] = casteloOrcBlock("ATroll_Tropa", int16(2644+2*(gen-first)), 1985, 5, 4)
	}
	w.RegisterGenerators(gens)
	sp := acampamentoTrollSpec
	e := &world.Entity{
		ID: 0, Mode: world.MobUser, Name: "Lider",
		Level: 330, MaxHP: 1000, HP: 1000, X: sp.saida[0], Y: sp.saida[1],
	}
	return d, w, &world.Session{Conn: 0, Mode: world.UserPlay}, e
}

func raiseXamaTroll(t *testing.T, d *Dispatcher, w *world.World) *world.Entity {
	t.Helper()
	d.garantirNPCCorrida(w, &d.acampamentoTroll)
	npc := w.Entity(d.acampamentoTroll.npcID)
	if npc == nil {
		t.Fatal("o Xamã Troll não nasceu")
	}
	return npc
}

func abrirAcampamento(t *testing.T) (*Dispatcher, *world.World, *world.Session, *world.Entity) {
	t.Helper()
	d, w, s, e := acampamentoTrollFixture(t)
	e.Carry[0] = world.Item{Index: itemChaveCasteloOrc}
	d.corridaNPC(w, &d.acampamentoTroll, s, e, raiseXamaTroll(t, d, w))
	if !d.acampamentoTroll.estado.active {
		t.Fatal("a chave não abriu o acampamento")
	}
	return d, w, s, e
}

// A entrada fica dentro da caixa e o NPC e a saída fora dela: com o NPC dentro,
// quem fosse entregar a chave durante uma corrida seria barrado, e a saída dentro
// devolveria o varrido para a própria caixa.
func TestAcampamentoTrollGeometria(t *testing.T) {
	sp := acampamentoTrollSpec
	if !sp.caixa.contains(sp.entrada[0], sp.entrada[1]) {
		t.Errorf("a entrada %v fica fora da caixa %v", sp.entrada, sp.caixa)
	}
	if sp.caixa.contains(sp.npc[0], sp.npc[1]) || sp.caixa.contains(sp.saida[0], sp.saida[1]) {
		t.Errorf("o NPC %v ou a saída %v ficam dentro da caixa %v", sp.npc, sp.saida, sp.caixa)
	}
	if sp.genBoss != world.AcampamentoTrollGenFirst || sp.genSeguidor != world.AcampamentoTrollGenFirst+1 {
		t.Errorf("boss %d e seguidores %d fora da ordem dos blocos", sp.genBoss, sp.genSeguidor)
	}
	if sp.caixa.contains(casteloOrcEntry[0], casteloOrcEntry[1]) || casteloOrcBox.contains(sp.entrada[0], sp.entrada[1]) {
		t.Error("o acampamento e o Castelo Orc se sobrepõem")
	}
}

// O Xamã Troll fica de pé como NPC de quest protegido — um só, por mais que o
// guardião o procure.
func TestAcampamentoTrollXamaNasceUmaVez(t *testing.T) {
	d, w, _, _ := acampamentoTrollFixture(t)
	npc := raiseXamaTroll(t, d, w)
	if npc.Merchant != 100 || npc.Grade != gradeAcampamentoTroll || !npc.NonCombatNPC {
		t.Errorf("Xamã Troll: merchant %d grau %d não-combate %v, want 100/41/true", npc.Merchant, npc.Grade, npc.NonCombatNPC)
	}
	first := d.acampamentoTroll.npcID
	d.garantirNPCCorrida(w, &d.acampamentoTroll)
	if d.acampamentoTroll.npcID != first {
		t.Errorf("o Xamã nasceu de novo (%d → %d) sem ter sumido", first, d.acampamentoTroll.npcID)
	}
	w.DespawnMob(first, 0)
	d.garantirNPCCorrida(w, &d.acampamentoTroll)
	if e := w.Entity(d.acampamentoTroll.npcID); e == nil || e.Mode == world.MobEmpty {
		t.Error("o Xamã sumiu e não voltou")
	}
}

func TestAcampamentoTrollSemChaveNaoAbre(t *testing.T) {
	d, w, s, e := acampamentoTrollFixture(t)
	// A antiga Chave dos Trolls voltou a ser o Cupom da Sorte e não abre nada.
	e.Carry[0] = world.Item{Index: 3223}
	d.corridaNPC(w, &d.acampamentoTroll, s, e, raiseXamaTroll(t, d, w))
	if d.acampamentoTroll.estado.active {
		t.Fatal("abriu sem a Chave do Rei Orc")
	}
	if e.Carry[0].Index != 3223 {
		t.Error("o Cupom da Sorte foi gasto no Xamã Troll")
	}
	if live(w, acampamentoTrollSpec.genBoss) != 0 {
		t.Error("o boss nasceu sem a corrida")
	}
}

// A chave abre o acampamento por 15 minutos e levanta tudo menos o boss: os
// quatro seguidores, os quatro guardiões e cada bloco de tropa cheio. O Enigma
// espera os 100 abates.
func TestAcampamentoTrollChaveAbreOAcampamento(t *testing.T) {
	d, w, s, e := acampamentoTrollFixture(t)
	e.Carry[3] = world.Item{Index: itemChaveCasteloOrc}
	d.corridaNPC(w, &d.acampamentoTroll, s, e, raiseXamaTroll(t, d, w))

	r := d.acampamentoTroll.estado
	if !r.active || r.secondsLeft != 15*60 || r.leaderName != "Lider" {
		t.Fatalf("corrida = %+v, want ativa com 900 s", r)
	}
	if e.Carry[3].Index != 0 {
		t.Error("a chave não foi consumida")
	}
	first := world.AcampamentoTrollGenFirst
	if live(w, first) != 0 || live(w, first+1) != 4 || live(w, first+2) != 2 || live(w, first+3) != 2 {
		t.Errorf("boss %d, seguidores %d, guardiões %d e %d; want 0, 4, 2 e 2",
			live(w, first), live(w, first+1), live(w, first+2), live(w, first+3))
	}
	for gen := first + 4; gen <= world.AcampamentoTrollGenLast; gen++ {
		if live(w, gen) != 5 {
			t.Errorf("bloco de tropa %d com %d mobs, want 5", gen, live(w, gen))
		}
	}
	if d.casteloOrc.active {
		t.Error("abrir o acampamento abriu o Castelo Orc")
	}
}

// Um grupo por vez: um segundo líder ouve que espere e fica com a chave; um membro
// não abre pelo grupo.
func TestAcampamentoTrollUmGrupoPorVez(t *testing.T) {
	d, w, _, _ := abrirAcampamento(t)
	outro := &world.Entity{ID: 7, Mode: world.MobUser, Name: "Outro", HP: 1000}
	outro.Carry[0] = world.Item{Index: itemChaveCasteloOrc}
	d.corridaNPC(w, &d.acampamentoTroll, &world.Session{Conn: 7, Mode: world.UserPlay}, outro, w.Entity(d.acampamentoTroll.npcID))
	if outro.Carry[0].Index != itemChaveCasteloOrc || d.acampamentoTroll.estado.leaderName != "Lider" {
		t.Error("um segundo grupo entrou ou perdeu a chave com o acampamento ocupado")
	}

	d2, w2, s2, membro := acampamentoTrollFixture(t)
	membro.Leader = 5
	membro.Carry[0] = world.Item{Index: itemChaveCasteloOrc}
	d2.corridaNPC(w2, &d2.acampamentoTroll, s2, membro, raiseXamaTroll(t, d2, w2))
	if d2.acampamentoTroll.estado.active || membro.Carry[0].Index != itemChaveCasteloOrc {
		t.Error("um membro de grupo abriu o acampamento")
	}
}

// Durante a corrida só o grupo anda para dentro; sem corrida, e fora da caixa,
// nada é barrado.
func TestAcampamentoTrollSoOGrupoEntra(t *testing.T) {
	d, _, _, _ := acampamentoTrollFixture(t)
	c := &d.acampamentoTroll
	dentro, fora := acampamentoTrollSpec.entrada, acampamentoTrollSpec.saida
	if !c.movimentoPermitido(7, dentro[0], dentro[1]) {
		t.Error("sem corrida, o acampamento recusou alguém")
	}
	c.estado = corridaEstado{active: true, secondsLeft: 100, party: []int{0, 3}}
	if !c.movimentoPermitido(3, dentro[0], dentro[1]) {
		t.Error("um membro do grupo foi barrado")
	}
	if c.movimentoPermitido(7, dentro[0], dentro[1]) {
		t.Error("um estranho entrou no acampamento durante a corrida")
	}
	if !c.movimentoPermitido(7, fora[0], fora[1]) {
		t.Error("um estranho foi barrado fora do acampamento")
	}
	if !d.casteloOrcMoveAllowed(7, casteloOrcEntry[0], casteloOrcEntry[1]) {
		t.Error("a corrida do acampamento fechou o Castelo Orc")
	}
}

// derrubarTrolls conta n abates de tropa da quest, como o mobKilled os entrega.
func derrubarTrolls(d *Dispatcher, w *world.World, c *corrida, n int) {
	tropa := &world.Entity{GenIndex: int16(world.AcampamentoTrollGenLast)}
	for range n {
		d.corridaMobMorto(w, c, tropa)
	}
}

func bossDoAcampamento(w *world.World, c *corrida) *world.Entity {
	var boss *world.Entity
	w.ForEachMob(func(_ int, m *world.Entity) {
		if int(m.GenIndex) == c.spec.genBoss {
			boss = m
		}
	})
	return boss
}

// O Enigma nasce no centro no 100º abate, um só; um monstro de fora da quest não
// conta. Morto, ele encerra a corrida e leva os monstros da quest junto.
func TestAcampamentoTrollEnigmaDepoisDe100Abates(t *testing.T) {
	d, w, _, _ := abrirAcampamento(t)
	c := &d.acampamentoTroll
	if c.spec.bossAposAbates != 100 {
		t.Fatalf("o Enigma nasce aos %d abates, want 100", c.spec.bossAposAbates)
	}
	for range 50 {
		d.corridaMobMorto(w, c, &world.Entity{GenIndex: 3804}) // o Troll do mundo aberto
	}
	derrubarTrolls(d, w, c, 99)
	if live(w, c.spec.genBoss) != 0 || c.estado.abates != 99 {
		t.Fatalf("com 99 abates: boss %d, contagem %d; want 0 e 99", live(w, c.spec.genBoss), c.estado.abates)
	}
	derrubarTrolls(d, w, c, 1)
	boss := bossDoAcampamento(w, c)
	if boss == nil || live(w, c.spec.genBoss) != 1 {
		t.Fatal("o Enigma não nasceu no 100º abate")
	}
	if dx, dy := boss.X-c.spec.entrada[0], boss.Y-c.spec.entrada[1]; dx < -2 || dx > 2 || dy < -2 || dy > 2 {
		t.Errorf("o Enigma nasceu em (%d,%d), longe do centro %v", boss.X, boss.Y, c.spec.entrada)
	}
	derrubarTrolls(d, w, c, 150)
	if live(w, c.spec.genBoss) != 1 {
		t.Errorf("mais abates levantaram %d Enigmas, want 1", live(w, c.spec.genBoss))
	}

	d.corridaMobMorto(w, c, boss)
	if !c.estado.bossDown || c.estado.secondsLeft > 10 {
		t.Fatalf("depois do Enigma: %+v, want o fim em segundos", c.estado)
	}
	for range c.spec.saque {
		d.tickCorrida(w, c)
	}
	if c.estado.active {
		t.Fatal("o Enigma caiu e a corrida continuou")
	}
	for gen := world.AcampamentoTrollGenFirst; gen <= world.AcampamentoTrollGenLast; gen++ {
		if live(w, gen) != 0 {
			t.Errorf("bloco %d ficou com %d mobs depois do fim", gen, live(w, gen))
		}
	}
}

// O relógio no fim encerra a corrida mesmo sem o Enigma.
func TestAcampamentoTrollTempoAcaba(t *testing.T) {
	d, w, _, _ := abrirAcampamento(t)
	c := &d.acampamentoTroll
	c.estado.secondsLeft = 1
	d.tickCorrida(w, c)
	if c.estado.active {
		t.Fatal("o relógio zerou e a corrida continuou")
	}
}

// Um guardião não conta como boss: soma um abate e não corta o relógio.
func TestAcampamentoTrollSoOBossCortaORelogio(t *testing.T) {
	d, w, _, _ := abrirAcampamento(t)
	c := &d.acampamentoTroll
	guardiao := &world.Entity{GenIndex: int16(world.AcampamentoTrollGenFirst + 2)}
	d.corridaMobMorto(w, c, guardiao)
	if c.estado.bossDown || c.estado.secondsLeft != 15*60 || c.estado.abates != 1 {
		t.Errorf("um guardião cortou o relógio ou não contou: %+v", c.estado)
	}
}

// Os seguidores voltam a cada meio minuto enquanto a corrida está aberta.
func TestAcampamentoTrollSeguidoresRenascem(t *testing.T) {
	d, w, _, _ := abrirAcampamento(t)
	c := &d.acampamentoTroll
	w.ClearGenerator(c.spec.genSeguidor) // o grupo matou todos
	c.estado.sinceFollower = c.spec.seguidorCada - 1
	d.tickCorrida(w, c)
	if live(w, c.spec.genSeguidor) == 0 {
		t.Error("os seguidores não renasceram")
	}
}

// Um grupo que saiu do acampamento não o segura: um minuto sem ninguém dentro
// encerra a corrida.
func TestAcampamentoTrollAbandonoLiberaOAcampamento(t *testing.T) {
	d, w, _, _ := abrirAcampamento(t)
	c := &d.acampamentoTroll
	for range c.spec.abandono {
		d.tickCorrida(w, c)
	}
	if c.estado.active {
		t.Error("o acampamento ficou preso com o grupo fora")
	}
}

// O relógio sai como o da Água e o do Orc, e volta a cada minuto para quem relogou
// ou entrou de novo.
func TestAcampamentoTrollContadorComoOdaAgua(t *testing.T) {
	d, w, s, _ := acampamentoTrollFixture(t)
	d.acampamentoTroll.estado.secondsLeft = 15 * 60
	d.enviarRelogioCorrida(w, &d.acampamentoTroll, s)
	if n := w.SentOfType(s, protocol.MsgStartTime); n != 1 {
		t.Errorf("%d MsgStartTime, want 1", n)
	}
	for _, tc := range []struct {
		left int
		due  bool
	}{{840, true}, {839, false}, {60, true}, {59, false}, {0, false}} {
		if got := corridaReenvioDevido(tc.left); got != tc.due {
			t.Errorf("reenvio aos %d s = %v, want %v", tc.left, got, tc.due)
		}
	}
}
