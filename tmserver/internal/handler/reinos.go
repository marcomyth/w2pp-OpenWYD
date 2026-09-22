package handler

import (
	"fmt"

	"github.com/jeanluca/w2pp-openwyd/internal/droprule"
	"github.com/jeanluca/w2pp-openwyd/internal/reinos"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

// Os Reinos de Hekalotia e Akelonia como quest de invasão (Atlas de Quests,
// área "Reinos e Reis", 17/09/2026; docs/reinos.md). Este arquivo tem:
//
//   - as regras de golpe: a capa decide o lado (internal/reinos.ClanDaCapa),
//     quem veste a capa de um reino não fere os monstros dele, e quem não é de
//     reino nenhum e bate num monstro de um reino vira Inimigo daquele reino por
//     10 minutos renovados a cada golpe (world/reinos.go, FindEnemyFromView);
//   - monstro do Reino não dá XP;
//   - o papel de cada template (tropa, elite, Escolta do Trono, Rei), que decide
//     o pacote do saque;
//   - a absorção do Rei Azul, o renascimento de 4 horas dos Reis e da Escolta e
//     os avisos da invasão.
//
// A força e o visual moram nos templates (Release/TMsrv/run/npc) e a chance de
// cada item do saque na Mesa de Drops (migração 0074).

// msgInimigoDoReino é o aviso de quando a marca liga. Cabe nos 94 bytes do painel.
func msgInimigoDoReino(clan uint8) string {
	return "Você agora é inimigo de " + reinos.Nome(clan) + ". Os guardas vão caçar você."
}

// msgProprioReino é o aviso de quem tenta ferir o próprio reino.
const msgProprioReino = "Você não pode atacar o seu próprio reino."

// monstroDoReino diz se mob é um monstro de um dos dois reinos, dos que levam
// golpe: nasceu na cidade dos Reinos, é de clã 7 ou 8 e não é NPC protegido.
func monstroDoReino(mob *world.Entity) bool {
	return mob != nil && !world.IsPlayer(mob.ID) && !mob.NonCombatNPC &&
		mob.Summoner == 0 && reinos.ClanDeReino(mob.Clan) &&
		reinos.Contem(int(mob.SpawnX), int(mob.SpawnY))
}

// golpeNoReinoPermitido aplica as regras dos Reinos ao golpe de atacante (um
// jogador, ou o dono de um pet) em alvo. Recusa quem veste a capa do reino do
// alvo; marca como inimigo quem não veste capa de reino nenhum. O aviso vai só
// quando a situação muda para o jogador, não a cada golpe.
func (d *Dispatcher) golpeNoReinoPermitido(w *world.World, atacante, alvo *world.Entity) bool {
	if atacante == nil || !world.IsPlayer(atacante.ID) || !monstroDoReino(alvo) {
		return true
	}
	lado := reinos.ClanDaCapa(atacante.Equip[reinos.SlotDaCapa].Index)
	switch lado {
	case alvo.Clan:
		// Recusa que avisa: um golpe zerado calado parece bug (silent refusals).
		sendClientMessage(w, w.Session(atacante.ID), msgProprioReino)
		return false
	case 0:
		if w.MarcarInimigoDoReino(atacante, alvo.Clan) {
			sendClientMessage(w, w.Session(atacante.ID), msgInimigoDoReino(alvo.Clan))
			d.log.Info("reinos: jogador marcado como inimigo", "personagem", atacante.Name,
				"reino", reinos.Nome(alvo.Clan))
		}
	}
	return true
}

// reinoAwardsExp é falso para os monstros do Reino: a invasão paga em saque,
// não em XP (decisão de 17/09/2026). O zero fica aqui pelo mesmo motivo do Orc
// (casteloOrcAwardsExp): o cmd/exptool regrava o Exp de todo monstro.
func reinoAwardsExp(mob *world.Entity) bool {
	return !monstroDoReino(mob)
}

// papelNoReino é o lugar de um monstro no exército.
type papelNoReino int

const (
	papelNenhum papelNoReino = iota
	papelTropa
	papelElite
	papelEscolta
	papelRei
)

// papelPorTemplate lista os templates do exército, os de Hekalotia e os de
// Akelonia (o mesmo nome com "_" no fim). A Torre Guardiã fica de fora: não muda.
var papelPorTemplate = func() map[string]papelNoReino {
	m := map[string]papelNoReino{}
	for p, nomes := range map[papelNoReino][]string{
		papelTropa:   {"Guarda_do_Rei", "Combatente", "Lanceiro", "Virago", "Bruxa"},
		papelElite:   {"Cav._Real", "Averest", "Feiticeira"},
		papelEscolta: {"Escolta_Real"},
	} {
		for _, n := range nomes {
			m[droprule.Canonical(n)] = p
			m[droprule.Canonical(n+"_")] = p
		}
	}
	m[droprule.Canonical(templateReiHarabard)] = papelRei
	m[droprule.Canonical(templateReiGlantuar)] = papelRei
	return m
}()

const (
	templateReiHarabard = "Rei_Harabard"
	templateReiGlantuar = "Rei_Glantuar"
)

// papelDoMonstro é o papel de mob no exército, ou papelNenhum para quem não é
// monstro do Reino.
func papelDoMonstro(mob *world.Entity) papelNoReino {
	if !monstroDoReino(mob) {
		return papelNenhum
	}
	return papelPorTemplate[droprule.Canonical(mob.TemplateName)]
}

// reiDoReino diz se mob é um dos dois Reis e de qual reino (0 Hekalotia,
// 1 Akelonia). O bloco decide, e não só o template: um Rei levantado por GM
// fora do bloco não dispara os avisos nem o relógio do trono.
func reiDoReino(mob *world.Entity) (int, bool) {
	if !monstroDoReino(mob) {
		return 0, false
	}
	switch mob.GenIndex {
	case kingHarabardGen:
		return 0, true
	case kingGlantuarGen:
		return 1, true
	}
	return 0, false
}

// absorcaoDoReiAzulPct é quanto do golpe o Rei Harabard absorve. Ele é o
// tanque: 6 milhões de vida valem 15 milhões de dano.
const absorcaoDoReiAzulPct = 60

// absorcaoDoRei tira do golpe a parte que o Rei Azul absorve. Todo golpe de
// jogador e de pet passa por absorbBlow, e é de lá que isto é chamado.
func absorcaoDoRei(vitima *world.Entity, dano int) int {
	if dano <= 0 {
		return dano
	}
	if i, ok := reiDoReino(vitima); !ok || i != 0 {
		return dano
	}
	return max(dano-dano*absorcaoDoReiAzulPct/100, 1)
}

// horasDoTrono é quanto os Reis e a Escolta do Trono demoram para voltar.
const horasDoTrono = 4

// tronoDoReino diz se o bloco idx é de um Rei ou da Escolta do Trono. Esses
// blocos não têm período de minuto (NPCGener.txt), então o morto volta pela fila
// individual, e esperaDoRenascimento dá a ele as 4 horas.
func tronoDoReino(idx int) bool {
	return idx == kingHarabardGen || idx == kingGlantuarGen || world.IsEscoltaDoTronoGenerator(idx)
}

// nomeDoRei é o nome do Rei de cada reino, para os avisos.
var nomeDoRei = [2]string{"Harabard", "Glantuar"}

// clanDoIndice é o clã de cada reino pelo índice usado aqui (0 Hekalotia).
var clanDoIndice = [2]uint8{reinos.ClanHekalotia, reinos.ClanAkelonia}

// vigiarReis manda ao reino o aviso de Rei sob ataque quando a vida dele cai
// abaixo de 90%, uma vez por luta. A luta acaba quando o Rei volta à vida cheia
// ou morre. Roda no pulso de seis segundos dos Reinos, e por isso pega o dano de
// qualquer caminho, golpe, pet ou veneno.
func (d *Dispatcher) vigiarReis(w *world.World) {
	var vivo [2]bool
	w.ForEachMob(func(_ int, e *world.Entity) {
		i, ok := reiDoReino(e)
		if !ok || e.Mode == world.MobEmpty || e.HP <= 0 {
			return
		}
		vivo[i] = true
		switch {
		case int64(e.HP)*10 < int64(e.MaxHP)*9:
			if d.reiAvisado[i] {
				return
			}
			d.reiAvisado[i] = true
			texto := fmt.Sprintf("[Reino] O Rei %s está sob ataque!", nomeDoRei[i])
			w.ForEachPlaying(-1, func(s *world.Session, p *world.Entity) {
				if reinos.ClanDaCapa(p.Equip[reinos.SlotDaCapa].Index) == clanDoIndice[i] {
					sendClientMessage(w, s, texto)
				}
			})
			d.log.Info("reinos: rei sob ataque", "rei", nomeDoRei[i], "hp", e.HP, "max", e.MaxHP)
		case e.HP >= e.MaxHP:
			d.reiAvisado[i] = false
		}
	})
	for i := range vivo {
		if !vivo[i] {
			d.reiAvisado[i] = false
		}
	}
}

// avisarQuedaDoRei conta ao servidor inteiro quem derrubou o Rei. O golpe de
// um pet é do dono.
func (d *Dispatcher) avisarQuedaDoRei(w *world.World, matador, rei *world.Entity) {
	i, ok := reiDoReino(rei)
	if !ok {
		return
	}
	if matador != nil && matador.Summoner != 0 {
		matador = w.Entity(matador.Summoner)
	}
	quem := "Um invasor"
	if matador != nil && world.IsPlayer(matador.ID) && matador.Name != "" {
		quem = matador.Name
	}
	broadcastNotice(w, fmt.Sprintf("[Reino] %s derrubou o Rei %s de %s!", quem, nomeDoRei[i], reinos.Nome(clanDoIndice[i])))
	d.reiAvisado[i] = false
	d.log.Info("reinos: rei derrubado", "rei", nomeDoRei[i], "matador", quem)
}

// reinosPacotes é quantas unidades um drop do Reino leva, por papel (desenho de
// 17/09/2026). A Mesa de Drops diz se o item cai e com que chance; ela não
// guarda quantidade, então o pacote mora aqui, como em acampamentoTrollPacks.
// O âmago segue a cor do reino: Andaluz N e Fenrir das Sombras no vermelho,
// Andaluz B e Fenrir no azul.
var reinosPacotes = map[papelNoReino]map[int16]int{
	papelTropa: {
		reinoAmagoAndaluzN: 5, reinoAmagoAndaluzB: 5,
		reinoClasseC: 5,
	},
	papelElite: {
		reinoAmagoAndaluzN: 10, reinoAmagoAndaluzB: 10,
		reinoClasseB: 10,
	},
	papelEscolta: {
		reinoAmagoFenrir: 5, reinoAmagoFenrirSombras: 5,
		reinoClasseA: 10,
	},
	papelRei: {
		reinoAmagoFenrir: 10, reinoAmagoFenrirSombras: 10,
		reinoMoeda5Mi:  10,
		jeffiPoeiraOri: 30, jeffiPoeiraLac: 15,
		reinoClasseA: 20,
	},
}

const (
	reinoAmagoAndaluzN      int16 = 2400
	reinoAmagoAndaluzB      int16 = 2405
	reinoAmagoFenrir        int16 = 2406
	reinoAmagoFenrirSombras int16 = 2408
	reinoClasseA            int16 = 4016
	reinoClasseB            int16 = 4017
	reinoClasseC            int16 = 4018
	reinoMoeda5Mi           int16 = 4027
)

// reinosFinish dá ao drop de um monstro do Reino o tamanho do pacote e devolve
// quantas cópias entregar. Empilhável sai numa pilha só, com EF_AMOUNT; o que não
// empilha sai em cópias soltas, porque EF_AMOUNT num item que não empilha vira uma
// pilha que o cliente não divide. A Moeda de Prata era o caso solto aqui até
// 20/09/2026; desde que ela entrou na lista de pilha, o Rei paga uma pilha de dez.
func reinosFinish(mob *world.Entity, it *world.Item) int {
	n := reinosPacotes[papelDoMonstro(mob)][it.Index]
	if n <= 1 {
		return 1
	}
	if isSplittable(it.Index) {
		setItemAmount(it, n)
		return 1
	}
	return n
}
