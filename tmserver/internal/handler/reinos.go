package handler

import (
	"github.com/jeanluca/w2pp-openwyd/internal/reinos"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

// Os Reinos de Hekalotia e Akelonia como quest de invasão (Atlas de Quests,
// área "Reinos e Reis", 17/09/2026). Este arquivo tem as regras de golpe:
//
//   - a capa decide o lado (internal/reinos.ClanDaCapa);
//   - quem veste a capa de um reino não fere os monstros dele;
//   - quem não é de reino nenhum e bate num monstro de um reino vira Inimigo
//     daquele reino por 10 minutos, renovados a cada golpe, e os guardas dele
//     passam a caçá-lo (world/reinos.go, FindEnemyFromView);
//   - monstro do Reino não dá XP.

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
	switch {
	case lado == alvo.Clan:
		// Recusa que avisa: um golpe zerado calado parece bug (silent refusals).
		sendClientMessage(w, w.Session(atacante.ID), msgProprioReino)
		return false
	case lado == 0:
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
