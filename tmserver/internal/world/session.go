package world

import (
	"net"
	"time"

	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/protocol"
)

// outFrame is a logical S→C message queued to a session's writer goroutine,
// which encodes it (CPSock) just before writing.
type outFrame struct {
	header  protocol.Header
	payload []byte
}

// AccessLevel is a session's GM/moderation privilege tier, derived from the
// account.role string at login (issue #122). It replaces the legacy fragile
// "character Level >= 1000" backdoor with an explicit server-side authority.
// Ordered so a gate can compare with >= (a higher tier passes lower-tier gates).
type AccessLevel uint8

// Access tiers. Player is the zero value: an unknown/absent role is never a GM.
const (
	AccessPlayer    AccessLevel = iota // no GM privilege
	AccessModerator                    // in-game moderation (the first-batch commands)
	AccessAdmin                        // reserved for future destructive/server-wide ops
)

// ParseAccess maps an account.role string ('player'/'moderator'/'admin') to its
// AccessLevel. Any unrecognized value is AccessPlayer (fail closed).
func ParseAccess(role string) AccessLevel {
	switch role {
	case "admin":
		return AccessAdmin
	case "moderator":
		return AccessModerator
	default:
		return AccessPlayer
	}
}

// EhStaff diz se este nível é de gente da casa — moderação ou administração.
//
// Existe como método e não como comparação solta porque a pergunta "isto é staff?"
// aparece em lugares distantes, e cada um escrevendo o seu `>= AccessModerator` é
// como um deles fica para trás no dia em que aparecer um nível novo.
func (a AccessLevel) EhStaff() bool { return a >= AccessModerator }

// String renders the tier for audit logs.
func (a AccessLevel) String() string {
	switch a {
	case AccessAdmin:
		return "admin"
	case AccessModerator:
		return "moderator"
	default:
		return "player"
	}
}

// Session is a player's connection/session state (CUser subset,
// domain-model.md §2.1). It is owned by the loop goroutine; the conn/out/closeCh
// plumbing is shared with this session's reader and writer goroutines only.
type Session struct {
	Conn        int // index into pUser/pMob; also HEADER.ID on the wire
	AccountName string
	AccountID   int64
	AccessLevel AccessLevel // account.role tier; gates in-game GM commands (issue #122)
	// Cash e RMT da conta. É o que o painel da Loja do Servidor mostra no rodapé.
	//
	// SÃO CÓPIA, E CÓPIA ENVELHECE: a carteira mora na conta, no banco, e muda
	// por caminhos que não passam por esta sessão — a recarga pelo site, um
	// ajuste da staff, outra sessão da mesma conta. Por isso o Cash é RELIDO do
	// banco antes de a vitrine ir para a tela (ver carteira.go), e não apenas
	// somado e subtraído aqui.
	//
	// Isso é só a TELA. Gastar não usa este número: a compra em Cash bate no
	// banco, com FOR UPDATE, e é o banco que recusa (ver lojacompra.go). Uma
	// cópia adiantada nunca virou dinheiro que não existe.
	//
	// O Rmt ainda é só do login: não existe RPC que leia a carteira de RMT
	// sozinha, e inventar um é maior que este conserto. Enquanto não existir,
	// o RMT no rodapé pode estar velho pelo mesmo motivo que o Cash estava.
	Cash int32
	Rmt  int32
	// CashEmLeitura marca que já há uma releitura da carteira a caminho do banco.
	// Sem isso, abrir e fechar a vitrine depressa viraria uma ida ao banco por
	// clique, e a última a voltar mandaria na tela — que nem sempre é a mais nova.
	CashEmLeitura bool
	// PasseNivel é o nível do passe da CONTA, lido no login e copiado para cada
	// personagem que entrar (ver character.go). Fica na sessão porque é da conta, e
	// porque é aqui que o SetPassLevel o atualiza sem esperar relogin.
	PasseNivel uint8
	// O painel da loja aberto, e em que página e filtro ele está. O servidor
	// avisa quem está com ele aberto quando o mercado muda, em vez de deixar o
	// cliente perguntar de tempos em tempos — ver handler.mercadoMudou.
	LojaAberta bool
	LojaPagina int16
	LojaFiltro int16
	// ReconciliandoEscrow está ligada enquanto a reconciliação do escrow em
	// dinheiro real está no banco, logo depois do login.
	//
	// Ela existe porque a reconciliação cancela TODO anúncio ativo da conta, e
	// faz isso sob a suposição de que quem acabou de entrar não tem barraca de
	// pé. A suposição vale no instante em que a pergunta é feita e pode deixar de
	// valer antes de a resposta chegar: o banco lento e o vendedor rápido, e um
	// anúncio recém-nascido é cancelado com a barraca nova de pé.
	//
	// Enquanto ligada, o lojaAbrir recusa prateleira em dinheiro real. É a mesma
	// natureza do HonraCobrando logo abaixo: trava de curta duração contra uma
	// ação do jogador que atravessa uma ida ao banco. Session-only, como todo o
	// resto daqui.
	ReconciliandoEscrow bool
	// A Loja de Honra aberta: qual God of War a abriu (0 = nenhuma) e se um
	// debito de pontos esta no ar. O id do NPC e o que permite a compra exigir
	// presenca, em vez de aceitar qualquer pedido de qualquer lugar do mundo; a
	// trava e o que impede dois cliques rapidos de virarem dois debitos
	// simultaneos. Ver handler/loja_de_honra.go.
	LojaHonraNPC  int
	HonraCobrando bool
	Slot          int
	Mode          Mode
	IP            string
	// Maquina is MSG_AccountLogin.AdapterName, kept as the legacy kept it in
	// CUser.Mac (_MSG_AccountLogin.cpp:70): the GUID of the client's first network
	// adapter, parsed into four ints by the WYD.exe (0x4865DC-0x4866FD, sscanf
	// "%x %x %x %x"). It is what tells two accounts on one computer apart from two
	// players — IP cannot, because behind the Railway proxy every connection
	// arrives from a different 100.64.0.x. Sent by the client, so it is a hint,
	// not proof: see handler.maquinaConhecida.
	Maquina    [4]int32
	CrackError int  // anti-cheat violation count (CUser.NumError)
	Whisper    bool // true blocks incoming whispers
	// Snd is the status line "/snd" sets, shown to anyone who inspects this
	// character (_MSG_MessageWhisper.cpp:591 sets it, :1640 shows it). Session
	// scope is deliberate and matches the legacy, which clears Snd on every login
	// (ProcessDBMessage.cpp:798) — it is never persisted.
	Snd string
	// Os três desligadores de canal do legado (_MSG_MessageChat.cpp:117-150),
	// alternados por "partychat"/"kingdomchat"/"guildchat" e lidos na ENTREGA:
	// true = este jogador não recebe mais aquele canal. Escopo de sessão, como o
	// legado, que zera os três a cada login (ProcessDBMessage.cpp:414-416).
	//
	// O canal Cidadão não tem desligador: o SyncMulticast do legado não olha
	// nada, e portar um seria inventar mecânica.
	PartyChat bool
	KingChat  bool
	GuildChat bool
	// UltimaMensagemCanal é o World.Now da última linha de Reino ou Cidadão desta
	// sessão; 0 é nunca. Os dois canais alcançam gente fora da tela, então o
	// legado põe 3 segundos entre uma linha e a outra (pUser.Message).
	UltimaMensagemCanal uint32
	GuildDisable        bool // hide guild tag (guildon/guildoff)
	// GuildaPedidoEm é quando este jogador pediu, pela última vez, uma aba do
	// Painel de Guilda que vai ao banco. É o freio contra um cliente remendado
	// pedir o quadro em laço (handler/guildapainel.go).
	GuildaPedidoEm time.Time
	// RecusasDeAcesso conta quantas vezes ESTA conexão levou uma recusa de acesso
	// restrito. É do laço, como todo o resto da sessão, e não precisa de trava.
	//
	// Ele existe para o fechamento atrasado de uma recusa não derrubar o que veio
	// DEPOIS dela: o cliente devolve os campos à pessoa e ela pode entrar de novo, com
	// a conta certa, NO MESMO SOCKET. Sem o contador, o fechamento agendado pela
	// recusa antiga chegaria em cima de uma sessão que agora é legítima.
	RecusasDeAcesso int

	TradeMode      int             // non-zero while in auto-trade (blocks attacks)
	Trade          TradeState      // P2P direct-trade state (lote2-trade-autotrade.md)
	AutoTrade      *AutoTradeState // non-nil while a personal shop is open (issue #115); TradeMode==1
	NovatoEmCurso  bool            // um /novato já está esperando a resposta do banco
	DonateEmCurso  bool            // uma RCoin já espera o crédito do banco
	CompraEmPontos bool            // uma compra paga em pontos de lojinha espera o banco
	// A última compra da Loja de Rcoin: o pedido que o cliente mandou e o 0x0F0F
	// que ela rendeu. Um pedido repetido recebe esta resposta de novo, sem ir ao
	// banco — é o que separa "cliquei duas vezes" de "quero comprar duas".
	RcoinPedido       uint32
	RcoinResposta     []byte
	LastAttackTick    uint32    // ClientTick of the last accepted attack (cadence gate)
	PotionTick        uint32    // CUser.PotionTime: server clock of the last accepted potion
	LastAttack        int       // SkillIndex of the last attack
	LastIllusionTick  uint32    // ClientTick of the last Huntress Ilusao movement
	ReqHp             int32     // CUser.ReqHp: server-owned HP target for regen/potions
	ReqMp             int32     // CUser.ReqMp: server-owned MP target for regen/potions
	CriticalProgress  uint16    // CUser.cProgress used by BASE_GetDoubleCritical
	ShortSkill        [16]uint8 // client hotbar layout (CUser.CharShortSkill, _MSG_SetShortSkill)
	LoginSpawnX       int16     // last server-injected login spawn, for movement diagnostics
	LoginSpawnY       int16
	LoginTick         uint32
	LoggedFirstAction bool // first post-login _MSG_Action diagnostic was emitted

	// AttackRefusals counts, per anti-cheat gate restored from the legacy
	// attack handler, the attacks this session had refused (keyed by the gate's
	// name). It is logged per account on disconnect: an honest client tripping
	// a gate has to show up in the log, not as "não consigo bater" in support.
	AttackRefusals map[string]int

	// DuelTarget is CUser.RankingTarget: the conn I've most recently invited to a
	// 1v1 duel (0 = none). DuelTargetExpiry is the wall-clock deadline (Unix
	// seconds) for that pending invite — the legacy _MSG_ReqRanking.cpp has no
	// explicit decline opcode, so a timeout is how "recusado" is modeled (same
	// shape as Entity.GuiltyUntil).
	DuelTarget       int
	DuelTargetExpiry int64

	// XPPerdidaAvisoAt is when (Unix seconds) this character was last told a
	// kill paid them nothing (handler.avisarXPPerdida). Session scope on
	// purpose: a fresh login may be told again at once.
	XPPerdidaAvisoAt int64

	seen map[int]struct{} // entity ids already create-mob'd to this client (view set)

	// S→C send diagnostics (sendstats.go): per-type counts, totals, the trailing
	// frames and the out-queue high-water mark, dumped on disconnect so a frozen
	// client leaves a post-mortem. Loop-owned like every other Session field.
	sentFrames   uint64
	sentBytes    uint64
	sentByType   map[protocol.Type]uint32
	lastSent     [lastSentRing]sentRecord
	lastSentIdx  int
	outHighWater int

	conn    net.Conn
	out     chan outFrame
	closeCh chan struct{}
	closed  bool
}

// close signals the session's writer to flush any queued S→C frames and then
// close the socket (which in turn unblocks the reader). The writer owns the
// socket close so that messages queued just before a close (e.g. an error
// notice) are still delivered. Idempotent; loop-only.
func (s *Session) close() {
	if s.closed {
		return
	}
	s.closed = true
	close(s.closeCh)
}

// TradeState is a player's direct (P2P) trade with another player
// (lote2-trade-autotrade.md). Active is set when the trade window opens; Slots
// and Money are the finalized offer recorded at confirmation.
type TradeState struct {
	Active     bool
	OpponentID int
	Confirmed  bool
	Money      int32
	Slots      []int // offered carry slots
}

// AutoTradeState is an open personal shop (the legacy pUser[conn].AutoTrade, issue
// #115). It is session-only — never persisted, so a shop never survives relogin —
// and loop-owned. Items are sold out of the account Cargo by CargoPos; the offered
// item is copied into the slot so a buy can memcmp it against the live Cargo slot
// (anti item-swap). While a shop is open the session's TradeMode is 1.
type AutoTradeState struct {
	Title string
	Tax   int16
	Slots [MaxAutoTrade]AutoTradeSlot

	// Moeda de cada slot na vitrine (Loja do Servidor): 0 ouro, 1 cash, 2 RMT.
	// A janela de barraca do cliente só sabe de ouro, então o vendedor escolhe a
	// moeda depois, pelo nosso painel, e o preço digitado passa a ser cobrado
	// nela. Slot sem escolha fica em ouro, que é o comportamento de sempre.
	Moeda [MaxAutoTrade]uint8

	// CloneID is the mob entity that stands in for the seller (Entity.ShopOwner
	// points back). MaxUser or above when the stall is a separate body; 0 when
	// the shop had to fall back to the legacy pose, which is what happens when no
	// clone template is configured or the world is out of mob slots.
	CloneID int

	// OpenedAt and PaidUntil drive the shop-points clock (pontos por tempo de
	// lojinha), both on the loop clock (World.Now, ClientTick ms) like the respawn
	// queue. OpenedAt is when the stall went up; PaidUntil is the end of the last
	// quarter-hour already credited, so a shop that closes mid-window is paid for
	// its completed windows only, and never twice for the same one.
	OpenedAt  uint32
	PaidUntil uint32
}

// AutoTradeSlot is one shop offer: the item (copy of the seller's Cargo slot), its
// source Cargo slot (-1 = empty), and its price. CargoPos < 0 means an unused slot.
type AutoTradeSlot struct {
	Item     Item
	CargoPos int
	Price    int32
}

// Entity is a world entity (CMob subset, domain-model.md §2.2). Players
// (ID < MaxUser) and mobs (ID >= MaxUser) share this type and the same index
// space. Phase 3 carries only the minimum; full STRUCT_MOB state arrives with
// the handlers (Phase 4).
type Entity struct {
	ID   int
	Mode EntityMode
	Name string
	// Tab é a linha que o jogador põe ACIMA do personagem com "/tab"
	// (pMob.Tab, _MSG_MessageWhisper.cpp:548). Vive aqui, e não na Session,
	// porque quem a desenha é o MSG_CreateMob da ENTIDADE — inclusive o que
	// outro jogador recebe ao entrar na tela. Não é persistida, como no legado.
	Tab []byte
	X   int16
	Y   int16
	// SaveX/SaveY are the Gema Estelar warp save-point (STRUCT_MOB.SPX/SPY,
	// _MSG_UseItem.cpp Vol 12/13) — distinct from X/Y, the player's live position.
	// 0/0 means no point has ever been saved.
	SaveX    int16
	SaveY    int16
	HP       int32
	MaxHP    int32
	MP       int32 // current mana (status display)
	MaxMP    int32
	Damage   int32 // CurrentScore.Damage (attacker output, combat §4.3)
	AC       int32 // CurrentScore.Ac (defender mitigation)
	Master   int   // weapon mastery (combat level)
	Critical uint8 // MOB.Critical: rand()%255 threshold for partial critical
	Parry    int   // CMob.Parry addend for GetParryRate
	Level    int32 // CurrentScore.Level (drop/exp curves)
	Exp      int64 // STRUCT_MOB.Exp: players accumulate it; for a mob it's the kill reward
	Coin     int32 // carried gold

	// Mob AI (mobai.go; only meaningful for monsters). EnemyList is the legacy
	// CMob.EnemyList[MAX_ENEMY=13]; Target is the currently selected entry
	// (0 = none). AtkTick is the mob's last-attack server time (cadence);
	// SpawnX/SpawnY is the position the mob (re)spawned at. Range is the mob's
	// attack reach — the max EF_RANGE over its template's equips
	// (BASE_GetMobAbility, Basedef.cpp:2415), cached at spawn; 0 means no ranged
	// gear (the AI falls back to melee adjacency).
	Target         int
	EnemyList      [protocol.MaxTarget]int
	AtkTick        uint32
	SpawnX, SpawnY int16
	Range          int16
	// AndouDesdeOGolpe marca que o pet recebeu um MsgAction depois do último
	// golpe; GolpeSeq alterna a animação do golpe do pet (handler/mobai.go).
	AndouDesdeOGolpe bool
	GolpeSeq         uint8

	// Mob roaming (CMob.h:47-69, StandingByProcessor/SetSegment). SegX/SegY are
	// this INSTANCE's waypoints (already randomized ±SegmentRange at spawn,
	// GenerateMob Server.cpp:3536-3546; 0 = unused slot, skipped by the walker).
	// SegmentX/SegmentY is the current waypoint — also the aggro/leash anchor
	// (BattleProcessor leashes on SegmentX±HALFGRIDX, CMob.cpp:292) — always set,
	// even for mobs without a route (= spawn point). WaitTicks counts down the
	// pause at a waypoint (legacy WaitSec; our tick≈1s so ticks≈seconds,
	// cadence UNVERIFIED). GenIndex is the NPCGener block that spawned this mob
	// (-1 = none), reserved for the per-generator respawn accounting (M5).
	RouteType          uint8
	SegListX, SegListY [5]int16
	SegWait            [5]int16
	SegProgress        int8
	SegDir             uint8 // 0 = forward, 1 = backward (RouteType 2/3 ping-pong)
	WaitTicks          int16
	SegmentX, SegmentY int16
	GenIndex           int16
	// Template is the raw STRUCT_MOB bytes this mob was spawned from (boot template,
	// shared by reference — no copy). Retained so the mob can be re-spawned at its
	// SpawnX/SpawnY after it dies (world/respawn.go). nil for players.
	Template []byte
	// TemplateName is the template file this mob was spawned from (MobSpawn).
	TemplateName string
	// GenRev is the recipe revision of its block when it was born (MobSpawn).
	GenRev   uint32
	Merchant uint8 // bit-packed: spawn city in bits 6-7 (lote2-movimento.md ChangeCity)
	// MobMerchant is the OTHER merchant byte, STRUCT_MOB.Merchant @17: the one the
	// legacy routes quest NPCs by (_MSG_Quest.cpp:33). The Treinadores are 36/40/41
	// here and 100/104/105 in Merchant above; see internal/campotreino.
	MobMerchant  uint8
	NonCombatNPC bool // true for town/service NPCs protected from player damage
	// ShopPointPrice is the shop-points price of this merchant's stock, keyed by
	// Carry index (0060_shop_points, npc_shop_item.price_points). A Carry slot
	// present here is paid for in POINTS; anything absent is paid for in gold, so
	// nil — the value every other entity in the world has — means "gold only".
	//
	// It lives on the entity rather than in a Dispatcher-side map so it dies with
	// the NPC: ids are recycled, and a stale price surviving a reload would sell
	// whatever moved in next at the old shop's rate.
	ShopPointPrice map[int]int32
	Grade          uint8 // NPC sub-type for Merchant==100 quest NPCs (EF_GRADE0 of Equip[0])

	Class       uint8    // character class (0=TK 1=FM 2=BM 3=HT); drives the visual model
	AttackRun   uint8    // CurrentScore.AttackRun speed byte — mobs: template value (set at spawn); players: derived live (handler attackRunOf)
	Route       [24]byte // last walk route from _MSG_Action (pMob.Route, MAX_ROUTE=24)
	LastCity    int16    // last city visited (0..3); login spawn = its default area
	Clan        uint8    // clan/race
	Guild       uint16   // guild id (0 = none)
	GuildLevel  uint8    // 0 = member … 9 = leader
	Citizen     uint8    // MobExtra.Citizen; city allegiance and guild creation metadata
	ClassMaster uint8    // party tier (MobExtra.ClassMaster)
	// Celestial quest gate flags (MobExtra.QuestInfo.Celestial, Basedef.h:659-678).
	// CelLv40/CelLv90 unlock the Celestial level 40/90 caps (CheckGetLevel gate,
	// CMob.cpp:1107); CelCircle marks the Cythera Arcana quest done (Pedra da
	// Fúria no nível 200). Persisted.
	CelLv40, CelLv90, CelCircle uint8
	// TerraMistica is MobExtra.QuestInfo.Mortal.TerraMistica (_MSG_Quest.cpp
	// AMU_MISTICO, issue #139): set once the party quest is completed, so the
	// NPC won't hand it out twice. Persisted.
	TerraMistica uint8
	// NewbieQuest is MobExtra.QuestInfo.Mortal.Newbie (_MSG_Quest.cpp:1896-2100):
	// which of the four training-field trainer steps is done (0..4). Each step
	// demands the previous one, so it is persisted.
	NewbieQuest uint8
	// MolarGargula marca que este personagem ja usou o Molar de Gargula (0093):
	// o molar sobe o set vestido para +7 uma unica vez, entao a marca precisa
	// sobreviver ao relog.
	MolarGargula uint8
	// NivelRetroativo e ate onde o personagem recebeu as pecas de nivel que o
	// jogo deixou de entregar (0172): 0 nada, 1-399 ate aquele nivel, 1000
	// concluido. Persistido, para a entrega do login nao se repetir.
	NivelRetroativo      uint16
	ArchLv355, ArchLv370 uint8
	MortalLevel          uint16
	CelestialArchLevel   uint8
	// ArchCristal is QuestInfo.Arch.Cristal: how many of the four Arch crystal
	// quests are done (0..4). Persisted — playerBaseAC rebuilds their AC from it.
	ArchCristal uint8
	// NightmareTickets is MobExtra.NT: the Arcano-tier Pesadelo entries a
	// Celestial holds. Escritura do Pesadelo grants 13; each admission spends
	// one (handler/pesadelo.go).
	//
	// Persisted on the character row rather than in the raw 552-byte MobExtra
	// blob, like the rest of the progression this port moved into Postgres
	// (migration 0025). Both the grant and the spend flush immediately, because
	// an entry is bought with gold.
	NightmareTickets int32
	// KefraTicket is MobExtra.KefraTicket: the Kefra Hall entries the character
	// holds. The Sobrevivente trades a Pergaminho_Selado for 100 and each passage
	// through the Hall tile spends one (handler/kefra_hall.go, migration 0068).
	// Persisted on the character row, and both the grant and the spend flush
	// immediately, for the same reason as the Pesadelo entries: the scroll is paid
	// for.
	KefraTicket int32
	// A SEGUNDA VIDA DO CELESTIAL (0061_sub_celestial). Os campos de progressao
	// acima sao sempre a vida ATIVA. A guardada viaja inteira como JSON e so a
	// troca a le; o NIVEL dela fica fora do JSON porque a formula de pontos do
	// Celestial CS o consulta em toda derivacao de score.
	SubCelestialGuardada string
	SubCelestialLevel    uint16
	// SubCelestialAtivo: 0 a principal em uso, 1 o Sub. Nao e redundante com
	// ClassMaster: os dois estados carregam ClassMaster 4.
	SubCelestialAtivo uint8
	CelestialReset    uint8
	Soul              uint8 // MobExtra.Soul; 0 means no modeled soul
	Fame              int32 // MobExtra.Fame; loaded from DB, updated by Selo do Guerreiro, and shown by /nick
	QuestFlag         uint8 // volatile quest-area pass (CMob.QuestFlag; e.g. Quest 256)
	// PKMode is the player-toggled Player-Killer consent flag (K key, _MSG_PKMode;
	// legacy pUser[conn].PKMode). It gates whether the player can land PvP combat
	// hits, but it does NOT by itself blink the nickname. Session-only, not persisted.
	PKMode bool

	// GMInvisible is "/gm invisivel", the port of the legacy "+snoop" (MSV_SNOOP,
	// imple.cpp:1567): no frame about this character reaches any other client
	// (World.hiddenFrom) and monsters do not take it as a target. Lives on the
	// per-connection entity and is never persisted, so it lasts until the GM
	// disconnects — a GM who forgot it on is visible again at the next login.
	GMInvisible bool

	// PKPoint is the legacy PKPoint byte (GetFunc.cpp GetPKPoint/SetPKPoint, the
	// hidden KILL_MARK carry slot): the chaos/karma counter, clamped [1,150] on
	// write, 75 = neutral. The wire/display value is PKPoint-75 (range [-74,+75],
	// the /cp command). Drives the nick color via handler.pkPoint (MobName[12],
	// GetFunc.cpp GetCreateMob): chaos = PKPoint unless Guilty > 0, in which case
	// chaos = 0 (red/blinking). Persisted.
	PKPoint uint8
	// Guilty is the legacy Guilty byte (same KILL_MARK slot, clamped [0,50] on
	// write): set to 8 on landing (or receiving) a PvP hit outside a duel
	// (_MSG_Attack.cpp "PK - War - Miss"), decremented by 1 roughly every 8s while
	// connected (RegenMob, Server.cpp). Guilty > 0 forces the nick red regardless
	// of PKPoint. Persisted (a player who logs out mid-decay must resume, not reset).
	Guilty uint8
	// CurKill/TotKill are the legacy kill-streak counters (GetCurKill/GetTotKill,
	// same KILL_MARK slot): CurKill is the current uninterrupted PvP kill streak
	// (reset to 0 on death, MobName[13]); TotKill is the lifetime PvP kill count
	// (MobName[14..15] LE). Cosmetic (packed into every MSG_CreateMob so other
	// clients can render them) — no gameplay effect modeled. Persisted.
	CurKill uint8
	TotKill uint16

	// PasseNivel é a moldura do passe de batalha desta pessoa, 0 a 4, e ela vem da
	// CONTA e não do personagem (0128).
	//
	// Ela está na entidade e não só na sessão porque quem monta o MSG_CreateMob tem a
	// entidade na mão — inclusive o de OUTRO jogador entrando na tela, que é
	// justamente quando a moldura de alguém precisa aparecer para terceiros.
	//
	// Monstro e NPC ficam em zero.
	PasseNivel uint8

	Str        int16 // CurrentScore attributes (base + equipment, kept live by refreshScore)
	Int        int16
	Dex        int16
	Con        int16
	Special    [4]int16 // CurrentScore.Special[4] = BaseSpecial + equipment/affects
	ScoreBonus uint16   // free attribute points

	// Segment is CMob::Segment (CMob.h:61): the last QUARTER of the current level
	// this character has been credited for. CheckGetLevel splits a level into four
	// and reports each crossing once, which is how the client is told to redraw its
	// experience bar between level-ups. Runtime state — it resets on every level
	// and is not persisted, exactly as the original keeps it on the live mob.
	Segment int32

	// Skill state (skills front). SkillBonus is derived (level*3 − Σ learned
	// costs, BASE_GetBonusSkillPoint) at login and level-up; SpecialBonus is
	// incremental (+2/level) and persisted. Magic scales caster skill damage;
	// SaveMana discounts mana costs (source of both on players UNVERIFIED —
	// zero until captured). Resist[4] feeds SkillResistScale (mobs: template).
	LearnedSkill    int32
	SecLearnedSkill int32
	SkillBonus      uint16
	SpecialBonus    uint16
	BaseSpecial     [4]int16 // allocated mastery points (BaseScore.Special)
	SkillBar        [4]uint8 // MOB.SkillBar (persisted with the character)
	Magic           int16
	SaveMana        int16
	Resist          [4]int16

	// BaseScore: the equipment-free score (allocated attributes + level/class-derived
	// AC/Damage/MaxHP/MaxMP). CurrentScore (the live fields above + AC/Damage/MaxHP/
	// MaxMP) = BaseScore + equipment, recomputed by handler.refreshScore whenever gear
	// or attributes change. Derived once on login (current − equipment) and not
	// persisted (it is re-derived from the persisted CurrentScore each login).
	BaseStr, BaseInt, BaseDex, BaseCon int16
	BaseAC, BaseDamage                 int32
	BaseMaxHP, BaseMaxMP               int32
	// Magic, Parry (evasion) and Resist have NO base term: the legacy has none either.
	// BASE_GetCurrentScore seeds a fresh local `magic` from equipment (Basedef.cpp:3194),
	// accumulates the derived terms and stores it at :4729 — there is no BaseScore.Magic
	// anywhere in the original, and the stored MOB.Magic never feeds back in. refreshScore
	// derives all three entirely from current equipment every call (issue #211 bug 3,
	// issue #231).

	// HpAddPct/MpAddPct: EF_HPADD/EF_MPADD percent bonus from equipment (e.g. +10 =
	// +10%). Cached by refreshScore and applied at READ time (effective max HP/MP),
	// never baked into MaxHP/MaxMP — so the persisted score stays flat and the base
	// derivation by subtraction holds (captura-wyd-affect-divina.md §E).
	HpAddPct, MpAddPct int32

	// RegenHP/RegenMP are MOB.RegenHP/RegenMP (Basedef.h:1840): the EF_REGENHP /
	// EF_REGENMP sum over equipment, clamped 0..255. Derived on every refreshScore
	// like Critical and the resists — never persisted.
	//
	// RegenMP does two unrelated jobs, which is why it is a field and not a local:
	// the ten-second trickle adds it for ClassMaster >= CELESTIAL
	// (ProcessSecMinTimer.cpp:676-680), and it is a term in the debuff-resist roll
	// for EVERYONE, at any tier (_MSG_Attack.cpp:1194).
	RegenHP, RegenMP int32

	// RunSpeedBonus is the summed EF_RUNSPEED from equipped gear (boots), cached by
	// refreshScore and applied at read time by handler.attackRunOf to the move-speed
	// (low) nibble of AttackRun.
	RunSpeedBonus int32
	// AttackSpeedBonus is the summed EF_ATTSPEED from equipped gear, refine-scaled
	// (Basedef.cpp:3201), cached by refreshScore for the attack nibble of AttackRun.
	AttackSpeedBonus int32
	// DanoFisicoPct and DanoMagicoPct are the percentage damage the Hércules and
	// Hecate accessories grant (reforma dos acessórios), refine-scaled and cached by
	// refreshScore. Not legacy.
	DanoFisicoPct, DanoMagicoPct int32

	// Affect holds the active buffs/debuffs (STRUCT_AFFECT[32]). DivineEnd is the
	// wall-clock (Unix seconds) deadline of the Divine buff — the source of truth for
	// its expiry; the slot's Affect.Time is only the client icon timer.
	Affect    [MaxAffect]Affect
	DivineEnd int64

	// InvisivelDesde is the server clock (World.Now, ms) at which the Huntress's
	// Invisibilidade landed, and InvisivelAtiva says the clock is running. The
	// affect slot is only the icon: the 10 s the skill lasts are shorter than one
	// 8 s affect sweep can measure, so the handler expires it from this clock
	// (handler/invisibilidade.go).
	InvisivelDesde uint32
	InvisivelAtiva bool

	// Rsv is the MOB.Rsv state-flag byte (RSV_HASTE/BLOCK/…), recomputed from
	// the active affects by refreshScore. The affect score contributions (Aff*)
	// are cached the same way and applied at READ time (effective getters), so
	// the persisted flat score never bakes a buff in (no double-count on
	// re-login — same policy as HpAddPct/Divine).
	Rsv            uint8
	AffDamage      int32
	AffAC          int32
	AffMaxHP       int32
	AffMaxMP       int32
	AffStr         int16
	AffInt         int16
	AffDex         int16
	AffCon         int16
	AffRunSpeed    int32
	AffAttackSpeed int32
	AffCritical    int16
	// AffEsquivaPct multiplies the dodge roll (+50 = ×1,5): the Huntress Captura
	// tree, Evasão Aprimorada and Proteção das Sombras (handler/arvore_captura.go).
	AffEsquivaPct int32
	// UltimoPvP é o World.Now do último golpe de jogador em jogador, dado ou
	// levado; 0 é nunca. A Aura da Vida do TK Confiança cura menos em PvP.
	UltimoPvP uint32
	// ImuneDebuffAte é o World.Now até quando o Desintoxicar da FM Magia Branca
	// segura debuff novo; 0 é nunca (handler/arvore_magia_branca.go).
	ImuneDebuffAte uint32
	// CuraReduzidaAte é o World.Now até quando o Choque Divino corta a cura que
	// este personagem recebe, poção inclusive (handler/arvore_magia_branca.go).
	CuraReduzidaAte uint32
	// SemPocaoAte é o World.Now até quando o Cancelamento da FM impede este
	// personagem de beber poção de vida ou de mana; 0 é nunca
	// (handler/arvore_magia_especial.go).
	SemPocaoAte       uint32
	AffSpecial        [4]int16
	AffResist         [4]int16
	AffForceDamage    int32 // ForceDamage, e.g. Ligacao Espectral
	AffForceMobDamage int32 // ForceMobDamage, e.g. Toxina da Serpente
	AffExpBonus       int32 // from affect type 39 (Baú de XP)
	// AffHpAbs is the on-hit lifesteal percent from the Jóia da Absorção (affect
	// type 8, bit 3): the attacker recovers AffHpAbs% of the damage dealt, 50%
	// proc, capped 350/hit (_MSG_Attack.cpp:1651). AffMagic is the +20% Magic
	// buff from the Jóia do Poder (affect 8, bit 5), read into effective Magic.
	AffHpAbs int32
	AffMagic int32

	// AffAccuracy is the attacker-side precision bonus: it is subtracted from the
	// target's parry rate, in the same thousandths that formula already uses for the
	// Dex/5 term, the +100 of skill bit 24 and the +500 of the Revelacao
	// (GetFunc.cpp:686, _MSG_Attack.cpp:1410). SERVER RULE: the original writes this
	// number into CMob::Accuracy (Basedef.cpp:4539) and never reads the field
	// anywhere, so the Joia da Precisao is inert in the legacy — this port spends its
	// 50 in the one place the legacy's own naming points at.
	AffAccuracy int32

	// MissStreak counts this entity's skill misses in a row on MissStreakTarget
	// (handler capMissStreak, combatrule.MaxMissStreak). Loop-owned combat state,
	// never persisted: a relog starting the count over is harmless.
	MissStreakTarget int32
	MissStreak       int32
	// AffDamageMultiPct is the legacy DAMAGEMULTI percentage (100 = neutral): a
	// READ-time damage multiplier applied over Damage+AffDamage but before
	// WeaponDamage, exactly where Basedef.cpp:4654 multiplies CurrentScore.Damage.
	// Additive deltas can't express it — attributeDamageBonus lands on the flat
	// Damage after the affect pass, and the multiplier must cover it too.
	AffDamageMultiPct int32
	EquipExpBonus     int32 // from fairy slot + grade/gem gear (CMob.cpp:711-870)
	// EquipDropBonus is the drop half of the same walk (CMob.cpp:700-870): the
	// Fada Azul and Vermelha, Grade 5 pieces and gem-0 pieces. It widens the odds
	// of an item falling AND the bonus rolled onto it when it does.
	EquipDropBonus int32

	// EquipForceDamage is the perfuração half of the same walk (CMob.cpp:866):
	// the Esmeralda gem (gem 1) on a +10..+15 piece, worth 40 per refine step
	// above +9, or 80 on a Grade 6 piece. It is a FLAT number added to a landed
	// blow after the target's defence has already come off, which is what makes
	// it perfuração and not damage (_MSG_Attack.cpp:1309).
	EquipForceDamage int32

	// EquipGarnet is the absorção half (CMob.cpp:873): the Garnet gem (gem 3) on a
	// +10..+15 piece, 40 per refine step above +9, or 80 on a Grade 8 piece.
	// garnet.go decides how much of a blow it actually takes.
	EquipGarnet int32

	EquipVisual [16]uint16 // visual item codes for MSG_CreateMob/UpdateEquip
	EquipAnct   [16]uint8  // refine/ancient glow overlay bytes paired with EquipVisual

	// Party state (lote2-party-guilda-guerra.md). Members point Leader at the
	// leader conn; leaders keep Leader=0 and own the PartyList. LastReqParty is
	// who last invited this entity (anti-forge gate).
	Leader       int
	LastReqParty int

	// Summoner is the conn of the player that evoked this mob (GenerateSummon's
	// pMob.Summoner, Server.cpp:3244); 0 = not a summon. A summon's Leader is
	// its owner — never the owner's party leader (see Evocacoes).
	Summoner  int
	PartyList [MaxParty]int

	// Evocacoes é o bando de um jogador: os ids dos pets que ELE evocou.
	//
	// DIVERGÊNCIA DELIBERADA DO LEGADO, decidida pelo Marco em 26/09/2026:
	// "evocações não devem entrar em grupo, mas quando o BM evoca ela simula um
	// grupo". No legado os pets moravam na PartyList do líder (GenerateSummon,
	// Server.cpp:2981-3027): ocupavam vaga de jogador, faziam o BM parecer já
	// agrupado ("já tem grupo" no convite) e entravam em todo laço sobre o grupo.
	// Aqui o bando é do dono e fica fora do grupo; o grupo e a guilda do pet são
	// os do dono, resolvidos na hora (handler.donoDaEvocacao).
	Evocacoes [MaxParty]int

	// ShopOwner is the conn of the player whose personal shop this mob IS, and it
	// is what makes the "lojinha solta" possible: the stall is its own entity, so
	// the owner walks away while it keeps selling. 0 = not a shop clone.
	//
	// A divergence from the legacy, where the seller himself was the stall. It is
	// only viable because the client gates the shop on the entity's title buffer
	// (+0x9BC) and not on its id: MSG_CreateMobTrade marks ANY entity as a stall,
	// and the click answers _MSG_ReqTradeList with the raw id — the id < MAX_USER
	// test lives in the other branch, the one for entities without a title.
	// Confirmed by disassembly of WYD.exe 7662 (0x0048541D and 0x004604D1).
	//
	// It is a back-reference, not ownership: the shop itself lives on the owner's
	// Session.AutoTrade, which points back here through CloneID. Both die together
	// in closeAutoTrade.
	ShopOwner int

	// inimigoDoReino é a marca de Inimigo do Reino, por reino (0 Hekalotia,
	// 1 Akelonia): se está ligada e desde quando (Now, em ms). Só jogador tem;
	// ver world/reinos.go.
	inimigoDoReino      [2]bool
	inimigoDoReinoDesde [2]uint32

	Equip [MaxEquip]Item // equipped items
	Carry [MaxCarry]Item // inventory; for mobs this is also the loot table (§2.2)
}

// IsPlayer reports whether an entity index belongs to a player (domain-model.md
// §1: id < MaxUser ⇒ player).
func IsPlayer(id int) bool { return id >= 0 && id < MaxUser }
