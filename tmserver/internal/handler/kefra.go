package handler

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

// O Kefra é chefe semanal. No original (ProcessSecMinTimer.cpp:808-816), toda
// terça ao meio-dia, se ele tinha morrido, o servidor devolve o Kefra (bloco
// KEFRA_BOSS, 396) e os guardas (KEFRA_MOB_INITIAL..END, 397-400). O original
// rodava no relógio do Brasil; o do contêiner é UTC, então o meio-dia vai em
// UTC: 15:00. O laço do boot do original usa "<" e pula o guarda 400; aqui ele
// volta junto, como na terça.
//
// Fica de fora, de propósito: a morte do Kefra no original liga o KefraLive — a
// XP inteira no servidor todo — até a terça seguinte, e dá +100 de fama à guilda
// que matou (MobKilled.cpp:1451-1492). A chave continua no painel, desligada: a
// Mesa de XP foi calibrada com ela desligada, e o prêmio vai para a guilda, que
// depende da cidadania, ainda não portada. No boot ele nasce sempre: guardar a
// morte exigiria o banco, e sem o prêmio não há o que proteger.

const (
	kefraDia     = time.Tuesday
	kefraHoraUTC = 15
)

// tickKefraSemanal devolve o Kefra e os guardas uma vez por terça, às 15h UTC.
// GenerateMob respeita o teto de população de cada bloco, então quem está vivo
// não nasce de novo.
func (d *Dispatcher) tickKefraSemanal(w *world.World) {
	if d.tickCount%minutoTicks != 0 {
		return
	}
	agora := d.now().UTC()
	if agora.Weekday() != kefraDia || agora.Hour() != kefraHoraUTC {
		return
	}
	dia := agora.Format(time.DateOnly)
	if d.kefraVolta == dia {
		return
	}
	d.kefraVolta = dia
	n := 0
	for idx := world.KefraBossGenIndex; idx <= world.KefraGuardLast; idx++ {
		ids := w.GenerateMob(idx)
		n += len(ids)
		d.revealSpawned(w, ids)
	}
	// A terça também DESLIGA o estado, que no legado é a mesma linha do renascer
	// (ProcessSecMinTimer.cpp:810). Só grava se estava ligado: fora disso toda
	// terça escreveria no banco para repetir o que já estava lá.
	if d.expEvents.KefraLive {
		antesGuilda := d.kefraGuildID
		d.marcaKefra(false, 0)
		d.gravaEstadoDoKefra(w, false, 0, "relogio semanal", true, antesGuilda)
		d.log.Info("kefra semanal: estado desligado, a experiência volta à metade", "guilda_anterior", antesGuilda)
	}
	d.log.Info("kefra semanal: chefe e guardas de volta", "monstros", n, "dia", dia)
}

// O CICLO. A morte do chefe liga o estado do servidor; a terça desliga.
//
// O KefraLive do legado não é um sim ou não: com guilda ele recebe o ID DA GUILDA
// que matou (MobKilled.cpp:1463) e, sem guilda, 1 (1487). O zero é o chefe VIVO —
// o nome da variável diz o contrário do que ela guarda, e essa troca já custou
// tempo aqui. A migração 0067 separa as duas coisas, o interruptor e a guilda, e
// por isso elas andam sempre juntas neste arquivo.

// kefraChefePos é onde o bloco 396 põe o chefe (NPCGener.txt). Fica DENTRO da
// caixa do saque, o que importa: na varredura ele conta a si mesmo.
var kefraChefePos = [2]int16{2367, 3931}

// kefraCaixa é a área que o saque varre: o legado vai de 2335 a 2394 em x e de
// 3896 a 3954 em y (MobKilled.cpp:1494-1496, escritos como `< 2395` e `< 3955`).
var kefraCaixa = areaBox{2335, 3896, 2394, 3954}

const (
	// kefraSorteios é o tamanho do sorteio no Carry do chefe: rand() % 60.
	kefraSorteios = 60
	// kefraFamaPremio é a fama que a guilda leva (MobKilled.cpp:1473).
	kefraFamaPremio = 100
	// kefraGravacaoTentativas é quantas vezes a gravação tenta antes de o
	// processo voltar atrás.
	kefraGravacaoTentativas = 2

	// chaveAvisoDoKefra é _SN_End_Khepra (Language.txt:378). A linha tem um %s: a
	// guilda.
	chaveAvisoDoKefra = "_SN_End_Khepra"
	msgAvisoDoKefra   = "A guilda %s derrotou Kefra!"
	// msgKefraSemGuilda: aqui o legado imprime "CdK", que não diz nada a quem
	// está jogando. Decisão nossa: dizer o fato.
	msgKefraSemGuilda = "O Kefra foi derrotado."
)

// O /kefra tem dois destinos DIFERENTES (_MSG_MessageWhisper.cpp:835-849), e o
// tile do deserto tem um terceiro (GetFunc.cpp:1009). Os três números se parecem e
// não são o mesmo:
//
//	chefe vivo      -> 2365,3884, a zona, com o aviso de que ele precisa cair
//	chefe derrotado -> 3250,1695, a cidade
//	tile do deserto -> 3250,1703, a cidade por outra porta
//
// 1695 NÃO é 1703. Confundir os dois é o erro fácil aqui.
var (
	kefraZonaPos   = [2]int16{2365, 3884}
	kefraCidadePos = [2]int16{3250, 1695}
)

// msgKefraAindaVivo é a linha que o legado escreve à mão, sem passar pelo
// Language.txt (_MSG_MessageWhisper.cpp:839), acentuação inclusa — copiada como
// está para o jogador ler o que sempre leu.
const msgKefraAindaVivo = "O kefra ainda esta vivo, precisa ser derrotado"

// destinoDoComandoKefra diz para onde o /kefra leva e se o jogador ouve o aviso.
// A cidade é o prêmio de ter derrubado o chefe; com ele de pé o comando leva à
// zona, que é onde se luta.
func (d *Dispatcher) destinoDoComandoKefra() (dest [2]int16, avisa bool) {
	if d.expEvents.KefraLive {
		return kefraCidadePos, false
	}
	return kefraZonaPos, true
}

// O piso do deserto, a outra porta da cidade. Irmão do piso do Hall
// (kefra_hall.go): os dois são rota com condição, que consulta de tabela não
// expressa, e por isso nenhum dos dois está na teleportTable.
//
// CUIDADO: os dois pisos têm o MESMO x, 2364, e diferem só no y — 3892 é o Hall e
// 3924 é o deserto. São checagens separadas de propósito; juntá-las numa só abriria
// a cidade a quem pisou no Hall.
const (
	kefraDesertoPisoX = 2364
	kefraDesertoPisoY = 3924
	kefraDesertoDestX = 3250
	kefraDesertoDestY = 1703
)

// noPisoDoDesertoDoKefra diz se a posição está no bloco de 4x4 do piso do deserto.
// O legado arredonda a posição para múltiplo de 4 antes de comparar
// (`xv = (*x) & 0xFFFC`, GetFunc.cpp:784-785), então é bloco e não casa.
func noPisoDoDesertoDoKefra(x, y int16) bool {
	return x&^3 == kefraDesertoPisoX && y&^3 == kefraDesertoPisoY
}

// destinoDoDesertoDoKefra sorteia onde o jogador cai, com o espalhamento de três
// casas de toda rota do legado (`3250 + rand() % 3`, GetFunc.cpp:1009-1010).
func destinoDoDesertoDoKefra(intn func(int) int) (int16, int16) {
	return kefraDesertoDestX + int16(intn(3)), kefraDesertoDestY + int16(intn(3))
}

// entraNoDesertoDoKefra é o piso do deserto. Devolve true quando o piso é dele — aí
// o chamador não procura mais rota nenhuma.
//
// A condição do legado é `KefraLive != 0` (GetFunc.cpp:1007), e vale lembrar que o
// nome mente: essa variável guarda a MORTE do chefe, então a rota abre com ele
// DERROTADO. Com o chefe de pé, pisar não faz nada e não diz nada, porque no
// original a posição simplesmente não casa com rota alguma.
//
// LOOP ONLY.
func (d *Dispatcher) entraNoDesertoDoKefra(w *world.World, s *world.Session, e *world.Entity) bool {
	if !noPisoDoDesertoDoKefra(e.X, e.Y) {
		return false
	}
	if !d.expEvents.KefraLive {
		return true // chefe vivo: o piso é nosso, e não leva a lugar nenhum
	}
	x, y := destinoDoDesertoDoKefra(w.Rand().Intn)
	d.doTeleport(w, s, x, y)
	return true
}

// kefraClanConta diz se a morte conta para quem bateu. O legado trata Clan fora
// de {0,4,7,8} noutro ramo, que só REMOVE o monstro e nem chega ao bloco do Kefra
// (MobKilled.cpp:1437-1438): sem estado, sem fama, sem aviso e sem saque.
func kefraClanConta(clan uint8) bool {
	return clan == 0 || clan == 4 || clan == 7 || clan == 8
}

// marcaKefra aplica o estado DENTRO do laço. É o que a conta de XP lê
// (level.ExpEvents.KefraLive) e o que o acesso à cidade vai consultar.
func (d *Dispatcher) marcaKefra(derrotado bool, guilda int32) {
	d.expEvents.KefraLive = derrotado
	d.kefraGuildID = guilda
}

// kefraKilled é a morte do chefe. Roda ANTES do laço de drop, que é a ordem do
// legado, e por isso o chefe ainda está na grade quando o saque varre a caixa.
//
// LOOP ONLY.
func (d *Dispatcher) kefraKilled(w *world.World, matador, mob *world.Entity) {
	if mob == nil || matador == nil || int(mob.GenIndex) != world.KefraBossGenIndex {
		return
	}
	if !kefraClanConta(matador.Clan) {
		d.log.Info("kefra morto por clan que não conta", "clan", matador.Clan, "personagem", matador.Name)
		return
	}
	antesLive, antesGuilda := d.expEvents.KefraLive, d.kefraGuildID
	guilda := matador.Guild
	// O estado vale JÁ. No legado a XP muda no golpe seguinte, não na próxima
	// sondagem de versão; a gravação no banco vem atrás, só confirmando.
	d.marcaKefra(true, int32(guilda))
	if guilda != 0 {
		info, _ := w.GuildInfo(guilda)
		nova := info.Fame + kefraFamaPremio
		w.SetGuildFame(guilda, nova)
		d.persistGuildFame(w, guilda, nova) // só a memória sumia no próximo boot
		// O nome ATUAL da guilda, não o que ela tinha na morte: o legado busca o
		// nome (1461) e então imprime a global KefraKiller (1479), que é um
		// descuido dele.
		broadcastNotice(w, d.avisoDoKefra(d.guildLabel(w, guilda)))
	} else {
		broadcastNotice(w, msgKefraSemGuilda)
	}
	d.saqueDoKefra(w, matador, mob)
	d.gravaEstadoDoKefra(w, true, int32(guilda), matador.Name, antesLive, antesGuilda)
	d.log.Info("kefra derrotado", "guilda", guilda, "personagem", matador.Name)
}

// tickKefraGuardas devolve os guardas ENQUANTO o chefe está de pé. Eles são o anel
// que protege o chefe, e um anel que só se refizesse na terça deixaria o chefe
// sozinho pelo resto da semana.
//
// DIVERGÊNCIA, pequena e de propósito: o legado repõe o guarda NO INSTANTE em que
// ele morre, e só o bloco que morreu — `if (GenerateIndex >= KEFRA_MOB_INITIAL &&
// GenerateIndex <= KEFRA_MOB_END && KefraLive == 0) GenerateMob(GenerateIndex)`
// (MobKilled.cpp:2989-2990). Aqui a volta acontece no tique seguinte e varre os
// quatro blocos. O efeito em jogo é o mesmo anel de volta em menos de um segundo,
// e o preço é não pendurar mais um gancho no caminho da morte. Quem ler isto e
// quiser fidelidade ao instante, o gancho é o mobKilled. A condição de o chefe
// estar vivo é fiel: é o `KefraLive == 0` da mesma linha.
//
// Roda a cada tique, e não de minuto em minuto, porque aqui não se lê relógio de
// parede, se olha população: pela régua do mintimer.go isto CONTA, não CONSULTA.
// GenerateMob respeita o teto de cada bloco, então bloco cheio não faz nada.
//
// LOOP ONLY.
func (d *Dispatcher) tickKefraGuardas(w *world.World) {
	chefe := w.GeneratorAt(world.KefraBossGenIndex)
	if chefe == nil || chefe.CurrentNumMob == 0 {
		return // chefe derrotado: a área fica limpa até a terça
	}
	n := 0
	for idx := world.KefraBossGenIndex + 1; idx <= world.KefraGuardLast; idx++ {
		ids := w.GenerateMob(idx)
		n += len(ids)
		d.revealSpawned(w, ids)
	}
	if n > 0 {
		d.log.Info("guardas do kefra de volta", "monstros", n)
	}
}

// ApplyKefraStateBoot tira o chefe e os guardas do mundo quando o banco diz que
// ele já foi derrotado nesta semana.
//
// Existe por causa da ordem do boot: o povoamento levanta todos os blocos do
// NPCGener ANTES de a configuração do portal chegar (main.go), então o certo é um
// passo DEPOIS, que remove o que o povoamento levantou — o mesmo formato do
// ApplyGeneratorOffBoot, que existe pela mesma razão.
//
// Leitura de configuração que falhou deixa o estado no padrão, que é o chefe
// VIVO, e aí este passo não tira nada. É a escolha segura: um chefe a mais é só um
// chefe, enquanto uma área vazia por engano tranca a cidade do Kefra e, com ela, o
// destrave do Celestial 40 e 90.
func (d *Dispatcher) ApplyKefraStateBoot(w *world.World) {
	if !d.expEvents.KefraLive {
		return
	}
	for idx := world.KefraBossGenIndex; idx <= world.KefraGuardLast; idx++ {
		w.ClearGenerator(idx)
	}
	d.log.Info("kefra derrotado no banco: chefe e guardas fora do mundo no boot", "guilda", d.kefraGuildID)
}

// avisoDoKefra usa a linha do Language.txt quando ela tem exatamente o %s de que
// precisa; qualquer outro formato cairia num Sprintf errado na cara do jogador.
func (d *Dispatcher) avisoDoKefra(guilda string) string {
	texto := msgAvisoDoKefra
	if t, ok := d.lang.Text(chaveAvisoDoKefra); ok && strings.Count(t, "%") == 1 && strings.Contains(t, "%s") {
		texto = t
	}
	return fmt.Sprintf(texto, guilda)
}

// saqueDoKefra dá a quem matou UM sorteio no Carry do chefe por monstro de pé na
// caixa (MobKilled.cpp:1494-1509). Bolsa cheia perde o item, como o PutItem do
// legado — mas registra, porque item que desaparece sem rastro ninguém investiga.
//
// O CHEFE CONTA A SI MESMO, e isso não é descuido: ele nasce dentro da própria
// caixa (kefraChefePos) e ainda está na grade quando esta varredura roda, porque
// quem o retira é o despawn no FIM do mobKilled. Ou seja, matar o chefe numa área
// vazia ainda rende um sorteio. Está fixado em teste de propósito: é o tipo de
// coisa que alguém "conserta" seis meses depois sem saber que era assim no
// original.
func (d *Dispatcher) saqueDoKefra(w *world.World, matador, mob *world.Entity) {
	entregues, perdidos := 0, 0
	w.ForEachMob(func(_ int, e *world.Entity) {
		if !kefraCaixa.contains(e.X, e.Y) {
			return
		}
		it := mob.Carry[w.Rand().Intn(kefraSorteios)]
		if it.Empty() {
			return
		}
		if d.putCarryItem(w, matador, it) < 0 {
			perdidos++
			return
		}
		entregues++
	})
	if perdidos > 0 {
		d.log.Warn("saque do kefra: bolsa cheia, itens perdidos",
			"perdidos", perdidos, "entregues", entregues, "personagem", matador.Name)
	}
	d.log.Info("saque do kefra", "entregues", entregues, "perdidos", perdidos, "personagem", matador.Name)
}

// gravaEstadoDoKefra confirma no banco o que o processo já aplicou.
//
// Se a gravação falhar de vez, o processo VOLTA ATRÁS e diz que voltou. Sem isso
// o tmserver ficaria "derrotado" e o banco "vivo", e a próxima leitura de
// configuração desfaria a morte em silêncio — divergência muda, que é a pior.
func (d *Dispatcher) gravaEstadoDoKefra(w *world.World, derrotado bool, guilda int32, personagem string, antesLive bool, antesGuilda int32) {
	if d.worldEventSource == nil {
		return
	}
	src, log := d.worldEventSource, d.log
	w.GoDetached(func() func(*world.World) {
		var err error
		for tentativa := 1; tentativa <= kefraGravacaoTentativas; tentativa++ {
			ctx, cancel := context.WithTimeout(context.Background(), worldEventFetchTimeout)
			var versao int64
			versao, err = src.SetKefraState(ctx, derrotado, guilda)
			cancel()
			if err == nil {
				log.Info("estado do kefra gravado", "guilda", guilda, "personagem", personagem, "versao", versao)
				return nil
			}
			log.Error("gravação do estado do kefra falhou",
				"tentativa", tentativa, "de", kefraGravacaoTentativas,
				"guilda", guilda, "personagem", personagem, "err", err)
		}
		return func(*world.World) {
			d.marcaKefra(antesLive, antesGuilda)
			log.Error("estado do kefra REVERTIDO no processo: a gravação não passou",
				"guilda", guilda, "personagem", personagem, "voltou_para_derrotado", antesLive, "err", err)
		}
	})
}
