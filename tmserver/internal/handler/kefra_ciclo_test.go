package handler

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/worldcfg"
)

// fonteDoCiclo é a ponte com o dbServer, de mentira. O SetKefraState é chamado
// FORA do laço (GoDetached roda o trabalho numa goroutine), então tudo aqui passa
// pelo mutex: sem ele o teste com -race acusa corrida com as asserções.
type fonteDoCiclo struct {
	mu       sync.Mutex
	live     bool
	guild    int32
	chamadas int
	erro     error
}

func (f *fonteDoCiclo) Version(context.Context) (int64, error) { return 1, nil }

func (f *fonteDoCiclo) Snapshot(context.Context) (worldcfg.Snapshot, error) {
	return worldcfg.Snapshot{}, nil
}

func (f *fonteDoCiclo) UpdateProgress(context.Context, int64, int32) (bool, error) {
	return true, nil
}

func (f *fonteDoCiclo) SetKefraState(_ context.Context, live bool, guildID int32) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.chamadas++
	f.live, f.guild = live, guildID
	if f.erro != nil {
		return 0, f.erro
	}
	return 9, nil
}

func (f *fonteDoCiclo) estado() (bool, int32, int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.live, f.guild, f.chamadas
}

// esperarGravacao espera a gravação assíncrona chegar à fonte.
func (f *fonteDoCiclo) esperarGravacao(t *testing.T) (bool, int32) {
	t.Helper()
	prazo := time.Now().Add(2 * time.Second)
	for time.Now().Before(prazo) {
		if live, guild, n := f.estado(); n > 0 {
			return live, guild
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("a gravação do estado do Kefra não chegou ao banco")
	return false, 0
}

// geradorNoPonto é um bloco de um monstro só, parado onde o teste quiser.
func geradorNoPonto(nome string, x, y int16) *world.Generator {
	return &world.Generator{
		Name: nome, MinuteGenerate: -1, MaxNumMob: 1,
		LeaderName: nome, LeaderTmpl: expMobTemplate(400, 1000, 0),
		SegX: [5]int16{x}, SegY: [5]int16{y},
	}
}

// mundoDoCicloDoKefra tem a grade inteira (a caixa do saque fica perto de 2350,
// 3900) e o chefe no ponto real. Os guardas ficam FORA da caixa de propósito:
// dentro dela cada um contaria como monstro no sorteio do saque.
func mundoDoCicloDoKefra(t *testing.T, fonte worldcfg.Source) (*Dispatcher, *world.World) {
	t.Helper()
	log := slog.New(slog.DiscardHandler)
	d := New(Config{Log: log, WorldEvents: fonte})
	w := world.New(world.Config{GridDim: 4096}, log, world.NopPersistence{}, d.Handle)
	gens := make([]*world.Generator, world.KefraGuardLast+1)
	gens[world.KefraBossGenIndex] = geradorNoPonto("Kefra", kefraChefePos[0], kefraChefePos[1])
	for idx := world.KefraBossGenIndex + 1; idx <= world.KefraGuardLast; idx++ {
		gens[idx] = geradorNoPonto("Guarda_Kefra", int16(100+idx), 100)
	}
	w.RegisterGenerators(gens)
	return d, w
}

// chefeVivo levanta o Kefra e devolve a entidade dele.
func chefeVivo(t *testing.T, w *world.World) *world.Entity {
	t.Helper()
	ids := w.GenerateMob(world.KefraBossGenIndex)
	if len(ids) != 1 {
		t.Fatalf("GenerateMob(Kefra) = %v, quero um chefe", ids)
	}
	e := w.Entity(ids[0])
	if e == nil {
		t.Fatal("o chefe nasceu sem entidade")
	}
	return e
}

// matadorDoKefra é quem bate. Clan 0 é um dos quatro que o legado aceita.
func matadorDoKefra(guilda uint16) *world.Entity {
	return &world.Entity{
		ID: 0, Mode: world.MobUser, Name: "Matador",
		Level: 400, HP: 1000, MaxHP: 1000, MP: 100, MaxMP: 100,
		Guild: guilda,
		Str:   8, Int: 4, Dex: 7, Con: 6,
		BaseStr: 8, BaseInt: 4, BaseDex: 7, BaseCon: 6,
	}
}

// itemDoSaqueDoKefra é de propósito um índice <= 390: o laço de drop comum pula
// esses (mobkilled.go, "it.Index <= 390"), e o saque do Kefra no legado não
// filtra nada. Assim o que chega na bolsa é só o saque do chefe, e o teste conta
// sorteios em vez de somar dois caminhos.
const itemDoSaqueDoKefra = int16(300)

// encherOSaqueDoChefe põe o mesmo item em todas as casas que o sorteio alcança,
// então qualquer rand()%60 devolve item — o teste conta quantas vezes sorteou,
// não o que saiu.
func encherOSaqueDoChefe(chefe *world.Entity) {
	for i := 0; i < kefraSorteios; i++ {
		chefe.Carry[i] = world.Item{Index: itemDoSaqueDoKefra}
	}
}

func itensNaBolsa(e *world.Entity, index int16) int {
	n := 0
	for _, it := range e.Carry {
		if it.Index == index {
			n++
		}
	}
	return n
}

// Matar o Kefra com guilda liga o estado NA HORA, dentro do laço, e só depois
// confirma no banco. No legado a XP muda no golpe seguinte, não minutos depois:
// por isso o estado do processo não espera a gravação.
//
// KefraLive no legado guarda o ID DA GUILDA (MobKilled.cpp:1463), e é assim que
// a 0067 separa as duas coisas: o interruptor e a guilda.
func TestKefraMorteComGuildaLigaOEstadoNaHora(t *testing.T) {
	fonte := &fonteDoCiclo{}
	d, w := mundoDoCicloDoKefra(t, fonte)
	chefe := chefeVivo(t, w)
	matador := matadorDoKefra(7)
	w.SetGuildName(7, "CavaleirosDoKefra")
	w.SetGuildFame(7, 40)

	d.mobKilled(w, matador, chefe)

	if !d.expEvents.KefraLive {
		t.Error("o estado do processo não ligou na hora da morte")
	}
	if d.kefraGuildID != 7 {
		t.Errorf("guilda no processo = %d, want 7", d.kefraGuildID)
	}
	if info, ok := w.GuildInfo(7); !ok || info.Fame != 40+kefraFamaPremio {
		t.Errorf("fama = %+v, want %d", info, 40+kefraFamaPremio)
	}
	if live, guild := fonte.esperarGravacao(t); !live || guild != 7 {
		t.Errorf("gravação = (live %v, guilda %d), want (true, 7)", live, guild)
	}
}

// Sem guilda o legado liga o estado do mesmo jeito (KefraLive = 1,
// MobKilled.cpp:1487) e não tem fama para dar.
func TestKefraMorteSemGuildaLigaOEstado(t *testing.T) {
	fonte := &fonteDoCiclo{}
	d, w := mundoDoCicloDoKefra(t, fonte)
	chefe := chefeVivo(t, w)

	d.mobKilled(w, matadorDoKefra(0), chefe)

	if !d.expEvents.KefraLive {
		t.Error("o estado não ligou na morte sem guilda")
	}
	if d.kefraGuildID != 0 {
		t.Errorf("guilda no processo = %d, want 0", d.kefraGuildID)
	}
	if live, guild := fonte.esperarGravacao(t); !live || guild != 0 {
		t.Errorf("gravação = (live %v, guilda %d), want (true, 0)", live, guild)
	}
}

// Clan fora de {0,4,7,8} não conta: no legado esse ramo só REMOVE o monstro e
// nem entra no bloco do Kefra (MobKilled.cpp:1437-1438). Nada de estado, nada de
// fama, nada de saque.
func TestKefraMorteDeClanForaDoConjuntoNaoConta(t *testing.T) {
	fonte := &fonteDoCiclo{}
	d, w := mundoDoCicloDoKefra(t, fonte)
	chefe := chefeVivo(t, w)
	encherOSaqueDoChefe(chefe)
	matador := matadorDoKefra(7)
	matador.Clan = 5
	w.SetGuildFame(7, 40)

	d.mobKilled(w, matador, chefe)

	if d.expEvents.KefraLive {
		t.Error("um Clan fora do conjunto ligou o estado")
	}
	if info, _ := w.GuildInfo(7); info.Fame != 40 {
		t.Errorf("fama = %d, want 40 intacta", info.Fame)
	}
	if n := itensNaBolsa(matador, itemDoSaqueDoKefra); n != 0 {
		t.Errorf("%d itens de saque com Clan fora do conjunto, want 0", n)
	}
	if _, _, chamadas := fonte.estado(); chamadas != 0 {
		t.Errorf("%d gravações, want nenhuma", chamadas)
	}
}

// O saque é UM sorteio por monstro de pé na caixa 2335-2394 x 3896-3954
// (MobKilled.cpp:1494-1509), e tudo vai para quem matou.
//
// O CHEFE CONTA A SI MESMO: ele fica dentro da própria caixa e ainda está na
// grade quando a varredura roda (o despawn vem no fim de mobKilled). São dois
// monstros semeados mais ele, logo três itens.
func TestKefraSaqueUmSorteioPorMonstroNaCaixa(t *testing.T) {
	fonte := &fonteDoCiclo{}
	d, w := mundoDoCicloDoKefra(t, fonte)
	chefe := chefeVivo(t, w)
	encherOSaqueDoChefe(chefe)
	if !kefraCaixa.contains(chefe.X, chefe.Y) {
		t.Fatalf("o chefe nasceu em (%d,%d), fora da própria caixa", chefe.X, chefe.Y)
	}
	for i, p := range [][2]int16{{2340, 3900}, {2341, 3900}} {
		if id := w.SpawnMob(expMobTemplate(100, 10, 0), p[0], p[1]); id < 0 {
			t.Fatalf("não semeei o monstro %d da caixa", i)
		}
	}
	matador := matadorDoKefra(0)

	d.mobKilled(w, matador, chefe)

	if n := itensNaBolsa(matador, itemDoSaqueDoKefra); n != 3 {
		t.Errorf("%d itens de saque, want 3 (dois monstros mais o próprio chefe)", n)
	}
}

// Bolsa cheia perde o item, como o PutItem do legado — e o servidor registra,
// porque item que desaparece sem rastro é o que ninguém consegue investigar.
func TestKefraSaqueComBolsaCheiaPerde(t *testing.T) {
	fonte := &fonteDoCiclo{}
	d, w := mundoDoCicloDoKefra(t, fonte)
	chefe := chefeVivo(t, w)
	encherOSaqueDoChefe(chefe)
	matador := matadorDoKefra(0)
	for i := range matador.Carry {
		matador.Carry[i] = world.Item{Index: 1297}
	}

	d.mobKilled(w, matador, chefe)

	if n := itensNaBolsa(matador, itemDoSaqueDoKefra); n != 0 {
		t.Errorf("%d itens de saque entraram numa bolsa cheia, want 0", n)
	}
	if n := itensNaBolsa(matador, 1297); n != len(matador.Carry) {
		t.Errorf("a bolsa cheia mudou: %d de %d casas com o item original", n, len(matador.Carry))
	}
	// O estado liga mesmo com a bolsa cheia: o saque é prêmio, o estado é evento.
	if !d.expEvents.KefraLive {
		t.Error("a bolsa cheia impediu o estado de ligar")
	}
}

// Uma gravação que falha não pode deixar o processo dizendo "derrotado" e o banco
// "vivo": a próxima leitura reverteria em silêncio. O estado do processo volta e o
// log diz que voltou.
func TestKefraGravacaoQueFalhaReverteOProcesso(t *testing.T) {
	fonte := &fonteDoCiclo{erro: errors.New("dbserver fora")}
	d, w := mundoDoCicloDoKefra(t, fonte)
	chefe := chefeVivo(t, w)

	d.mobKilled(w, matadorDoKefra(7), chefe)

	if !d.expEvents.KefraLive {
		t.Fatal("o estado tinha de ligar na hora, antes de a gravação falhar")
	}
	fonte.esperarGravacao(t)
	// A reversão entra pela fila de callbacks do laço, então quem aplica é o
	// laço; aqui o mundo não está servindo, e o teste de ponta a ponta da
	// reversão é o do servidor de verdade.
	if _, _, chamadas := fonte.estado(); chamadas < 2 {
		t.Errorf("%d tentativas de gravação, want ao menos 2 (a nova tentativa)", chamadas)
	}
}

// Os guardas voltam ENQUANTO o chefe está de pé, e voltam rápido: eles são o anel
// que protege o chefe, e um anel que só se refaz na terça deixa o chefe sozinho
// pelo resto da semana. O chefe NÃO volta por aqui — ele é semanal.
func TestKefraGuardasVoltamComOChefeVivo(t *testing.T) {
	agora := time.Date(2026, time.September, 16, 15, 0, 0, 0, time.UTC) // quarta: fora da terça
	w := mundoDoKefra(t)
	d := dispatcherNaHora(&agora)
	d.tickKefraSemanal(w) // terça não é, então levanto tudo pela mão do teste
	for idx := world.KefraBossGenIndex; idx <= world.KefraGuardLast; idx++ {
		w.GenerateMob(idx)
	}
	if got := vivosDoKefra(w); got != 5 {
		t.Fatalf("montagem: %d vivos, quero o chefe e os 4 guardas", got)
	}

	// Mata um guarda e o chefe fica.
	var guarda int
	w.ForEachMob(func(id int, e *world.Entity) {
		if int(e.GenIndex) == world.KefraBossGenIndex+1 {
			guarda = id
		}
	})
	if guarda == 0 {
		t.Fatal("não achei o guarda para matar")
	}
	w.DespawnMob(guarda, 1)
	if got := vivosDoKefra(w); got != 4 {
		t.Fatalf("depois de matar o guarda: %d vivos, quero 4", got)
	}

	d.tickKefraGuardas(w)

	if got := vivosDoKefra(w); got != 5 {
		t.Errorf("depois do tique: %d vivos, quero o guarda de volta", got)
	}
}

// Com o chefe DERROTADO os guardas não voltam: a área fica limpa até a terça, que
// é o que dá sentido a ter matado o chefe.
func TestKefraGuardasNaoVoltamComOChefeDerrotado(t *testing.T) {
	agora := time.Date(2026, time.September, 16, 15, 0, 0, 0, time.UTC)
	w := mundoDoKefra(t)
	d := dispatcherNaHora(&agora)
	for idx := world.KefraBossGenIndex + 1; idx <= world.KefraGuardLast; idx++ {
		w.GenerateMob(idx) // só os guardas: o chefe está morto
	}
	var guarda int
	w.ForEachMob(func(id int, e *world.Entity) {
		if int(e.GenIndex) == world.KefraBossGenIndex+1 {
			guarda = id
		}
	})
	w.DespawnMob(guarda, 1)
	antes := vivosDoKefra(w)

	d.tickKefraGuardas(w)

	if got := vivosDoKefra(w); got != antes {
		t.Errorf("os guardas voltaram com o chefe derrotado: %d vivos, want %d", got, antes)
	}
}

// A terça DESLIGA o estado, além de renascer o chefe. No legado a volta semanal só
// acontece se o estado estiver ligado, e zerar a variável é a mesma linha
// (ProcessSecMinTimer.cpp:810).
func TestKefraTercaDesligaOEstado(t *testing.T) {
	agora := time.Date(2026, time.September, 15, 15, 0, 0, 0, time.UTC) // terça, 12:00 em Brasília
	w := mundoDoKefra(t)
	fonte := &fonteDoCiclo{}
	d := New(Config{
		Log:         slog.New(slog.DiscardHandler),
		Now:         func() time.Time { return agora },
		WorldEvents: fonte,
	})
	d.marcaKefra(true, 7) // chefe derrotado pela guilda 7, como ficaria depois da morte

	d.tickKefraSemanal(w)

	if d.expEvents.KefraLive {
		t.Error("a terça não desligou o estado")
	}
	if d.kefraGuildID != 0 {
		t.Errorf("guilda depois da terça = %d, want 0", d.kefraGuildID)
	}
	if got := vivosDoKefra(w); got != 5 {
		t.Errorf("%d vivos na terça, quero o chefe e os 4 guardas", got)
	}
	if live, guild := fonte.esperarGravacao(t); live || guild != 0 {
		t.Errorf("gravação da terça = (live %v, guilda %d), want (false, 0)", live, guild)
	}
}

// No boot, com o chefe DERROTADO no banco, a área nasce vazia: o povoamento
// levanta tudo antes de a configuração chegar (main.go), então este passo tira o
// que ele levantou — o mesmo formato do ApplyGeneratorOffBoot.
func TestKefraBootLimpaQuandoDerrotado(t *testing.T) {
	agora := time.Date(2026, time.September, 16, 15, 0, 0, 0, time.UTC)
	w := mundoDoKefra(t)
	d := dispatcherNaHora(&agora)
	for idx := world.KefraBossGenIndex; idx <= world.KefraGuardLast; idx++ {
		w.GenerateMob(idx)
	}
	d.marcaKefra(true, 7)

	d.ApplyKefraStateBoot(w)

	if got := vivosDoKefra(w); got != 0 {
		t.Errorf("%d vivos depois do boot com o chefe derrotado, want 0", got)
	}
}

// Leitura de configuração que falhou deixa o estado no padrão, que é VIVO — e
// então o boot não tira nada. É a escolha segura: um chefe a mais é um chefe; uma
// área vazia por engano tranca a cidade e o destrave do Celestial.
func TestKefraBootNaoLimpaQuandoVivo(t *testing.T) {
	agora := time.Date(2026, time.September, 16, 15, 0, 0, 0, time.UTC)
	w := mundoDoKefra(t)
	d := dispatcherNaHora(&agora)
	for idx := world.KefraBossGenIndex; idx <= world.KefraGuardLast; idx++ {
		w.GenerateMob(idx)
	}

	d.ApplyKefraStateBoot(w)

	if got := vivosDoKefra(w); got != 5 {
		t.Errorf("%d vivos depois do boot com o chefe vivo, want 5 intactos", got)
	}
}
