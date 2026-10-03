package world

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"
	"testing"
	"time"
)

// bancoDoFiltro é o banco dos testes da fila do filtro das fadas: guarda a ordem
// do que foi gravado, pode segurar uma gravação até o teste soltar, e pode falhar.
type bancoDoFiltro struct {
	NopPersistence

	mu      sync.Mutex
	eventos []string          // "filtro:conta" e "par:conta", na ordem em que o banco TERMINOU cada um
	filtros []FadaFiltroSalvo // o que cada SaveFadaFiltro levou
	falha   []error           // o erro de cada gravação do filtro, em ordem (acabou: nil)
	segura  chan struct{}     // não-nil: a PRIMEIRA gravação do filtro espera fechar
	chegou  chan struct{}     // recebe quando uma gravação do filtro entra no banco
}

func (b *bancoDoFiltro) SaveFadaFiltro(_ context.Context, conta int64, slot int, ligado bool, itens []int16) error {
	b.mu.Lock()
	primeira := len(b.filtros) == 0
	n := len(b.filtros)
	b.filtros = append(b.filtros, FadaFiltroSalvo{Conta: conta, Slot: slot, Ligado: ligado, Itens: slices.Clone(itens)})
	var err error
	if n < len(b.falha) {
		err = b.falha[n]
	}
	segura := b.segura
	b.mu.Unlock()
	if b.chegou != nil {
		b.chegou <- struct{}{}
	}
	if primeira && segura != nil {
		<-segura
	}
	b.mu.Lock()
	b.eventos = append(b.eventos, fmt.Sprintf("filtro:%d", conta))
	b.mu.Unlock()
	return err
}

func (b *bancoDoFiltro) SaveOnShutdown(_ context.Context, cs CharacterSave, _, _ int64, _ bool) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.eventos = append(b.eventos, fmt.Sprintf("par:%d", cs.AccountID))
	return nil
}

func (b *bancoDoFiltro) gravados() []FadaFiltroSalvo {
	b.mu.Lock()
	defer b.mu.Unlock()
	return slices.Clone(b.filtros)
}

// voltaDoFiltro roda no "laço" do teste a próxima volta de gravação.
func voltaDoFiltro(t *testing.T, w *World) {
	t.Helper()
	select {
	case ev := <-w.callbacks:
		ev.apply(w)
	case <-time.After(5 * time.Second):
		t.Fatal("a gravação do filtro não voltou")
	}
}

func estadoDoFiltro(ligado bool, itens ...int16) FadaFiltroSalvo {
	return FadaFiltroSalvo{Conta: 101, Slot: 2, Ligado: ligado, Itens: itens}
}

func mesmoFiltro(a, b FadaFiltroSalvo) bool {
	return a.Conta == b.Conta && a.Slot == b.Slot && a.Ligado == b.Ligado && slices.Equal(a.Itens, b.Itens)
}

// A MUDANÇA QUE ESPERAVA NA FILA É GRAVADA COM O JOGADOR FORA. Antes, a fila era
// da sessão: a volta da gravação em curso era descartada na saída, e o estado
// mais novo nunca ia ao banco — o filtro voltava ligado no login seguinte.
func TestFadaFiltroFilaTerminaComOJogadorFora(t *testing.T) {
	b := &bancoDoFiltro{segura: make(chan struct{}), chegou: make(chan struct{}, 8)}
	w := mundoParaDesligar(t, Config{}, b, 1)
	w.sessions[1].Slot = 2

	ligado, mexido, desligado := estadoDoFiltro(true, 400), estadoDoFiltro(true, 400, 401), estadoDoFiltro(false, 400, 401)
	w.GravaFadaFiltro(estadoDoFiltro(false, 400), ligado, nil)
	<-b.chegou // a primeira está no banco, presa
	w.GravaFadaFiltro(ligado, mexido, nil)
	w.GravaFadaFiltro(mexido, desligado, nil)

	// O login que acontecer agora tem de ver o estado mais novo, e não o do banco.
	if p, ok := w.FadaFiltroACaminho(101, 2); !ok || !mesmoFiltro(p, desligado) {
		t.Fatalf("a caminho = %+v %v; queria o desligado", p, ok)
	}

	// O jogador sai: a sessão some.
	w.sessions[1], w.entities[1] = nil, nil

	close(b.segura)
	voltaDoFiltro(t, w) // volta da primeira: dispara a que esperava
	<-b.chegou
	voltaDoFiltro(t, w)

	g := b.gravados()
	if len(g) != 2 || !mesmoFiltro(g[0], ligado) || !mesmoFiltro(g[1], desligado) {
		t.Fatalf("gravações = %+v; queria [ligado, desligado] (só o mais novo da fila)", g)
	}
	if w.FadaFiltrosNaFila() != 0 {
		t.Errorf("a fila ficou com %d personagens depois de gravar tudo", w.FadaFiltrosNaFila())
	}
	if _, ok := w.FadaFiltroACaminho(101, 2); ok {
		t.Error("ainda diz que há estado a caminho com tudo gravado")
	}
}

// NA FALHA A MEMÓRIA VOLTA AO QUE O BANCO TEM, o jogador é avisado com esse
// estado, e o estado confirmado é regravado uma vez (a falha por prazo pode ter
// gravado). O lado seguro é o que não descarta: a tela nunca mostra "desligado"
// com o banco guardando "ligado".
func TestFadaFiltroFalhaVoltaAMemoria(t *testing.T) {
	b := &bancoDoFiltro{falha: []error{errors.New("banco fora")}, chegou: make(chan struct{}, 8)}
	w := mundoParaDesligar(t, Config{}, b, 1)
	s, e := w.sessions[1], w.entities[1]
	s.Slot = 2

	antes, novo := estadoDoFiltro(false, 400), estadoDoFiltro(true, 400)
	e.FadaFiltroLigado, e.FadaFiltro = novo.Ligado, novo.Itens // o handler já mudou a memória
	avisos := 0
	w.GravaFadaFiltro(antes, novo, func(_ *World, sa *Session, ea *Entity) {
		avisos++
		if sa != s || ea != e {
			t.Error("o aviso foi para outra sessão")
		}
		if ea.FadaFiltroLigado {
			t.Error("o aviso saiu com a memória ainda no estado que não gravou")
		}
	})
	<-b.chegou
	voltaDoFiltro(t, w)

	if e.FadaFiltroLigado || !slices.Equal(e.FadaFiltro, antes.Itens) {
		t.Errorf("memória depois da falha = ligado %v itens %v; queria o estado do banco %+v",
			e.FadaFiltroLigado, e.FadaFiltro, antes)
	}
	if avisos != 1 {
		t.Errorf("avisos = %d; queria 1", avisos)
	}
	<-b.chegou
	voltaDoFiltro(t, w) // a volta do reparo
	g := b.gravados()
	if len(g) != 2 || !mesmoFiltro(g[0], novo) || !mesmoFiltro(g[1], antes) {
		t.Fatalf("gravações = %+v; queria [novo, confirmado de novo]", g)
	}
	if w.FadaFiltrosNaFila() != 0 {
		t.Error("a fila não esvaziou depois do reparo")
	}
	if w.SavesFalhados() != 1 {
		t.Errorf("saves falhados = %d; queria 1", w.SavesFalhados())
	}
}

// O reparo que falha PARA: não há laço de tentativas contra um banco fora.
func TestFadaFiltroReparoQueFalhaPara(t *testing.T) {
	erro := errors.New("banco fora")
	b := &bancoDoFiltro{falha: []error{erro, erro, erro, erro}, chegou: make(chan struct{}, 8)}
	w := mundoParaDesligar(t, Config{}, b, 1)
	w.sessions[1].Slot = 2

	w.GravaFadaFiltro(estadoDoFiltro(false), estadoDoFiltro(false, 400), nil)
	<-b.chegou
	voltaDoFiltro(t, w)
	<-b.chegou
	voltaDoFiltro(t, w)
	w.WaitSaves()

	if n := len(b.gravados()); n != 2 {
		t.Errorf("tentativas = %d; queria 2 (a gravação e um reparo)", n)
	}
	if w.FadaFiltrosNaFila() != 0 {
		t.Error("a fila não esvaziou depois do reparo falhar")
	}
	select {
	case <-w.callbacks:
		t.Error("sobrou uma volta na fila: há uma terceira tentativa")
	default:
	}
}

// Com o jogador fora, a falha tenta o MESMO estado mais uma vez (não há tela
// para corrigir, e o que ele pediu por último é o que vale).
func TestFadaFiltroFalhaComOJogadorForaTentaDeNovo(t *testing.T) {
	b := &bancoDoFiltro{falha: []error{errors.New("banco fora")}, chegou: make(chan struct{}, 8)}
	w := mundoParaDesligar(t, Config{}, b, 0)

	desligado := estadoDoFiltro(false, 400)
	w.GravaFadaFiltro(estadoDoFiltro(true, 400), desligado, nil)
	<-b.chegou
	voltaDoFiltro(t, w)
	<-b.chegou
	voltaDoFiltro(t, w)

	g := b.gravados()
	if len(g) != 2 || !mesmoFiltro(g[0], desligado) || !mesmoFiltro(g[1], desligado) {
		t.Fatalf("gravações = %+v; queria o desligado duas vezes", g)
	}
}

// O DESLIGAMENTO GRAVA O FILTRO QUE ESTAVA NA FILA, e na ordem certa: espera a
// gravação que já estava no ar (senão o estado velho dela chegaria depois e
// cobriria o novo) e grava o filtro ANTES do par do personagem, que solta a
// posse da conta para o servidor novo.
func TestDesligamentoGravaOFiltroDaFada(t *testing.T) {
	b := &bancoDoFiltro{segura: make(chan struct{}), chegou: make(chan struct{}, 8)}
	w := mundoParaDesligar(t, Config{ShutdownSaveDeadline: 5 * time.Second}, b, 1)
	w.sessions[1].Slot = 2

	ligado, desligado := estadoDoFiltro(true, 400), estadoDoFiltro(false, 400)
	w.GravaFadaFiltro(estadoDoFiltro(false, 400), ligado, nil)
	<-b.chegou // no ar, presa
	w.GravaFadaFiltro(ligado, desligado, nil)

	// Um segundo personagem, de quem já saiu do jogo, com gravação no ar também.
	deFora := FadaFiltroSalvo{Conta: 900, Slot: 0, Ligado: false, Itens: []int16{500}}
	w.GravaFadaFiltro(FadaFiltroSalvo{Conta: 900, Ligado: true, Itens: []int16{500}}, deFora, nil)
	<-b.chegou

	go func() {
		time.Sleep(50 * time.Millisecond)
		close(b.segura)
	}()
	w.shutdown(false)

	g := b.gravados()
	var doJogador, doDeFora []FadaFiltroSalvo
	for _, f := range g {
		if f.Conta == 101 {
			doJogador = append(doJogador, f)
		} else {
			doDeFora = append(doDeFora, f)
		}
	}
	if len(doJogador) != 2 || !mesmoFiltro(doJogador[0], ligado) || !mesmoFiltro(doJogador[1], desligado) {
		t.Fatalf("filtro de quem está em jogo = %+v; queria [ligado, desligado]", doJogador)
	}
	// O de fora é regravado pelo desligamento (a volta dele não roda mais).
	if len(doDeFora) != 2 || !mesmoFiltro(doDeFora[1], deFora) {
		t.Fatalf("filtro de quem já saiu = %+v; queria o estado dele gravado no desligamento", doDeFora)
	}
	b.mu.Lock()
	eventos := slices.Clone(b.eventos)
	b.mu.Unlock()
	par := slices.Index(eventos, "par:101")
	ultimoFiltro := -1
	for i, ev := range eventos {
		if ev == "filtro:101" {
			ultimoFiltro = i
		}
	}
	if par < 0 || par < ultimoFiltro {
		t.Errorf("ordem no banco = %v; o par da conta saiu antes do filtro dela", eventos)
	}
	if w.FadaFiltrosNaFila() != 0 {
		t.Error("o desligamento deixou filtro na fila")
	}
}
