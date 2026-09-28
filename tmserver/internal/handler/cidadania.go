package handler

import (
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

// A CIDADANIA DA KIBITA (_MSG_Quest.cpp:2431-2461).
//
// POR QUE ELA CHEGOU AGORA: nada no servidor escrevia e.Citizen, e criar guilda
// EXIGE cidadania (guild.go:170, fiel a _MSG_MessageWhisper.cpp:209). Resultado:
// ninguém conseguia criar guilda, em nível nenhum, com ouro nenhum — e o Painel de
// Guilda que subiu antes disso oferecia "Criar sua Guild" num botão que sempre
// recusava. A Hanna esbarrou nisso em produção.
//
// O QUE A CIDADANIA É: não é um sim/não. O valor guardado é QUAL SERVIDOR —
// ServerIndex + 1 —, então ela é por canal, e quem é cidadão de um canal não é de
// outro. O "+1" é o que deixa o zero significar "nenhuma".
//
// CUSTO: 4.000.000 de ouro, o número do legado (:2433), confirmado pela Hanna em
// 24/09/2026 em vez de trocado.
//
// SEM REQUISITO DE NÍVEL, e isso não é esquecimento: o ramo da cidadania não olha
// nível nenhum. Quem olha é o ramo da Alma, do mesmo NPC, que exige 369.
//
// PERMANENTE, sem prazo e sem expiração. A única saída é o comando que a zera.
const cidadaniaCusto = 4_000_000

// As três frases, e duas delas são do próprio legado — achadas no Language.txt em
// vez de inventadas:
//
//	427 _DD_JOINTOWNPEP        "Você se cadastrou neste servidor."
//	513 _DN_NO_TOWNSPEOPLE     "Você não possui cidadania."
//	514 _DN_ANOTHER_TOWNSPEOPLE "Você já possui cidadania em outro servidor."
//
// A 514 existe no legado e NUNCA É USADA por este ramo: quem já é cidadão cai
// calado no código da Alma. A Hanna pediu que essa pessoa recebesse uma resposta,
// e a resposta que o jogo já tinha escrita é justamente esta.
const (
	msgCidadaniaFeita     = "Você se cadastrou neste servidor."
	msgCidadaniaNenhuma   = "Você não possui cidadania."
	msgCidadaniaDeOutro   = "Você já possui cidadania em outro servidor."
	msgCidadaniaTirada    = "Você deixou de ser cidadão deste servidor."
	msgCidadaniaDesteAqui = "Você já é cidadão deste servidor."
)

// kibita atende o NPC 74, e a ORDEM AQUI É A REGRA.
//
// O legado tem UM case KIBITA que tenta a cidadania primeiro e, quando a condição
// falha, DEIXA A EXECUÇÃO CAIR no código da Alma logo abaixo (:2463 em diante). Não
// há campo no pacote que diga qual dos dois serviços o jogador quis: é sequência, e
// a cidadania tem precedência.
//
// É por isso que o pedido de cidadania "caía no ramo da Alma" no nosso servidor.
// Não era o despacho errado — era o primeiro ramo não existir.
func (d *Dispatcher) kibita(w *world.World, s *world.Session, e *world.Entity) {
	if d.comprouCidadania(w, s, e) {
		return
	}
	// A ALMA CONTINUA SENDO TENTADA, e isto é o que salva a fidelidade: no legado,
	// quem não pode comprar a cidadania NÃO é atendido por ela — a execução escorrega
	// para a Alma. Um Mortal 369 com a pedra e menos de 4 milhões de ouro tem direito
	// à Alma, e eu quase tirei isso dele: a primeira versão desta função tratava "sem
	// ouro" como pedido atendido e engolia a Alma inteira. O teste da Alma, cujo
	// personagem tem zero de ouro, foi quem me mostrou.
	if d.kibitaSoul(w, s, e) {
		return
	}
	// E SÓ AQUI A CIDADANIA EXPLICA POR QUE NÃO ACONTECEU: quando nem ela nem a Alma
	// fizeram nada. É a divergência que a Hanna pediu — no legado este clique é
	// silencioso — colocada no único lugar onde ela não atrapalha: quem levou a Alma
	// não precisa ouvir sobre ouro que não gastou.
	d.explicaCidadania(w, s, e)
}

// comprouCidadania vende a cidadania, e só devolve true quando a venda ACONTECEU.
//
// Devolver false nas recusas é o que mantém a sequência do legado: a Alma vem depois.
func (d *Dispatcher) comprouCidadania(w *world.World, s *world.Session, e *world.Entity) bool {
	if e.Citizen != 0 || e.Coin < cidadaniaCusto {
		return false
	}
	minha := uint8(d.serverIndex + 1)
	e.Coin -= cidadaniaCusto
	e.Citizen = minha
	d.sendEtc(w, s, e)
	sendClientMessage(w, s, msgCidadaniaFeita)
	w.SaveCharacterAsync(s)

	// A CIDADANIA DA GUILDA (:2445-2456) NÃO ENTRA NESTE PASSO, e é de propósito.
	//
	// No legado, um LÍDER que compra cidadania grava a dele também em
	// GuildInfo[Guild].Citizen e empurra ao DBServer. Aqui isso precisa de um caminho
	// de gravação que não existe: não há RPC que escreva guild.citizen, e a coluna só
	// é preenchida na CRIAÇÃO da guilda (store/guild.go:70, a partir da cidadania do
	// líder).
	//
	// O que fica de fora, exatamente: um líder que já tinha guilda e compra cidadania
	// DEPOIS não passa a cidadania para a guilda. Quem CRIA guilda a partir de agora
	// está coberto, porque a criação copia a do líder — e é esse o caminho que a Hanna
	// precisa hoje.
	//
	// Registrado, e não escondido: inventar meia gravação aqui deixaria memória e
	// banco discordando, que é pior do que não gravar.
	d.log.Info("cidadania comprada", "conn", s.Conn, "name", e.Name,
		"servidor", minha, "custo", cidadaniaCusto, "ouro_restante", e.Coin,
		"lider_de_guilda", e.Guild != 0 && e.GuildLevel == guildLeaderLevel)
	return true
}

// explicaCidadania diz por que a compra não aconteceu, para o clique não ser mudo.
func (d *Dispatcher) explicaCidadania(w *world.World, s *world.Session, e *world.Entity) {
	switch {
	case e.Citizen == uint8(d.serverIndex+1):
		sendClientMessage(w, s, msgCidadaniaDesteAqui)
	case e.Citizen != 0:
		// A frase 514 do legado, que existe no Language.txt e nenhum ramo usava.
		sendClientMessage(w, s, msgCidadaniaDeOutro)
	default:
		sendClientMessage(w, s, combineNeedsGold(cidadaniaCusto))
	}
}

// tirarCidadania atende /tirarcidadania.
//
// É o /getout do legado (_MSG_MessageWhisper.cpp:57-64) com o nome que a Hanna
// escolheu. Zera e avisa com a mesma frase que o legado usa ali.
//
// ELE EXISTE PARA A CIDADANIA NÃO SER PORTA DE MÃO ÚNICA: como o valor é o canal e
// a compra exige Citizen == 0, sem um jeito de zerar ninguém troca de servidor
// nunca. E ele NÃO devolve o ouro, igual ao legado — quem sai paga de novo para
// voltar.
func (d *Dispatcher) tirarCidadania(w *world.World, s *world.Session) {
	e := w.Entity(s.Conn)
	if e == nil || s.Mode != world.UserPlay {
		return
	}
	if e.Citizen == 0 {
		sendClientMessage(w, s, msgCidadaniaNenhuma)
		return
	}
	anterior := e.Citizen
	e.Citizen = 0
	d.sendEtc(w, s, e)
	sendClientMessage(w, s, msgCidadaniaTirada)
	w.SaveCharacterAsync(s)
	d.log.Info("cidadania tirada", "conn", s.Conn, "name", e.Name, "era", anterior)
}
