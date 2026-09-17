package handler

import (
	"bytes"
	"io"
	"log/slog"
	"testing"

	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/protocol"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

// Uma Bruxa azul como a do NPCGener: clã 7, byte 17 em 0 e loja 64 no byte 104,
// que no port a fazia NPC imortal.
func bruxaAzul(hp uint32) []byte {
	b := targetMobWithClan("Bruxa", 7, 64, hp)
	b[17] = 0
	return b
}

// golpeNoReino loga um jogador com a capa capa colado numa Bruxa azul da cidade
// dos Reinos, dá um golpe e devolve o dano do eco e os avisos que chegaram antes.
func golpeNoReino(t *testing.T, capa int16) (int32, [][]byte) {
	t.Helper()
	db := skillCombatDB(1 << 2)
	db.loadResult.X, db.loadResult.Y = 1700, 1680
	db.loadResult.Equip[15] = world.Item{Index: capa}
	addr, stop, _ := startServerSkillsTargetMobAt(t, db, bruxaAzul(5000), 4096, 1701, 1680)
	defer stop()
	c := enterWorld(t, addr)
	defer c.Close()

	skillAttackFrame(t, c, serverTime, world.MaxUser, -1, -2)
	var avisos [][]byte
	for {
		h, payload, ok := readMaybeHeader(t, c)
		if !ok {
			t.Fatal("o eco do golpe não chegou")
		}
		switch h.Type {
		case protocol.MsgMessagePanel:
			avisos = append(avisos, payload)
		case protocol.MsgAttack:
			var body protocol.MsgAttackBody
			if err := body.Decode(payload); err != nil {
				t.Fatal(err)
			}
			return body.Dam[0].Damage, avisos
		}
	}
}

func temAviso(avisos [][]byte, texto string) bool {
	for _, a := range avisos {
		if bytes.HasPrefix(a, protocol.ClientText(texto)) {
			return true
		}
	}
	return false
}

// O exército dos Reinos leva golpe: a Bruxa, com loja no byte do port, zerava
// todo golpe. Quem veste a capa do outro reino fere sem aviso nenhum.
func TestReinosCapaInimigaFereOGuarda(t *testing.T) {
	dmg, avisos := golpeNoReino(t, 3198) // Mestre de Akelonia, vermelha
	if dmg <= 0 {
		t.Fatalf("golpe da capa vermelha na Bruxa azul = %d, want > 0", dmg)
	}
	if len(avisos) != 0 {
		t.Errorf("capa inimiga recebeu %d avisos, want 0", len(avisos))
	}
}

// A capa do próprio reino não fere o reino, e o jogador fica sabendo por quê.
func TestReinosCapaNaoFereOProprioReino(t *testing.T) {
	dmg, avisos := golpeNoReino(t, 3191) // Elite de Hekalotia, azul
	if dmg != 0 {
		t.Errorf("golpe da capa azul na Bruxa azul = %d, want 0", dmg)
	}
	if !temAviso(avisos, msgProprioReino) {
		t.Error("golpe recusado sem aviso")
	}
}

// Sem capa de reino o golpe fere e marca o jogador como inimigo daquele reino.
func TestReinosSemCapaViraInimigo(t *testing.T) {
	dmg, avisos := golpeNoReino(t, 0)
	if dmg <= 0 {
		t.Fatalf("golpe sem capa na Bruxa azul = %d, want > 0", dmg)
	}
	if !temAviso(avisos, msgInimigoDoReino(7)) {
		t.Error("o jogador virou inimigo de Hekalotia sem aviso")
	}
}

func reinosWorld(t *testing.T) (*Dispatcher, *world.World, *world.Entity, *uint32) {
	t.Helper()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	agora := uint32(1_000_000)
	d := New(Config{Log: log})
	w := world.New(world.Config{GridDim: 4096, Now: func() uint32 { return agora }}, log, nil, d.Handle)
	jogador := &world.Entity{ID: 0, Mode: world.MobUser, Name: "Heroi", Level: 1, HP: 100, MaxHP: 100, X: 1700, Y: 1680}
	return d, w, jogador, &agora
}

// A marca é do reino ferido, dura 10 minutos desde o último golpe, avisa só
// quando liga, e a morte a apaga.
func TestReinosMarcaDeInimigo(t *testing.T) {
	d, w, jogador, agora := reinosWorld(t)
	id := w.SpawnMobAt(world.MobSpawn{Template: bruxaAzul(5000), X: 1701, Y: 1680, GenIndex: -1})
	bruxa := w.Entity(id)
	if bruxa == nil || bruxa.NonCombatNPC || !monstroDoReino(bruxa) {
		t.Fatalf("Bruxa azul = %+v, want monstro do Reino", bruxa)
	}

	if !d.golpeNoReinoPermitido(w, jogador, bruxa) {
		t.Fatal("golpe sem capa recusado")
	}
	if !w.InimigoDoReino(jogador, 7) || w.InimigoDoReino(jogador, 8) {
		t.Fatal("a marca tem de ser só de Hekalotia")
	}
	*agora += world.InimigoDoReinoDuracaoMs - 1
	if w.MarcarInimigoDoReino(jogador, 7) {
		t.Error("renovar a marca não é marca nova: avisaria a cada golpe")
	}
	*agora += world.InimigoDoReinoDuracaoMs - 1
	if !w.InimigoDoReino(jogador, 7) {
		t.Error("a marca venceu antes de 10 minutos desde o último golpe")
	}
	*agora += 2
	if w.InimigoDoReino(jogador, 7) {
		t.Error("a marca passou de 10 minutos")
	}

	w.MarcarInimigoDoReino(jogador, 7)
	world.LimparInimigoDoReino(jogador)
	if w.InimigoDoReino(jogador, 7) {
		t.Error("a morte não apagou a marca")
	}

	// O mesmo template fora da cidade dos Reinos continua NPC, e golpe nele não
	// marca ninguém.
	fora := w.Entity(w.SpawnMobAt(world.MobSpawn{Template: bruxaAzul(5000), X: 2600, Y: 1700, GenIndex: -1}))
	if fora == nil || !fora.NonCombatNPC || monstroDoReino(fora) {
		t.Fatalf("Bruxa fora dos Reinos = %+v, want NPC como antes", fora)
	}
	d.golpeNoReinoPermitido(w, jogador, fora)
	if w.InimigoDoReino(jogador, 7) {
		t.Error("golpe fora dos Reinos marcou o jogador")
	}
}

// Monstro do Reino não dá XP; um monstro comum no mesmo tile e o mesmo guarda
// fora da cidade seguem pela regra de sempre. (A tabela de XP por zona zera o
// abate de um personagem de teste ali, então o portão é conferido direto.)
func TestReinosNaoDaXP(t *testing.T) {
	_, w, _, _ := reinosWorld(t)
	spawn := func(clan uint8, x, y int16) *world.Entity {
		return w.Entity(w.SpawnMobAt(world.MobSpawn{Template: expMobTemplate(1, 1000, clan), X: x, Y: y, GenIndex: -1}))
	}
	if reinoAwardsExp(spawn(7, 1701, 1680)) {
		t.Error("guarda azul do Reino dá XP")
	}
	if reinoAwardsExp(spawn(8, 1702, 1766)) {
		t.Error("guarda vermelho do Reino dá XP")
	}
	if !reinoAwardsExp(spawn(1, 1703, 1680)) {
		t.Error("um monstro comum na cidade dos Reinos perdeu o XP")
	}
	if !reinoAwardsExp(spawn(7, 2600, 1700)) {
		t.Error("um monstro de clã 7 fora dos Reinos perdeu o XP")
	}
}
