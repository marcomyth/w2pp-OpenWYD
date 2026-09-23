package handler

import (
	"log/slog"
	"testing"

	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

// mundoComPet monta um mundo com um dono em jogo e uma evocação dele.
func mundoComPet(t *testing.T) (*Dispatcher, *world.World, *world.Entity, *world.Entity) {
	t.Helper()
	log := slog.New(slog.DiscardHandler)
	tigre := summonTemplate("Tigre")
	d := New(Config{Log: log, SummonMobs: [][]byte{tigre}})
	w := world.New(world.Config{GridDim: 64}, log, nil, d.Handle)

	petID := w.SpawnMobAt(world.MobSpawn{Template: tigre, X: 20, Y: 20, RouteType: 5, GenIndex: -1})
	if petID < 0 {
		t.Fatal("não consegui criar a evocação")
	}
	pet := w.Entity(petID)
	pet.Clan = summonClan
	pet.Summoner = 1
	pet.EquipVisual[0] = summonTemplateFace(tigre)

	dono := &world.Entity{ID: 1, Mode: world.MobUser, X: 20, Y: 20, HP: 5000, MaxHP: 5000,
		Class: 2, Level: 399, Special: [4]int16{0, 0, 320, 0}, BaseSpecial: [4]int16{0, 0, 320, 0}}
	return d, w, dono, pet
}

// alvoJogador é um jogador solto, longe de cidade.
func alvoJogador(id int) *world.Entity {
	return &world.Entity{ID: id, Mode: world.MobUser, X: 21, Y: 21, HP: 30_000, MaxHP: 30_000, AC: 2400}
}

// A regra que sustenta tudo: a evocação NÃO escolhe jogador sozinha. Só quem
// entrou na EnemyList dela — por revide — pode ser alvo.
func TestEvocacaoSoMiraJogadorQueEstaNaListaDeInimigos(t *testing.T) {
	_, w, _, pet := mundoComPet(t)
	alvo := alvoJogador(2)

	if validTarget(w, pet, alvo) {
		t.Fatal("sem revide, a evocação NÃO pode mirar um jogador — ela viraria um caçador solto")
	}
	// O caminho POSITIVO (com revide e sessão viva) está em
	// TestEvocacaoAtacaJogadorDepoisDoRevideNoFio: validTarget exige
	// SessionMode == UserPlay, que só existe com o jogador conectado de verdade.
	if naEnemyList(pet, alvo.ID) {
		t.Error("a lista tinha de começar vazia")
	}
	addEnemyList(pet, alvo)
	if !naEnemyList(pet, alvo.ID) {
		t.Error("o revide tinha de entrar na lista")
	}
	if naEnemyList(pet, 3) {
		t.Error("quem não entrou na briga não pode estar na lista")
	}
}

// Cidade continua zona segura, mesmo com o revide feito: o gate da cidade vem
// ANTES do ramo da evocação em validTarget, e é isso que se prova.
func TestEvocacaoNaoSegueJogadorParaDentroDaCidade(t *testing.T) {
	_, w, _, pet := mundoComPet(t)
	alvo := alvoJogador(2)
	addEnemyList(pet, alvo)
	alvo.X, alvo.Y = 2100, 2100
	if world.Village(alvo.X, alvo.Y) < 0 {
		t.Skip("coordenada escolhida não é cidade neste mapa")
	}
	if validTarget(w, pet, alvo) {
		t.Error("dentro da cidade a evocação tem de largar o alvo")
	}
}

// A cadência do golpe em jogador é a da evocação, não a de monstro; e contra
// MONSTRO ela volta a ser a de monstro, porque o PvE está calibrado nela.
func TestCadenciaDaEvocacaoEhMaisLentaEmJogador(t *testing.T) {
	_, _, _, pet := mundoComPet(t)
	jogador := alvoJogador(2)
	monstro := &world.Entity{ID: world.MaxUser + 9, Clan: 1, HP: 100}

	if got := cadenciaDoGolpe(pet, jogador); got != evocacaoCadenciaMs {
		t.Errorf("pet em jogador: cadência %d, want %d", got, evocacaoCadenciaMs)
	}
	if got := cadenciaDoGolpe(pet, monstro); got != mobAttackCadence {
		t.Errorf("pet em monstro: cadência %d, want %d (o PvE está calibrado nela)", got, mobAttackCadence)
	}
	// Monstro comum em jogador não herda a cadência da evocação.
	comum := &world.Entity{ID: world.MaxUser + 8, Clan: 1, HP: 100}
	if got := cadenciaDoGolpe(comum, jogador); got != mobAttackCadence {
		t.Errorf("monstro comum: cadência %d, want %d", got, mobAttackCadence)
	}
}

// Bater na evocação é bater no DONO para ponto de PK, guilda e guerra
// (_MSG_Attack.cpp:1342). A troca de identidade exige o dono EM JOGO: um pet
// cujo dono caiu não pode marcar criminoso ninguém.
//
// A sessão viva é coberta pelo teste de socket
// (TestEvocacaoAtacaJogadorDepoisDoRevideNoFio); aqui ficam os casos em que a
// troca NÃO pode acontecer, que são os perigosos.
func TestDonoDaEvocacaoSoTrocaComDonoEmJogo(t *testing.T) {
	_, w, _, pet := mundoComPet(t)

	// Sem sessão em jogo o dono não responde — um pet órfão não marca PK.
	if got := donoDaEvocacao(w, pet); got != pet {
		t.Error("sem o dono em jogo, o responsável é o próprio bicho")
	}
	// Um monstro comum nunca troca de identidade, nem com Summoner preenchido.
	monstro := &world.Entity{ID: world.MaxUser + 9, Clan: 1, Summoner: 1}
	if got := donoDaEvocacao(w, monstro); got != monstro {
		t.Error("monstro comum não responde por dono nenhum")
	}
	// Um jogador responde por si.
	alvo := alvoJogador(2)
	if got := donoDaEvocacao(w, alvo); got != alvo {
		t.Error("jogador responde por si")
	}
	if got := donoDaEvocacao(w, nil); got != nil {
		t.Error("nil não pode virar dono nenhum")
	}
	// Summoner apontando para um índice de mob, não de jogador: sem troca.
	forjado := &world.Entity{ID: world.MaxUser + 10, Clan: summonClan, Summoner: world.MaxUser + 5}
	if got := donoDaEvocacao(w, forjado); got != forjado {
		t.Error("Summoner que não é jogador não pode virar responsável")
	}
}

// golpeContaComoPvP em jogador: sempre, menos contra si mesmo. (O ramo da
// evocação precisa do dono em jogo e vai no teste de socket.)
func TestGolpeEmJogadorContaComoPvP(t *testing.T) {
	_, w, _, pet := mundoComPet(t)
	alvo := alvoJogador(2)

	if !golpeContaComoPvP(w, 1, 2, alvo) {
		t.Error("bater em jogador conta como PvP")
	}
	if golpeContaComoPvP(w, 2, 2, alvo) {
		t.Error("bater em si mesmo não conta como PvP")
	}
	monstro := &world.Entity{ID: world.MaxUser + 9, Clan: 1, HP: 100}
	if golpeContaComoPvP(w, 1, monstro.ID, monstro) {
		t.Error("monstro comum não conta como PvP")
	}
	// Pet sem dono em jogo: não conta, senão bater num bicho abandonado marcaria
	// PK contra ninguém.
	if golpeContaComoPvP(w, 2, pet.ID, pet) {
		t.Error("evocação sem dono em jogo não conta como PvP")
	}
}
