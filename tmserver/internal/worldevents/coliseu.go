package worldevents

import "time"

// O Coliseu do legado são duas máquinas de relógio na mesma arena
// (2604-2648 × 1708-1744), ambas no GuildProcess, que o ProcessMinTimer chama a
// cada 12 s (Server.cpp:6759, ProcessSecMinTimer.cpp:2822):
//
//   - o evento de ondas, ColoState (Server.cpp:6942-7057), na hora de guilda e
//     na hora de novato;
//   - a Batalha Real, BrState (Server.cpp:6842-6939), três rodadas por hora na
//     hora dela, só quando há um item de prêmio configurado.
//
// Aqui fica só a decisão: que passo cabe a este relógio. Portões, monstros,
// avisos e teleportes são do Dispatcher (handler/coliseu.go).

// ColiseuFase é o ColoState do legado, renomeado pelo que cada valor quer dizer.
// A ordem muda num ponto: o legado nasce em 8 (Server.cpp:661) e zera para 0 nos
// minutos 0-2 da hora do evento. Aqui o "parado" é o zero, para que o valor zero
// da struct seja o estado de boot.
type ColiseuFase uint8

const (
	ColiseuParado          ColiseuFase = iota // ColoState 8: fora do evento
	ColiseuPronto                             // 0: zerado nos minutos 0-2, espera o minuto 3
	ColiseuEntradaFechada                     // 1: minuto 3, entrada trancada
	ColiseuOnda1                              // 2: minuto 4
	ColiseuInternosAbertos                    // 3: minuto 5
	ColiseuOnda2                              // 4: minuto 7
	ColiseuOnda3                              // 5: minuto 9
	ColiseuOnda4                              // 6: minuto 11
	ColiseuOnda5                              // 7: minuto 13
)

// Blocos do NPCGener que o evento solta (GenerateColoseum). Na hora de guilda
// são os Ciclopes, na de novato os Orcs; as posições 0-2 de cada trio casam uma
// com a outra (Server.cpp:6981-7044).
var (
	ColiseuBlocosGuilda = [3]int{0, 1, 2}
	ColiseuBlocosNovato = [3]int{5, 6, 7}
)

// ColiseuPasso é o que um passo do relógio manda fazer, na ordem em que o legado
// faz.
type ColiseuPasso struct {
	// Zera: minutos 0-2, fase Parado → Pronto. O legado também zera TaxChanged
	// aqui (Server.cpp:6962); o port controla a troca de imposto por dia em
	// guild.go (taxChangedAt), então não há o que zerar.
	Zera bool
	// FechaEntrada: SetColoseumDoor(3), os dois portões de fora.
	FechaEntrada bool
	// LimiteNovato: é a hora de novato — liga o Colo150Limit e expulsa quem
	// está na arena com nível 150 a 400 (Server.cpp:7052-7057).
	LimiteNovato bool
	// Ondas: um GenerateColoseum por bloco, nesta ordem.
	Ondas []int
	// AbreInternos: SetColoseumDoor2(1), os cinco portões de dentro.
	AbreInternos bool
	// Fim: minuto 15. Abre a entrada, tranca os de dentro, apaga os monstros que
	// sobraram e desliga o limite (Server.cpp:6968-6974).
	Fim bool
}

// Vazio diz se o passo não manda nada.
func (p ColiseuPasso) Vazio() bool {
	return !p.Zera && !p.FechaEntrada && !p.LimiteNovato && len(p.Ondas) == 0 && !p.AbreInternos && !p.Fim
}

// Coliseu guarda o ColoState e o Colo150Limit.
type Coliseu struct {
	fase ColiseuFase
	// limite150 é o Colo150Limit: enquanto ligado, quem anda na arena com nível
	// 150 ou mais volta para a cidade (_MSG_Action.cpp:326-337).
	limite150 bool

	// Um evento forçado pelo GM corre no relógio dele, a partir do minuto 3 da
	// hora de guilda, e não espera as 20h. Acaba quando volta a Parado.
	forcado bool
	inicio  time.Time
}

// Fase devolve o ColoState atual.
func (c *Coliseu) Fase() ColiseuFase { return c.fase }

// Limite150 diz se o limite de nível 150 da hora de novato está ligado.
func (c *Coliseu) Limite150() bool { return c.limite150 }

// Forcado diz se o evento em curso foi aberto pelo GM.
func (c *Coliseu) Forcado() bool { return c.forcado }

// Forcar abre um evento agora, no relógio do GM: o próximo passo já tranca a
// entrada, como no minuto 3. Devolve false se já há um em curso.
func (c *Coliseu) Forcar(now time.Time) bool {
	if c.fase != ColiseuParado {
		return false
	}
	c.fase, c.forcado, c.inicio = ColiseuPronto, true, now
	return true
}

// Encerrar leva o evento ao fim agora. Devolve o passo de fim, ou um passo
// vazio quando não há evento.
func (c *Coliseu) Encerrar() ColiseuPasso {
	if c.fase <= ColiseuPronto {
		c.fase, c.forcado, c.limite150 = ColiseuParado, false, false
		return ColiseuPasso{}
	}
	c.fase, c.forcado, c.limite150 = ColiseuParado, false, false
	return ColiseuPasso{Fim: true}
}

// Relogio devolve a hora e o minuto que o passo deve ler: o relógio de parede,
// ou, num evento forçado, a hora de guilda com o minuto contado desde o início a
// partir do 3.
func (c *Coliseu) Relogio(now time.Time, horaGuilda int) (hora, minuto int) {
	if !c.forcado {
		return now.Hour(), now.Minute()
	}
	return horaGuilda, 3 + int(now.Sub(c.inicio)/time.Minute)
}

// Passo é o encadeamento de ifs de Server.cpp:6942-7057, uma transição por
// chamada. Na hora de guilda o evento solta os Ciclopes; em qualquer outra das
// duas horas, os Orcs. O legado vem com as duas horas em 20 (Server.cpp:642 e
// :660), e então testa a de guilda primeiro: os Orcs nunca saem, mas o limite
// da hora de novato vale mesmo assim.
func (c *Coliseu) Passo(hora, minuto, horaGuilda, horaNovato int) ColiseuPasso {
	naHora := hora == horaGuilda || hora == horaNovato
	if !naHora {
		return ColiseuPasso{}
	}
	blocos := ColiseuBlocosNovato
	if hora == horaGuilda {
		blocos = ColiseuBlocosGuilda
	}
	var p ColiseuPasso
	switch {
	case minuto >= 3 && c.fase == ColiseuPronto:
		p.FechaEntrada = true
		c.fase = ColiseuEntradaFechada
		if hora == horaNovato {
			c.limite150 = true
			p.LimiteNovato = true
		}
	case minuto >= 4 && c.fase == ColiseuEntradaFechada:
		p.Ondas = []int{blocos[0]}
		c.fase = ColiseuOnda1
	case minuto >= 5 && c.fase == ColiseuOnda1:
		p.AbreInternos = true
		c.fase = ColiseuInternosAbertos
	case minuto >= 7 && c.fase == ColiseuInternosAbertos:
		p.Ondas = []int{blocos[1]}
		c.fase = ColiseuOnda2
	case minuto >= 9 && c.fase == ColiseuOnda2:
		p.Ondas = []int{blocos[0], blocos[1]}
		c.fase = ColiseuOnda3
	case minuto >= 11 && c.fase == ColiseuOnda3:
		p.Ondas = []int{blocos[1], blocos[2]}
		c.fase = ColiseuOnda4
	case minuto >= 13 && c.fase == ColiseuOnda4:
		p.Ondas = []int{blocos[1], blocos[2]}
		c.fase = ColiseuOnda5
	case minuto >= 15 && c.fase == ColiseuOnda5:
		p.Fim = true
		c.fase, c.forcado, c.limite150 = ColiseuParado, false, false
	case minuto < 3 && c.fase == ColiseuParado:
		p.Zera = true
		c.fase, c.limite150 = ColiseuPronto, false
	}
	return p
}

// BatalhaFase é o BrState do legado, com os mesmos números.
type BatalhaFase uint8

const (
	BatalhaParada   BatalhaFase = iota // 0
	BatalhaPronta                      // 1: minuto 0 da rodada, aviso "inicia em 2min"
	BatalhaEmLuta                      // 2: minuto 5, entrada trancada
	BatalhaPremiada                    // 3: minuto 13, prêmio no altar
)

// BatalhaMinutos é o tamanho de uma rodada: a hora se divide em três
// (BrGrid = minuto / 20, BrMod = minuto % 20, Server.cpp:6845-6848).
const BatalhaMinutos = 20

// BatalhaPasso é o que um passo da Batalha Real manda fazer. Grade é a rodada
// da hora (0, 1 ou 2), que decide o nível que entra e os textos.
type BatalhaPasso struct {
	Prepara, Inicia, Premia, Reabre bool
	Grade                           int
}

// Vazio diz se o passo não manda nada.
func (p BatalhaPasso) Vazio() bool { return !p.Prepara && !p.Inicia && !p.Premia && !p.Reabre }

// Batalha guarda o BrState.
type Batalha struct {
	fase BatalhaFase

	// A rodada forçada pelo GM corre como se fosse o minuto 0 da grade pedida,
	// na hora da Batalha Real. Acaba quando volta a Parada.
	forcada bool
	grade   int
	inicio  time.Time
}

// Fase devolve o BrState atual.
func (b *Batalha) Fase() BatalhaFase { return b.fase }

// Forcada diz se a rodada em curso foi aberta pelo GM.
func (b *Batalha) Forcada() bool { return b.forcada }

// Forcar abre uma rodada agora, na grade dada (0, 1 ou 2). Devolve false se
// já há uma em curso ou a grade não existe.
func (b *Batalha) Forcar(now time.Time, grade int) bool {
	if b.fase != BatalhaParada || grade < 0 || grade > 2 {
		return false
	}
	b.forcada, b.grade, b.inicio = true, grade, now
	return true
}

// Encerrar leva a rodada ao fim agora e diz se a entrada precisa reabrir (ela
// fica trancada da Luta até o prêmio).
func (b *Batalha) Encerrar() bool {
	reabre := b.fase == BatalhaEmLuta || b.fase == BatalhaPremiada
	b.fase, b.forcada = BatalhaParada, false
	return reabre
}

// Relogio devolve a hora e o minuto que o passo deve ler: o de parede, ou o da
// rodada forçada, que começa no minuto 0 da grade pedida.
func (b *Batalha) Relogio(now time.Time, horaBatalha int) (hora, minuto int) {
	if !b.forcada {
		return now.Hour(), now.Minute()
	}
	return horaBatalha, b.grade*BatalhaMinutos + int(now.Sub(b.inicio)/time.Minute)
}

// Passo é o encadeamento de Server.cpp:6849-6939. Os três primeiros testam o
// minuto EXATO da rodada (0, 5 e 13): um relógio que pula esse minuto pula a
// transição, como no legado. O último é >= 14.
func (b *Batalha) Passo(hora, minuto, horaBatalha int) BatalhaPasso {
	grade, mod := minuto/BatalhaMinutos, minuto%BatalhaMinutos
	p := BatalhaPasso{Grade: grade}
	if hora == horaBatalha {
		switch {
		case b.fase == BatalhaParada && mod == 0:
			p.Prepara = true
			b.fase = BatalhaPronta
		case b.fase == BatalhaPronta && mod == 5:
			p.Inicia = true
			b.fase = BatalhaEmLuta
		case b.fase == BatalhaEmLuta && mod == 13:
			p.Premia = true
			b.fase = BatalhaPremiada
		}
		// Este if é separado no legado (Server.cpp:6935): no mesmo passo que
		// premia ele não roda, porque 13 < 14.
		if b.fase == BatalhaPremiada && mod >= 14 {
			p.Reabre = true
			b.fase, b.forcada = BatalhaParada, false
		}
	}
	return p
}
