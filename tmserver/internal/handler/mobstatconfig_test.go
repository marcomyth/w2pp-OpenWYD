package handler

import (
	"context"
	"encoding/binary"
	"errors"
	"log/slog"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/mobstat"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

// errDaFicha is the read failure the keep-what-is-loaded test injects.
var errDaFicha = errors.New("dbserver fora do ar")

// fonteDeFicha is a MobStatSource that answers from memory. The poll calls it
// from a detached goroutine, so the test changes it under the mutex.
type fonteDeFicha struct {
	mu      sync.Mutex
	versao  int64
	overs   map[string]mobstat.Override
	lerErr  error
	fetches int
}

func (f *fonteDeFicha) Version(context.Context) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.versao, f.lerErr
}

func (f *fonteDeFicha) Fetch(context.Context) (map[string]mobstat.Override, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.fetches++
	if f.lerErr != nil {
		return nil, f.lerErr
	}
	// Copy: the map crosses into the loop goroutine.
	out := make(map[string]mobstat.Override, len(f.overs))
	for k, v := range f.overs {
		out[k] = v
	}
	return out, nil
}

func (f *fonteDeFicha) trocar(versao int64, overs map[string]mobstat.Override) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.versao, f.overs = versao, overs
}

// fichaComExp is a template file whose Exp is known, so a test can tell the
// file's value from an override's.
func fichaComExp(nome string, exp int64) []byte {
	b := plainMobTemplate(nome)
	// Exp é STRUCT_MOB.Exp @32, um long long — FORA do bloco de Score. Uso o mesmo
	// deslocamento que protocol.ParseMobBasics, para o teste não inventar layout.
	binary.LittleEndian.PutUint64(b[32:], uint64(exp))
	return b
}

// expDoMolde reads Exp back out of raw template bytes.
func expDoMolde(b []byte) int64 {
	return int64(binary.LittleEndian.Uint64(b[32:]))
}

// fichaViva is an Override with the Exp under test plus a body that can actually
// spawn: Apply is AUTHORITATIVE over the whole Score block, so an override with
// only Exp set would produce a level-0 monster with no HP.
func fichaViva(exp int64) mobstat.Override {
	return mobstat.Override{Exp: exp, Level: 10, MaxHp: 100, Hp: 100, MaxMp: 10, Mp: 10}
}

// servirComFicha serves a world whose Dispatcher reads monster sheets from src,
// with ONE generator for nome whose template is the file plus the boot override.
func servirComFicha(t *testing.T, src MobStatSource, nome string, arquivo []byte,
	boot map[string]mobstat.Override, versaoBoot int64) (func(), *Dispatcher, *world.World) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	log := slog.New(slog.DiscardHandler)
	d := New(Config{
		Log: log, MobStats: src, MobStatOverrides: boot, MobStatVersion: versaoBoot,
		MobTemplate: func(string) ([]byte, error) {
			// O "arquivo" do conteúdo: uma cópia, porque quem lê não pode ficar com
			// o mesmo slice que o gerador está usando.
			return append([]byte(nil), arquivo...), nil
		},
	})
	w := world.New(world.Config{GridDim: 32}, log, nil, d.Handle)
	w.RegisterGenerators([]*world.Generator{{
		LeaderName: nome,
		LeaderTmpl: mobstat.ApplyOverride(append([]byte(nil), arquivo...), nome, boot),
		MaxNumMob:  1, MinGroup: 1, MaxGroup: 1,
		SegX: [5]int16{10}, SegY: [5]int16{10},
	}})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { _ = w.Serve(ctx, ln); close(done) }()
	return func() {
		cancel()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Error("o servidor não parou")
		}
	}, d, w
}

// recarregarAgora makes the next pollMobStats the one that asks dbServer, and
// waits for the detached work to land back in the loop.
func recarregarAgora(t *testing.T, d *Dispatcher, w *world.World) {
	t.Helper()
	noLaco(t, w, func(w *world.World) {
		d.mobStatPollTick = mobStatPollPeriod - 1
		d.pollMobStats(w)
	})
	// O poll roda destacado; espera o resultado voltar para dentro do laço.
	prazo := time.Now().Add(2 * time.Second)
	for {
		var ocupado bool
		noLaco(t, w, func(*world.World) { ocupado = d.mobStatPolling })
		if !ocupado {
			return
		}
		if time.Now().After(prazo) {
			t.Fatal("a recarga da ficha não voltou para o laço")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// TestFichaNovaChegaNoQueNASCEDepois is the test that matters: it is not enough
// for a table to change, because the spawn copies g.LeaderTmpl. A monster BORN
// after the reload has to carry the new number.
func TestFichaNovaChegaNoQueNASCEDepois(t *testing.T) {
	const nome = "Bicho_De_Teste"
	arquivo := fichaComExp(nome, 1000)
	src := &fonteDeFicha{versao: 7}
	parar, d, w := servirComFicha(t, src, nome, arquivo, nil, 7)
	defer parar()

	// Antes: o gerador carrega o valor do arquivo.
	var antes int64
	noLaco(t, w, func(w *world.World) { antes = expDoMolde(w.GeneratorAt(0).LeaderTmpl) })
	if antes != 1000 {
		t.Fatalf("antes da recarga o molde do gerador tem Exp %d, quero 1000", antes)
	}

	src.trocar(8, map[string]mobstat.Override{nome: fichaViva(5000)})
	recarregarAgora(t, d, w)

	var depois int64
	noLaco(t, w, func(w *world.World) { depois = expDoMolde(w.GeneratorAt(0).LeaderTmpl) })
	if depois != 5000 {
		t.Fatalf("o molde do gerador ficou com Exp %d, quero 5000", depois)
	}

	// E a prova de verdade: um monstro NASCIDO agora paga o valor novo.
	var nascido int64
	noLaco(t, w, func(w *world.World) {
		ids := w.GenerateMobUpTo(0, 1)
		if len(ids) == 0 {
			t.Error("o gerador não fez nascer ninguém")
			return
		}
		if e := w.Entity(ids[0]); e != nil {
			nascido = e.Exp
		}
	})
	if nascido != 5000 {
		t.Errorf("o monstro que nasceu depois da recarga paga %d, quero 5000", nascido)
	}
}

// TestApagarExcecaoVoltaAoArquivo is the case a first implementation always
// forgets: the override is DELETED in the panel, and the monster has to go back
// to the file's value without a restart. It only works because the rebuild walks
// the UNION of the old and new names.
func TestApagarExcecaoVoltaAoArquivo(t *testing.T) {
	const nome = "Bicho_De_Teste"
	arquivo := fichaComExp(nome, 1000)
	boot := map[string]mobstat.Override{nome: fichaViva(5000)}
	src := &fonteDeFicha{versao: 7, overs: boot}
	parar, d, w := servirComFicha(t, src, nome, arquivo, boot, 7)
	defer parar()

	var antes int64
	noLaco(t, w, func(w *world.World) { antes = expDoMolde(w.GeneratorAt(0).LeaderTmpl) })
	if antes != 5000 {
		t.Fatalf("o boot devia ter aplicado a exceção: Exp %d, quero 5000", antes)
	}

	// A exceção foi APAGADA no painel: o conjunto novo não tem o nome.
	src.trocar(8, map[string]mobstat.Override{})
	recarregarAgora(t, d, w)

	var depois int64
	noLaco(t, w, func(w *world.World) { depois = expDoMolde(w.GeneratorAt(0).LeaderTmpl) })
	if depois != 1000 {
		t.Errorf("depois de apagar a exceção o molde tem Exp %d, quero 1000 (o valor do arquivo)", depois)
	}
}

// TestRecargaSemMudancaNaoMexeEmNada: o bump veio de NPC ou de loja, que
// compartilham a versão. A recarga acontece e não muda ficha nenhuma — é o caso
// normal do compartilhamento, e o que o log conta como fichas=0.
func TestRecargaSemMudancaNaoMexeEmNada(t *testing.T) {
	const nome = "Bicho_De_Teste"
	arquivo := fichaComExp(nome, 1000)
	boot := map[string]mobstat.Override{nome: fichaViva(5000)}
	src := &fonteDeFicha{versao: 7, overs: boot}
	parar, d, w := servirComFicha(t, src, nome, arquivo, boot, 7)
	defer parar()

	// Versão nova, MESMAS exceções: é o bump de outra coisa.
	src.trocar(8, boot)
	recarregarAgora(t, d, w)

	var exp, versao int64
	noLaco(t, w, func(w *world.World) {
		exp, versao = expDoMolde(w.GeneratorAt(0).LeaderTmpl), d.mobStatVersion
	})
	if exp != 5000 {
		t.Errorf("a ficha mudou sem a exceção ter mudado: Exp %d, quero 5000", exp)
	}
	if versao != 8 {
		t.Errorf("a versão não avançou: %d, quero 8", versao)
	}
}

// TestLeituraQueFalhaMantemAFicha: uma piscada de rede NÃO pode devolver o
// monstro ao valor do arquivo calado — seria uma mudança de ritmo do servidor
// inteiro causada por um erro de leitura.
func TestLeituraQueFalhaMantemAFicha(t *testing.T) {
	const nome = "Bicho_De_Teste"
	arquivo := fichaComExp(nome, 1000)
	boot := map[string]mobstat.Override{nome: fichaViva(5000)}
	src := &fonteDeFicha{versao: 7, overs: boot}
	parar, d, w := servirComFicha(t, src, nome, arquivo, boot, 7)
	defer parar()

	src.mu.Lock()
	src.versao, src.lerErr = 8, errDaFicha
	src.mu.Unlock()
	recarregarAgora(t, d, w)

	var exp, versao int64
	noLaco(t, w, func(w *world.World) {
		exp, versao = expDoMolde(w.GeneratorAt(0).LeaderTmpl), d.mobStatVersion
	})
	if exp != 5000 {
		t.Errorf("a leitura falhou e a ficha caiu para %d; tinha de ficar em 5000", exp)
	}
	if versao != 7 {
		t.Errorf("a versão avançou apesar da falha: %d, quero 7", versao)
	}
}
