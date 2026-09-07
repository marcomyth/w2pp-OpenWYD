package world

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

// persistChat captures the batches, and can be told to fail.
type persistChat struct {
	NopPersistence
	mu     sync.Mutex
	lotes  [][]ChatLinha
	err    error
	espera chan struct{} // quando não-nil, segura a gravação até ser fechado
}

func (p *persistChat) RecordChat(_ context.Context, linhas []ChatLinha) error {
	if p.espera != nil {
		<-p.espera
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.err != nil {
		return p.err
	}
	p.lotes = append(p.lotes, linhas)
	return nil
}

func (p *persistChat) recebido(t *testing.T, querLinhas int) [][]ChatLinha {
	t.Helper()
	prazo := time.Now().Add(2 * time.Second)
	for {
		p.mu.Lock()
		n := 0
		for _, l := range p.lotes {
			n += len(l)
		}
		lotes := append([][]ChatLinha(nil), p.lotes...)
		p.mu.Unlock()
		if n >= querLinhas {
			return lotes
		}
		if time.Now().After(prazo) {
			t.Fatalf("chegaram %d linhas, queria %d", n, querLinhas)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func mundoChat(t *testing.T, p Persistence) *World {
	t.Helper()
	return New(Config{GridDim: 16}, slogDiscard(), p, nil)
}

func linha(texto string) ChatLinha {
	return ChatLinha{Tipo: ChatPublico, Character: "Falante", Texto: texto}
}

// Nada sai por linha: o chat é a coisa mais frequente do servidor, e uma ida ao
// banco por frase colocaria o banco no caminho de digitar.
func TestChatNaoEnviaUmaPorUma(t *testing.T) {
	p := &persistChat{}
	w := mundoChat(t, p)
	for i := 0; i < 10; i++ {
		w.RegistraChat(linha("oi"))
	}
	p.mu.Lock()
	n := len(p.lotes)
	p.mu.Unlock()
	if n != 0 {
		t.Errorf("mandou %d lotes com só 10 linhas na fila", n)
	}
	if len(w.chatBuf) != 10 {
		t.Errorf("fila = %d, want 10", len(w.chatBuf))
	}
}

func TestChatEnviaAoEncherOLote(t *testing.T) {
	p := &persistChat{}
	w := mundoChat(t, p)
	for i := 0; i < chatLote; i++ {
		w.RegistraChat(linha("oi"))
	}
	// GoDetached devolve o resultado pelo laço; aqui não há laço, então o envio
	// já saiu e o que se espera é a gravação em si.
	lotes := p.recebido(t, chatLote)
	if len(lotes) != 1 || len(lotes[0]) != chatLote {
		t.Fatalf("lotes = %v", func() []int {
			var n []int
			for _, l := range lotes {
				n = append(n, len(l))
			}
			return n
		}())
	}
}

// Linha vazia é o cliente conversando com o servidor, não gente falando. Uma
// tela cheia delas é uma tela que ninguém lê.
func TestChatDescartaLinhaVazia(t *testing.T) {
	w := mundoChat(t, &persistChat{})
	w.RegistraChat(linha(""))
	w.RegistraChat(ChatLinha{Tipo: ChatPublico, Character: "", Texto: "sem dono"})
	if len(w.chatBuf) != 0 {
		t.Errorf("fila = %d, want 0", len(w.chatBuf))
	}
}

// A hora tem de ser a da fala. Se a linha entrar sem hora, quem carimba é o
// registro, não o envio — senão a conversa inteira cai no mesmo instante.
func TestChatCarimbaAHoraDaFalaEnaoADoEnvio(t *testing.T) {
	w := mundoChat(t, &persistChat{})
	antes := time.Now()
	w.RegistraChat(linha("oi"))
	if w.chatBuf[0].At.Before(antes) {
		t.Error("carimbou uma hora anterior ao registro")
	}
	if w.chatBuf[0].At.After(time.Now()) {
		t.Error("carimbou uma hora no futuro")
	}
	// E uma hora já vinda de fora é respeitada.
	dado := time.Now().Add(-time.Hour)
	l := linha("antiga")
	l.At = dado
	w.RegistraChat(l)
	if !w.chatBuf[1].At.Equal(dado) {
		t.Errorf("trocou a hora da fala: %v", w.chatBuf[1].At)
	}
}

// Banco fora do ar não pode virar servidor morto: a fila para de crescer e as
// linhas mais VELHAS caem, que são as mais distantes do que se vai investigar.
func TestChatComABancoForaAFilaNaoCresceParaSempre(t *testing.T) {
	segura := make(chan struct{})
	p := &persistChat{espera: segura, err: errors.New("sem banco")}
	w := mundoChat(t, p)
	defer close(segura)

	for i := 0; i < chatTeto+chatLote+50; i++ {
		w.RegistraChat(linha("enche"))
	}
	if len(w.chatBuf) > chatTeto {
		t.Errorf("fila = %d, passou do teto de %d", len(w.chatBuf), chatTeto)
	}
	if w.chatDescartadas == 0 {
		t.Error("estourou o teto e não contou nenhuma descartada")
	}
}

// Um lote de cada vez. Dois competiriam pelas mesmas linhas e dobrariam a carga
// num banco que já está lento — que é a razão de o primeiro estar demorando.
func TestChatSoUmLoteDeCadaVez(t *testing.T) {
	segura := make(chan struct{})
	p := &persistChat{espera: segura}
	w := mundoChat(t, p)

	for i := 0; i < chatLote; i++ {
		w.RegistraChat(linha("primeiro"))
	}
	if !w.chatEnviando {
		t.Fatal("não marcou que tem lote em voo")
	}
	for i := 0; i < chatLote; i++ {
		w.RegistraChat(linha("segundo"))
	}
	p.mu.Lock()
	n := len(p.lotes)
	p.mu.Unlock()
	if n != 0 {
		t.Errorf("gravou %d lotes com o primeiro ainda preso", n)
	}
	if len(w.chatBuf) != chatLote {
		t.Errorf("fila = %d, want %d esperando o primeiro terminar", len(w.chatBuf), chatLote)
	}
	close(segura)
}

// Servidor calado nunca enche o lote; sem o relógio a conversa do dia ficaria
// na memória até alguém falar duzentas vezes.
func TestChatOTempoEsvaziaLoteIncompleto(t *testing.T) {
	p := &persistChat{}
	w := mundoChat(t, p)
	w.RegistraChat(linha("sozinha"))

	// Antes do intervalo, nada sai.
	w.chatTick(time.Now())
	p.mu.Lock()
	n := len(p.lotes)
	p.mu.Unlock()
	if n != 0 {
		t.Fatal("esvaziou antes da hora")
	}

	w.chatTick(time.Now().Add(2 * chatIntervalo))
	lotes := p.recebido(t, 1)
	if len(lotes[0]) != 1 {
		t.Errorf("lote = %d linhas, want 1", len(lotes[0]))
	}
}
