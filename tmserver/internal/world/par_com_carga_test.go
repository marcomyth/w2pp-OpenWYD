package world

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

// errDeProposito é a falha que o teste injeta.
var errDeProposito = errors.New("gravação recusada de propósito pelo teste")

// capturaPar registra qual das três formas de gravar foi usada, que é a coisa
// que o conserto do dupe decide.
type capturaPar struct {
	NopPersistence
	mu        sync.Mutex
	pares     int
	soCarga   int
	soPessoa  int
	falharPar bool
}

func (c *capturaPar) SalvarPersonagemComCarga(context.Context, CharacterSave, CargoSave, []int64, []int64, int64, int64) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.pares++
	if c.falharPar {
		return errDeProposito
	}
	return nil
}

func (c *capturaPar) SaveCargo(context.Context, CargoSave) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.soCarga++
	return nil
}

func (c *capturaPar) SaveCargoWithDeliveries(context.Context, CargoSave, []int64, []int64) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.soCarga++
	return nil
}

func (c *capturaPar) SaveOnShutdown(context.Context, CharacterSave) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.soPessoa++
	return nil
}

func (c *capturaPar) conta() (pares, soCarga, soPessoa int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.pares, c.soCarga, c.soPessoa
}

// esperaGravacao espera qualquer gravação chegar ao porto de persistência.
func (c *capturaPar) esperaGravacao(t *testing.T) {
	t.Helper()
	limite := time.Now().Add(2 * time.Second)
	for time.Now().Before(limite) {
		if p, a, b := c.conta(); p+a+b > 0 {
			time.Sleep(100 * time.Millisecond) // deixa uma segunda, se houver, aparecer
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("nenhuma gravação em 2 segundos")
}

// TestEntregaComSessaoEsperandoOBancoGravaOPar é o buraco que a revisão pegou.
//
// O critério do par era "a sessão está em UserPlay". Só que o personagem continua
// VIVO na memória em UserWaitDB, que é justamente o modo de quem está no meio de
// uma ida ao banco — criar guilda, promover. Uma entrega que chegasse nessa janela
// via "ninguém jogando" e gravava a CARGA SOZINHA, com a mochila do banco
// atrasada: o dupe de volta, na janela mais provável de todas, porque é o que
// acontece quando a pessoa saca da carga e cria a guilda.
func TestEntregaComSessaoEsperandoOBancoGravaOPar(t *testing.T) {
	for _, modo := range []struct {
		nome string
		m    Mode
	}{
		{"em jogo", UserPlay},
		{"esperando o banco", UserWaitDB},
		{"esperando o personagem", UserCharWait},
	} {
		t.Run(modo.nome, func(t *testing.T) {
			pc := &capturaPar{}
			w := New(Config{GridDim: 16}, slogDiscard(), pc, nil)
			s := &Session{Conn: 1, AccountID: 42, Mode: modo.m}
			w.sessions[1] = s
			w.entities[1] = &Entity{ID: 1, Mode: MobUser, HP: 100}
			w.SetCargo(42, &CargoState{})

			w.ApplyDeliveries(s, []Delivery{{ID: 21, Item: Item{Index: 4321}}})
			pc.esperaGravacao(t)

			pares, soCarga, soPessoa := pc.conta()
			if soCarga != 0 {
				t.Errorf("a carga foi gravada sozinha %d vez(es) com personagem vivo na conta — é o dupe", soCarga)
			}
			if pares == 0 {
				t.Errorf("a entrega não gravou o par (pares=%d, só personagem=%d)", pares, soPessoa)
			}
		})
	}
}

// TestEntregaSemPersonagemVivoGravaSoACarga é o outro lado: sem mochila viva não
// há o que discordar da carga, e forçar o par ali gravaria um personagem que não
// existe.
func TestEntregaSemPersonagemVivoGravaSoACarga(t *testing.T) {
	pc := &capturaPar{}
	w := New(Config{GridDim: 16}, slogDiscard(), pc, nil)
	s := &Session{Conn: 1, AccountID: 42, Mode: UserSelChar}
	w.sessions[1] = s // sem entidade: a conta está na tela de seleção
	w.SetCargo(42, &CargoState{})

	w.ApplyDeliveries(s, []Delivery{{ID: 22, Item: Item{Index: 4321}}})
	pc.esperaGravacao(t)

	pares, soCarga, _ := pc.conta()
	if pares != 0 {
		t.Errorf("gravou o par sem personagem vivo (%d)", pares)
	}
	if soCarga == 0 {
		t.Error("a carga não foi gravada")
	}
}

// TestEntidadeDocadaNaoContaComoMochilaViva: a entidade que sobrou depois que o
// personagem voltou para a seleção já foi gravada. Tratá-la como viva faria o par
// republicar estado velho por cima de um instantâneo mais novo.
func TestEntidadeDocadaNaoContaComoMochilaViva(t *testing.T) {
	pc := &capturaPar{}
	w := New(Config{GridDim: 16}, slogDiscard(), pc, nil)
	s := &Session{Conn: 1, AccountID: 42, Mode: UserSelChar}
	w.sessions[1] = s
	w.entities[1] = &Entity{ID: 1, Mode: MobUserDock, HP: 100}
	w.SetCargo(42, &CargoState{})

	w.ApplyDeliveries(s, []Delivery{{ID: 23, Item: Item{Index: 4321}}})
	pc.esperaGravacao(t)

	if pares, _, _ := pc.conta(); pares != 0 {
		t.Errorf("a entidade docada foi tratada como mochila viva (%d pares)", pares)
	}
}

// TestONumeroDoParSaiDoInstantaneo: o número tem de ser tirado QUANDO o
// instantâneo é tirado, e não quando a gravação sai.
//
// É a diferença entre ordenar o que se quis gravar e ordenar quem chegou primeiro
// ao banco — e é justamente porque a segunda ordem não é confiável que o número
// existe. Duas gravações podem sair na ordem certa e chegar na errada; o número
// que viaja com cada uma diz ao banco qual é a mais nova.
func TestONumeroDoParSaiDoInstantaneo(t *testing.T) {
	pc := &capturaSeq{}
	w := New(Config{GridDim: 16}, slogDiscard(), pc, nil)
	w.DefineEpocaDoPar(9)
	s := &Session{Conn: 1, AccountID: 42, Mode: UserPlay}
	w.sessions[1] = s
	w.entities[1] = &Entity{ID: 1, Mode: MobUser, HP: 100}
	w.SetCargo(42, &CargoState{})

	_, _, _, temCarga, primeiro := w.parDeSalvamento(s)
	_, _, _, _, segundo := w.parDeSalvamento(s)
	if !temCarga {
		t.Fatal("a conta tinha carga carregada")
	}
	if segundo <= primeiro {
		t.Errorf("números %d e %d: o segundo instantâneo tem de ter número maior", primeiro, segundo)
	}

	// E a época vai junto, senão o banco não sabe de que execução o número é.
	if err := SalvarPar(context.Background(), pc, CharacterSave{AccountID: 42}, CargoSave{AccountID: 42},
		true, nil, w.EpocaDoPar(), primeiro); err != nil {
		t.Fatal(err)
	}
	epoca, seq := pc.ultimo()
	if epoca != 9 || seq != primeiro {
		t.Errorf("chegou (época %d, número %d), queria (9, %d)", epoca, seq, primeiro)
	}
}

// capturaSeq guarda a época e o número que chegaram ao porto de persistência.
type capturaSeq struct {
	NopPersistence
	mu            sync.Mutex
	epoca, numero int64
}

func (c *capturaSeq) SalvarPersonagemComCarga(_ context.Context, _ CharacterSave, _ CargoSave,
	_, _ []int64, epoca, seq int64,
) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.epoca, c.numero = epoca, seq
	return nil
}

func (c *capturaSeq) ultimo() (int64, int64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.epoca, c.numero
}
