package handler

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/worldevents"
)

// /gm coliseu liga, desliga, configura e força o Coliseu (coliseu.go):
//
//	/gm coliseu ligar | desligar       o interruptor (nasce desligado a cada boot)
//	/gm coliseu estado                 fases, horas e prêmio
//	/gm coliseu iniciar | fim          força o evento de ondas agora / encerra
//	/gm coliseu batalha [0|1|2] | fim  força uma rodada da Batalha Real / encerra
//	/gm coliseu horas <guilda> [novato] as horas do evento de ondas (padrão 20 e 20)
//	/gm coliseu horabatalha <hora>     a hora da Batalha Real (padrão 19)
//	/gm coliseu premio <item>          o prêmio da Batalha Real; 0 a desliga
//
// As horas e o prêmio são os comandos guildhour, newbiehour e britem do legado
// (imple.cpp:811-826, :939). O forçado corre no relógio do GM e passa pelas
// mesmas transições do horário: os mesmos portões, ondas, avisos e limites.

const usoGMColiseu = "uso: /gm coliseu <ligar|desligar|estado|iniciar|fim|batalha [0-2]|batalha fim|horas <guilda> [novato]|horabatalha <hora>|premio <item>>"

func (d *Dispatcher) gmColiseu(w *world.World, s *world.Session, rest string) {
	op := strings.ToLower(firstToken(rest))
	args := strings.Fields(strings.TrimSpace(strings.TrimPrefix(rest, firstToken(rest))))
	c := &d.coliseu
	switch op {
	case "ligar", "on":
		if !d.ligarColiseu(w) {
			sendClientMessage(w, s, "O Coliseu já está ligado.")
			return
		}
	case "desligar", "off":
		if !d.desligarColiseu(w) {
			sendClientMessage(w, s, "O Coliseu já está desligado.")
			return
		}
	case "estado", "status", "":
	case "iniciar", "abrir", "start":
		if !c.ligado {
			sendClientMessage(w, s, "O Coliseu está desligado; use /gm coliseu ligar.")
			return
		}
		if !c.ondas.Forcar(d.now()) {
			sendClientMessage(w, s, "O evento de ondas já está em andamento.")
			return
		}
		// O primeiro passo sai agora, sem esperar o relógio de 12 s.
		d.passoDoColiseu(w, d.now())
	case "fim", "encerrar", "end":
		if c.ondas.Fase() == worldevents.ColiseuParado {
			sendClientMessage(w, s, "Não há evento de ondas em andamento.")
			return
		}
		d.aplicaPassoDoColiseu(w, c.ondas.Encerrar())
	case "batalha", "br":
		if !d.gmColiseuBatalha(w, s, args) {
			return
		}
	case "horas", "hora":
		g, ok := gmHora(args, 0)
		if !ok {
			sendClientMessage(w, s, "uso: /gm coliseu horas <guilda 0-23> [novato 0-23]")
			return
		}
		n := g
		if len(args) > 1 {
			if n, ok = gmHora(args, 1); !ok {
				sendClientMessage(w, s, "uso: /gm coliseu horas <guilda 0-23> [novato 0-23]")
				return
			}
		}
		c.horaGuilda, c.horaNovato = g, n
	case "horabatalha", "horabr":
		h, ok := gmHora(args, 0)
		if !ok {
			sendClientMessage(w, s, "uso: /gm coliseu horabatalha <0-23>")
			return
		}
		c.horaBatalha = h
	case "premio", "prêmio", "britem":
		if len(args) == 0 {
			sendClientMessage(w, s, "uso: /gm coliseu premio <item> (0 desliga a Batalha Real)")
			return
		}
		n, err := strconv.Atoi(args[0])
		if err != nil || n < 0 || n >= maxItemList {
			sendClientMessage(w, s, fmt.Sprintf("item inválido (0 a %d)", maxItemList-1))
			return
		}
		c.itemBatalha = int16(n)
		// Sem prêmio não há Batalha Real (o legado embrulha tudo em BRItem > 0).
		// Uma rodada em curso acaba aqui, e a entrada que ela trancou reabre.
		if n == 0 && c.batalha.Encerrar() && c.ligado {
			d.portoesDoColiseu(w, false, world.StateOpen)
		}
		d.atualizaAnonimato(w)
	default:
		sendClientMessage(w, s, usoGMColiseu)
		return
	}
	d.log.Info("gm coliseu", "account", s.AccountName, "op", op, "args", args)
	sendClientMessage(w, s, d.estadoDoColiseuTexto())
}

// gmColiseuBatalha força ou encerra uma rodada da Batalha Real. Devolve false
// quando já respondeu ao GM com uma recusa.
func (d *Dispatcher) gmColiseuBatalha(w *world.World, s *world.Session, args []string) bool {
	c := &d.coliseu
	if len(args) > 0 && strings.EqualFold(args[0], "fim") {
		if c.batalha.Fase() == worldevents.BatalhaParada {
			sendClientMessage(w, s, "Não há Batalha Real em andamento.")
			return false
		}
		if c.batalha.Encerrar() {
			d.portoesDoColiseu(w, false, world.StateOpen)
		}
		d.atualizaAnonimato(w)
		return true
	}
	if !c.ligado {
		sendClientMessage(w, s, "O Coliseu está desligado; use /gm coliseu ligar.")
		return false
	}
	if c.itemBatalha <= 0 {
		sendClientMessage(w, s, "A Batalha Real precisa de um prêmio: /gm coliseu premio <item>.")
		return false
	}
	grade := 0
	if len(args) > 0 {
		n, err := strconv.Atoi(args[0])
		if err != nil || n < 0 || n > 2 {
			sendClientMessage(w, s, "rodada inválida: 0 (Nv < 100), 1 (Nv < 200) ou 2 (qualquer nível)")
			return false
		}
		grade = n
	}
	if !c.batalha.Forcar(d.now(), grade) {
		sendClientMessage(w, s, "A Batalha Real já está em andamento.")
		return false
	}
	d.passoDaBatalha(w, d.now())
	return true
}

// gmHora lê a hora na posição i dos argumentos.
func gmHora(args []string, i int) (int, bool) {
	if i >= len(args) {
		return 0, false
	}
	h, err := strconv.Atoi(args[i])
	if err != nil || h < 0 || h > 23 {
		return 0, false
	}
	return h, true
}

// estadoDoColiseuTexto é a linha de /gm coliseu estado.
func (d *Dispatcher) estadoDoColiseuTexto() string {
	c := &d.coliseu
	if !c.ligado {
		return "Coliseu: desligado."
	}
	ondas := fmt.Sprintf("ondas às %02d:00 (novato %02d:00), fase %d", c.horaGuilda, c.horaNovato, c.ondas.Fase())
	if c.ondas.Forcado() {
		ondas += " (GM)"
	}
	if c.ondas.Limite150() {
		ondas += ", limite 150 ligado"
	}
	batalha := "Batalha Real sem prêmio (desligada)"
	if c.itemBatalha > 0 {
		batalha = fmt.Sprintf("Batalha Real às %02d:00, prêmio %d (%s), fase %d, rodada %d",
			c.horaBatalha, c.itemBatalha, d.itemName(c.itemBatalha), c.batalha.Fase(), c.grade)
		if c.batalha.Forcada() {
			batalha += " (GM)"
		}
	}
	return "Coliseu: ligado; " + ondas + "; " + batalha + "."
}
