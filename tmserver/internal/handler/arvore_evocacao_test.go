package handler

import (
	"encoding/binary"
	"log/slog"
	"testing"

	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

// Na defesa de referência a tabela vale ao pé da letra — é o contrato que deixa
// quem opera ler 650 e saber que são 650.
func TestEvocacaoEntregaATabelaNaDefesaDeReferencia(t *testing.T) {
	for criatura, quer := range evocacaoDano {
		if quer == 0 {
			continue
		}
		got := danoDaEvocacaoEmJogador(criatura, evocacaoMaestriaCheia, evocacaoDefesaRef)
		if got != quer {
			t.Errorf("criatura %d: dano = %d, want %d (a tabela é o número na defesa de referência)", criatura, got, quer)
		}
	}
}

// A razão de existir da regra: contra a subtração do servidor o mesmo Tigre
// varia 6,6× entre um mago e um TransKnight. Pela curva tem de variar pouco.
func TestEvocacaoNaoDesabaContraAlvoBlindado(t *testing.T) {
	const tigre = 4
	fraco := danoDaEvocacaoEmJogador(tigre, evocacaoMaestriaCheia, 1500)
	medio := danoDaEvocacaoEmJogador(tigre, evocacaoMaestriaCheia, 2200)
	forte := danoDaEvocacaoEmJogador(tigre, evocacaoMaestriaCheia, 3000)
	if !(fraco > medio && medio > forte) {
		t.Fatalf("a defesa tem de pesar: %d / %d / %d", fraco, medio, forte)
	}
	// Teto de 2× entre as pontas. Com a subtração isto dava 6,6×, que é o que a
	// regra existe para não deixar acontecer.
	if raz := float64(fraco) / float64(forte); raz > 2 {
		t.Errorf("variação entre alvo fraco e blindado = %.1f×, quero no máximo 2× (%d contra %d)", raz, fraco, forte)
	}
	// E contra alvo sem armadura nenhuma o teto é o dobro da tabela, não o infinito.
	if nu := danoDaEvocacaoEmJogador(tigre, evocacaoMaestriaCheia, 0); nu != evocacaoDano[tigre]*2 {
		t.Errorf("contra defesa 0 o dano = %d, want %d (o dobro da tabela)", nu, evocacaoDano[tigre]*2)
	}
}

// A força anda com a maestria e para no teto, como o número de cabeças.
func TestEvocacaoEscalaPelaMaestriaEParaNoTeto(t *testing.T) {
	const tigre = 4
	cheio := danoDaEvocacaoEmJogador(tigre, evocacaoMaestriaCheia, evocacaoDefesaRef)
	metade := danoDaEvocacaoEmJogador(tigre, evocacaoMaestriaCheia/2, evocacaoDefesaRef)
	if metade != cheio/2 {
		t.Errorf("metade da Evocação = %d, want %d", metade, cheio/2)
	}
	if alem := danoDaEvocacaoEmJogador(tigre, evocacaoMaestriaCheia*2, evocacaoDefesaRef); alem != cheio {
		t.Errorf("acima do teto = %d, want %d (a maestria para em %d)", alem, cheio, evocacaoMaestriaCheia)
	}
	if zero := danoDaEvocacaoEmJogador(tigre, 0, evocacaoDefesaRef); zero != 1 {
		t.Errorf("sem Evocação = %d, want o piso 1", zero)
	}
}

// A Invocação Final não tem regra própria, como não tem escalonamento em
// summonBonus. Fora da tabela, nada.
func TestEvocacaoSemRegraForaDaTabela(t *testing.T) {
	for _, criatura := range []int{-1, 8, 9, 100} {
		if got := danoDaEvocacaoEmJogador(criatura, evocacaoMaestriaCheia, evocacaoDefesaRef); got != 0 {
			t.Errorf("criatura %d: dano = %d, want 0", criatura, got)
		}
	}
}

// evocacaoDoPet tem de achar a criatura pela FACE, que é o único vestígio do
// template que o pet carrega depois de nascer.
func TestEvocacaoDoPetSaiDaFace(t *testing.T) {
	condor, tigre := summonTemplate("Condor"), summonTemplate("Tigre")
	binary.LittleEndian.PutUint16(tigre[140:], 244)
	d := New(Config{Log: slog.New(slog.DiscardHandler), SummonMobs: [][]byte{condor, tigre}})

	pet := &world.Entity{ID: world.MaxUser + 1, Clan: summonClan, Summoner: 3}
	pet.EquipVisual[0] = 244
	if got := d.evocacaoDoPet(pet); got != 1 {
		t.Errorf("pet com a face do Tigre: criatura = %d, want 1", got)
	}
	pet.EquipVisual[0] = summonTemplateFace(condor)
	if got := d.evocacaoDoPet(pet); got != 0 {
		t.Errorf("pet com a face do Condor: criatura = %d, want 0", got)
	}
	// Face desconhecida (cria de montaria, evocação de item): sem regra.
	pet.EquipVisual[0] = 999
	if got := d.evocacaoDoPet(pet); got != -1 {
		t.Errorf("face fora das oito: criatura = %d, want -1", got)
	}
	// Um monstro comum não é evocação nem com a face certa.
	mob := &world.Entity{ID: world.MaxUser + 2, Clan: 1}
	mob.EquipVisual[0] = 244
	if got := d.evocacaoDoPet(mob); got != -1 {
		t.Errorf("monstro comum: criatura = %d, want -1", got)
	}
}

// A regra vale SÓ contra jogador: o golpe em monstro segue pela conta do
// servidor, com o Damage que summonBonus escalona.
func TestEvocacaoSoTemRegraPropriaContraJogador(t *testing.T) {
	log := slog.New(slog.DiscardHandler)
	tigre := summonTemplate("Tigre")
	binary.LittleEndian.PutUint16(tigre[140:], 244)
	d := New(Config{Log: log, SummonMobs: [][]byte{tigre}})
	dono := &world.Entity{ID: 3, Mode: world.MobUser, Special: [4]int16{0, 0, 320, 0}, BaseSpecial: [4]int16{0, 0, 320, 0}}
	pet := &world.Entity{ID: world.MaxUser + 1, Clan: summonClan, Summoner: 3}
	pet.EquipVisual[0] = 244

	jogador := &world.Entity{ID: 4, Mode: world.MobUser, AC: evocacaoDefesaRef, HP: 9000, MaxHP: 9000}
	dano, ok := d.golpeDaEvocacaoEmJogador(dono, pet, jogador)
	if !ok {
		t.Fatal("contra jogador a regra tem de valer")
	}
	if dano != evocacaoDano[0] {
		t.Errorf("dano = %d, want %d", dano, evocacaoDano[0])
	}

	monstro := &world.Entity{ID: world.MaxUser + 2, Clan: 1, AC: evocacaoDefesaRef, HP: 9000}
	if _, ok := d.golpeDaEvocacaoEmJogador(dono, pet, monstro); ok {
		t.Error("contra monstro a regra NÃO pode valer — o golpe é o da conta do servidor")
	}

	// Sem dono não há maestria de onde tirar a força.
	if _, ok := d.golpeDaEvocacaoEmJogador(nil, pet, jogador); ok {
		t.Error("sem dono não há maestria de onde tirar a força")
	}
}
