// Package world is the authoritative, in-memory game state of the tmServer and
// its single-owner game loop (domain-model.md §1/§5, migration-plan.md §3.5).
//
// Concurrency model — the one rule that preserves parity and kills item dup:
// ALL world state is owned by exactly one goroutine (Run); it is never mutated
// elsewhere. Network I/O runs in per-connection goroutines that only exchange
// messages with the loop over channels (events in, per-session out). There are
// no locks on world state, mirroring the original single-threaded WinSock
// reactor (domain-model.md §5, guidelines §9).
package world

import (
	"context"
	"fmt"
	"log/slog"
	"runtime/debug"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/protocol"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/rng"
)

// Capacity limits (Basedef.h via domain-model.md §6). The index space is shared:
// pMob[0..MaxUser) are players, pMob[MaxUser..MaxMob) are mobs/NPCs.
const (
	MaxUser        = 1000
	MaxMob         = 25000
	MaxItem        = 5000 // ground items (pItem[])
	MaxCarry       = 64   // inventory slots per entity (MAX_CARRY)
	MaxEquip       = 16   // equipment slots (MAX_EQUIP)
	MaxCargo       = 128  // account-shared warehouse slots (MAX_CARGO)
	MaxAutoTrade   = 12   // personal-shop item slots (MAX_AUTOTRADE, issue #115)
	MaxParty       = 12   // party members (MAX_PARTY)
	DefaultGridDim = 4096

	// DefaultOutBuffer is the per-session outbound queue depth.
	//
	// It was 64, and 64 is smaller than a single ordinary event: teleporting
	// into a populated area sends one MsgCreateMob per monster now in view plus
	// one MsgRemoveMob per monster left behind, all enqueued from the same loop
	// iteration. A Pesadelo room that has finished repopulating holds ~82
	// monsters, so entering it queued about 125 frames against a 64-slot channel
	// — the overflow branch fired and the player was DISCONNECTED at the moment
	// of entry, every time, and the log said "queue full; dropping connection"
	// as if the client were at fault.
	//
	// The drop itself is right and stays: a genuinely stuck client must never
	// stall the single game loop. What was wrong is the threshold, which sat
	// below the busiest legitimate burst instead of above it. 512 clears a full
	// instance four times over and still catches a client that has stopped
	// reading. The cost is the channel's own slots — about 24 KB per session at
	// this depth, since the payloads were already allocated.
	DefaultOutBuffer = 512

	// KefraBossGenIndex is KEFRA_BOSS (Basedef.h:475), the NPCGener block with
	// special fixed-range / fixed-position combat rules in CMob.cpp.
	KefraBossGenIndex = 396
	// KefraGuardLast is KEFRA_MOB_END (Basedef.h:477): the Kefra's guards are
	// blocks KefraBossGenIndex+1 .. KefraGuardLast (397-400).
	KefraGuardLast = 400

	// GroundItemIDOffset is added to a ground item's index on the wire
	// (_MSG_GetItem decodes ItemID-10000; handlers/_MSG_GetItem.md).
	GroundItemIDOffset = 10000
)

// Mode is the session state machine CUser.Mode (domain-model.md §3.1).
type Mode uint8

// Session modes (CUser.h:26-37).
const (
	UserEmpty       Mode = 0
	UserAccept      Mode = 1
	UserLogin       Mode = 2
	UserSelChar     Mode = 11
	UserCharWait    Mode = 12
	UserWaitDB      Mode = 13
	UserPlay        Mode = 22
	UserSaving4Quit Mode = 24
)

// EntityMode is the world-entity state machine CMob.Mode (domain-model.md §3.2).
type EntityMode uint8

// Entity modes (CMob.h:26-35).
const (
	MobEmpty    EntityMode = 0
	MobUserDock EntityMode = 1
	MobUser     EntityMode = 2
	MobIdle     EntityMode = 3
	MobPeace    EntityMode = 4
	MobCombat   EntityMode = 5
	MobReturn   EntityMode = 6
	MobFlee     EntityMode = 7
	MobRoam     EntityMode = 8
	MobWaitDB   EntityMode = 9
)

// Handler processes one decoded client frame inside the loop goroutine, so it
// may freely mutate world state. Phase 4 replaces the default with the real
// per-message dispatch (handlers/*.md).
type Handler func(w *World, s *Session, h protocol.Header, payload []byte)

// Config tunes a World. GridDim defaults to DefaultGridDim (4096); tests use a
// small value to avoid allocating the full dense spatial grids.
type Config struct {
	GridDim    int
	OutBuffer  int           // per-session outbound queue depth
	EventQueue int           // inbound event queue depth
	Now        func() uint32 // server clock (ClientTick); injectable for tests
	// Marcavel decides which items are worth an identity (0033_item_serial).
	// Injected because the rule reads the item catalog — the equip-slot class,
	// and the indexes the original game singled out in BASE_NeedLog — which is
	// content the world neither has nor should have. Nil means nothing is
	// stamped, which is what tests and a catalog-less boot want.
	Marcavel func(Item) bool
	// ShutdownGrace is how long the loop waits after warning players that the
	// server is stopping, so their sockets flush the frame before shutdown closes
	// them. ZERO means announce and move on, which is what tests want: they spin
	// worlds up and down constantly and would otherwise pay this on every one.
	// cmd/tmserver sets the real value.
	ShutdownGrace time.Duration

	// Hardening (Fase 7, migration-plan.md §5), all opt-in:
	// RejectChecksum drops a connection on a CPSock checksum mismatch. The legacy
	// stack is non-rejecting and the ClientPatch NOPs client checks, so this is
	// off by default; enable once a capture confirms the client sends correct
	// checksums (protocol-spec.md §1.5).
	RejectChecksum bool
	// MaxMsgPerSec rate-limits inbound messages per connection (0 = disabled);
	// MsgBurst is the bucket depth (defaults to MaxMsgPerSec when <=0). A flood
	// disconnects the offending connection, protecting the reactor (NF1).
	MaxMsgPerSec float64
	MsgBurst     int

	// IdleTimeout drops a connection that sends nothing for this long. The
	// handshake deadline is cleared once a connection becomes a session
	// (edge.go), so an authenticated socket that goes silent holds one of the
	// MaxUser slots and its goroutine indefinitely — no exploit required, just an
	// open socket that stops talking.
	//
	// OFF by default (0), for the same reason RejectChecksum is: the real
	// client's idle cadence is not documented in the migration notes, and a
	// timeout shorter than it would disconnect legitimate players. Enable once a
	// capture shows how often an idle client actually sends.
	IdleTimeout time.Duration

	// ItemRanges maps item index → its catalog EF_RANGE value (content
	// ItemList.Ranges). SpawnMob uses it to derive a mob's attack reach from its
	// template equips (BASE_GetMobAbility, Basedef.cpp:2415). Immutable after
	// boot; nil means no catalog (every mob fights at melee reach).
	ItemRanges map[int]int16

	// LogSends logs every queued S→C frame (conn/type/id/len) at INFO — the
	// outbound mirror of the dispatcher's "recv packet" log. High volume: enable
	// only while reproducing an incident (freeze investigation,
	// docs/migration/investigacao-freeze-cliente.md).
	LogSends bool

	// StatusFile is the path to the channel-status page (serv00.htm) the client
	// fetches over HTTP before opening the CPSock game connection. When set, the
	// edge answers a "GET" probe with this file's contents; empty serves a
	// built-in default. The client-edge HTTP status check is undocumented in
	// protocol-spec.md (CPSock-only) — discovered from a live client capture.
	StatusFile string

	// ShopCloneTemplate is the raw 816-byte STRUCT_MOB the personal-shop clone is
	// built from (Release/TMsrv/run/npc/Merc_Carbunkle). NIL IS A SUPPORTED
	// STATE, not an oversight: without it a shop falls back to the legacy pose
	// that pins the seller in place, which is what every test world and a
	// content-less boot get.
	ShopCloneTemplate []byte
}

// World holds all mutable game state. Every field is touched only by Run's
// goroutine (and by helpers it calls). Do not access from other goroutines.
type World struct {
	cfg     Config
	log     *slog.Logger
	persist Persistence
	billing Billing
	handler Handler

	sessions []*Session    // index = conn ∈ [0, MaxUser)
	entities []*Entity     // index space shared with players (domain-model.md §1)
	ground   []*GroundItem // pItem[]: items on the floor, index ∈ [1, MaxItem)
	static   []int         // ground ids of the seeded world objects (gates/doors), in seed order
	grid     *Grid
	rng      *rng.MSVC // loop-owned MSVC LCG (parity; like the original global rand())

	// nextMobSlot is where the next mob-id search starts, so ids are handed out
	// in rotation instead of always reusing the lowest free slot. See SpawnMobAt.
	nextMobSlot int

	// cargo is the account-shared warehouse, keyed by account id. It is loaded on
	// account login and lives for the whole account session (it spans character
	// select ↔ play), so it is keyed by account, not session/conn. Loop-owned.
	cargo map[int64]*CargoState

	// quitSaves counts, per account, the teardown saves (character and cargo) of
	// a session that has already closed but whose writes have not returned yet.
	// While it is non-zero the account is still in use, exactly as the legacy
	// DBSrv keeps the account's slot until the quit-save lands: a login that read
	// the database now would load the state from BEFORE that save. See
	// AccountSaving. Loop-owned.
	quitSaves map[int64]int

	// deliveryPlaced holds, per account, the delivery_queue ids this process has
	// already put into the in-memory cargo, for as long as the cargo is loaded.
	// Two drains of the same mailbox — the login and a deliver-now, or two
	// deliver-nows from the site while the game is open — each fetch the pending
	// list before the other's ack commits, and without this the second one would
	// place the same paid item again. Loop-owned.
	deliveryPlaced map[int64]map[int64]bool
	// deliveryUnacked holds the placed ids whose 'delivered' mark has not
	// committed yet. Every cargo save of that account carries them, so the cargo
	// that holds the items and the mark land in one transaction: a plain SaveCargo
	// that wrote the items while their rows stayed 'pending' would deliver them a
	// second time at the next login. Loop-owned.
	deliveryUnacked map[int64][]int64

	// guilds is the minimal guild registry (guild.go): name/fame keyed by guild
	// id. In-memory only — there is no guild-creation flow yet to persist
	// against. Loop-owned.
	guilds map[uint16]GuildInfo

	// worldEvent is the portal-managed global drop event state. Loop-owned; the
	// dispatcher applies snapshots from dbServer and advances CurrentIndex on
	// successful event drops.
	worldEvent EventConfig

	// Chat log buffer (chatlog.go). Loop-owned: lines are appended while
	// handling chat, and flushed in batches off the loop. chatEnviando keeps one
	// batch in flight at a time.
	chatBuf         []ChatLinha
	chatEnviando    bool
	chatUltimo      time.Time
	chatDescartadas int

	// Item-serial block (serial.go). Loop-owned: the numbers are handed out
	// while stamping items on save, which happens inside the loop, and refilled
	// off it. serialProximo == serialFim means the block is spent and items go
	// out unmarked until the next one lands.
	serialProximo int64
	serialFim     int64
	serialPedindo bool
	// marcavel decides which items are worth a serial. Injected because the rule
	// reads the item catalog (nPos, and the indexes the original game singled
	// out), which is content the world does not and should not know about.
	marcavel func(Item) bool

	// newbieEvent mirrors the legacy NewbieEventServer flag (Server.cpp:617).
	// The world itself only needs it for the spawn-time HP handicap; the EXP
	// side lives in the dispatcher's ExpEvents. Loop-owned.
	newbieEvent bool

	events    chan event
	callbacks chan event // async handler results (World.Go / World.GoDetached); separate
	// from events so a long mob-AI tick cannot block login/db callbacks on the main queue.
	done   chan struct{}  // closed when the loop stops; unblocks conn goroutines
	saveWG sync.WaitGroup // tracks in-flight async character saves (logout/disconnect)
	// savesFalhados conta as gravações de saída que NÃO confirmaram. O dreno do
	// painel lê isto: esperar as gravações terminarem não é o mesmo que elas terem
	// dado certo, e quem vai reiniciar precisa saber a diferença.
	savesFalhados atomic.Int64

	// onTick is the periodic simulation hook (mob AI), run inside the loop; see
	// tick.go. nil disables the ticker (e.g. in protocol/transport tests).
	onTick       func(*World)
	tickInterval time.Duration

	// onSessionEnd is the teardown hook (party unlink), run inside the loop just
	// before a session's slot is freed; see event.go. nil disables it.
	onSessionEnd func(*World, *Session)

	// respawnQueue holds dead monsters awaiting respawn, drained by SpawnDueRespawns
	// from the tick (world/respawn.go). Loop-owned.
	respawnQueue []respawnEntry

	// generators is the NPCGener block table (world/generator.go): spawn recipes
	// plus live CurrentNumMob accounting. mobCount tracks the live mob/NPC total
	// (the generateWorldCap gate). Loop-owned.
	generators []*Generator
	mobCount   int

	// respawnDelayFor, when set, replaces DefaultRespawnDelay for one
	// generator's dead monsters. It is a hook rather than a plain field because
	// the pacing is configured per AREA and read live: the handler owns that
	// configuration, and the world has no business knowing what a "deserto" is.
	// Loop-only, like everything else here.
	respawnDelayFor func(genIndex int32) uint32
}

// New creates a World with the given dependencies. A nil handler installs a
// no-op default (Phase 3: transport plumbing only).
func New(cfg Config, log *slog.Logger, persist Persistence, handler Handler) *World {
	if cfg.GridDim <= 0 {
		cfg.GridDim = DefaultGridDim
	}
	if cfg.OutBuffer <= 0 {
		cfg.OutBuffer = DefaultOutBuffer
	}
	if cfg.EventQueue <= 0 {
		cfg.EventQueue = 1024
	}
	if cfg.Now == nil {
		cfg.Now = func() uint32 { return uint32(time.Now().UnixMilli()) }
	}
	if log == nil {
		log = slog.Default()
	}
	if handler == nil {
		handler = func(*World, *Session, protocol.Header, []byte) {}
	}
	return &World{
		cfg:       cfg,
		log:       log,
		marcavel:  cfg.Marcavel,
		persist:   persist,
		billing:   AllowAllBilling{},
		handler:   handler,
		sessions:  make([]*Session, MaxUser),
		entities:  make([]*Entity, MaxMob),
		ground:    make([]*GroundItem, MaxItem),
		cargo:     make(map[int64]*CargoState),
		quitSaves: make(map[int64]int),

		deliveryPlaced:  make(map[int64]map[int64]bool),
		deliveryUnacked: make(map[int64][]int64),
		guilds:          make(map[uint16]GuildInfo),
		grid:            newGrid(cfg.GridDim),
		rng:             rng.New(),
		events:          make(chan event, cfg.EventQueue),
		callbacks:       make(chan event, 256),
		done:            make(chan struct{}),
	}
}

// Run is the single owner of world state. It processes inbound events until ctx
// is cancelled, then drains/saves active sessions and returns ctx.Err().
func (w *World) Run(ctx context.Context) error {
	w.log.Info("world loop started", "grid", w.cfg.GridDim)
	if w.onTick != nil && w.tickInterval > 0 {
		go w.runTicker(ctx)
	}
	for {
		select {
		case <-ctx.Done():
			w.announceShutdown()
			w.shutdown()
			return ctx.Err()
		case cb := <-w.callbacks:
			w.applyTimed(cb)
			continue
		default:
		}
		select {
		case <-ctx.Done():
			w.announceShutdown()
			w.shutdown()
			return ctx.Err()
		case cb := <-w.callbacks:
			w.applyTimed(cb)
		case ev := <-w.events:
			w.applyTimed(ev)
		}
	}
}

// slowEventThreshold is how long one loop event may take before it is logged:
// anything past this stalls every session at once (the loop is single-owner), so
// a "slow world event" WARN is the smoking gun for server-side global lag.
const slowEventThreshold = 100 * time.Millisecond

// applyTimed applies one loop event and warns when it exceeds slowEventThreshold,
// identifying the event (frame type/conn, tick, callback) so a stall can be
// traced to its handler (freeze investigation instrumentation).
func (w *World) applyTimed(ev event) {
	start := time.Now()
	w.applyRecovered(ev)
	d := time.Since(start)
	if d < slowEventThreshold {
		return
	}
	switch e := ev.(type) {
	case frameEvent:
		w.log.Warn("slow world event", "dur_ms", d.Milliseconds(), "kind", "frame", "conn", e.s.Conn, "type", formatSendType(e.header.Type))
	case tickEvent:
		w.log.Warn("slow world event", "dur_ms", d.Milliseconds(), "kind", "tick")
	case callbackEvent:
		w.log.Warn("slow world event", "dur_ms", d.Milliseconds(), "kind", "callback", "conn", e.conn)
	default:
		w.log.Warn("slow world event", "dur_ms", d.Milliseconds(), "kind", fmt.Sprintf("%T", ev))
	}
}

// applyRecovered runs one loop event and contains a handler panic to the session
// that caused it.
//
// Why this has to exist: the loop is the single owner of ALL world state, so an
// unrecovered panic does not merely fail one request — it takes the process down
// with every player on it. Handlers parse bytes straight off the client edge,
// where a malformed frame reaching an out-of-range index is the realistic
// failure mode, so the blast radius has to be one session and not the server.
//
// The trade-off is deliberate: a recovered handler may leave that session's
// state half-mutated, so the session is dropped rather than resumed. Corrupt
// state for one player beats a crash for everyone.
func (w *World) applyRecovered(ev event) {
	defer func() {
		r := recover()
		if r == nil {
			return
		}
		conn := -1
		var sess *Session
		switch e := ev.(type) {
		case frameEvent:
			sess = e.s
		case callbackEvent:
			sess, conn = e.sess, e.conn
		case disconnectEvent:
			sess = e.s
		}
		if sess != nil {
			conn = sess.Conn
		}
		w.log.Error("recovered panic in world loop",
			"kind", fmt.Sprintf("%T", ev),
			"conn", conn,
			"panic", fmt.Sprint(r),
			"stack", string(debug.Stack()))
		if sess == nil {
			return
		}
		// Teardown reads the same state the first panic may have corrupted, so it
		// can panic again; containing that keeps the loop alive either way.
		defer func() {
			if r2 := recover(); r2 != nil {
				w.log.Error("panic while dropping session after panic", "conn", conn, "panic", fmt.Sprint(r2))
			}
		}()
		w.removeSession(sess) // closes the socket as part of teardown
	}()
	ev.apply(w)
}

// shutdown drains active sessions: persist players in-world, then stop their I/O.
func (w *World) shutdown() {
	close(w.done) // signal conn goroutines to stop sending events
	saved := 0
	// O REINÍCIO SEGURO TAMBÉM GRAVA OS DOIS JUNTOS.
	//
	// Aqui eram duas varreduras em sequência: todos os personagens, depois todas as
	// cargas, cada uma na sua transação. A janela era pequena — o processo estaria
	// morrendo no meio do desligamento — mas era o mesmo espelho do dupe, e não há
	// motivo para deixá-la. Agora cada conta com personagem em jogo vai numa
	// transação só, e a segunda varredura cuida apenas das cargas que sobraram:
	// contas na tela de seleção, que não têm mochila viva para discordar delas.
	for _, s := range w.sessions {
		if s == nil {
			continue
		}
		// A MOCHILA VIVA, e não o modo: uma sessão apanhada em UserWaitDB — no meio
		// de uma ida ao banco — tem personagem carregado, e deixá-la de fora aqui
		// faria a carga dela ou ser gravada sozinha embaixo, ou não ser gravada.
		if w.temMochilaViva(s) {
			cs, carga, unacked, temCarga := w.parDeSalvamento(s)
			if err := SalvarPar(context.Background(), w.persist, cs, carga, temCarga, unacked); err != nil {
				w.log.Warn("save on shutdown failed", "conn", s.Conn, "err", err)
			} else {
				saved++
				if temCarga {
					delete(w.cargo, cs.AccountID)
					delete(w.deliveryPlaced, cs.AccountID)
					delete(w.deliveryUnacked, cs.AccountID)
				}
			}
		}
		s.close()
	}
	// As cargas que sobraram: contas carregadas SEM mochila viva — a conta na tela
	// de seleção —, onde não há personagem para discordar delas.
	//
	// A conta cujo par falhou acima fica de fora de propósito. Gravar a carga dela
	// sozinha recriaria as duas metades em desacordo. Ficando de fora, o banco
	// mantém o par ANTIGO inteiro, que é consistente: perde-se o progresso desde o
	// último save, mas nas duas metades JUNTAS, e nada duplica.
	for accountID := range w.cargo {
		if w.personagemVivoNaConta(accountID) != nil {
			continue
		}
		if err := saveCargoFor(context.Background(), w.persist, w.cargoSave(accountID), w.deliveryUnacked[accountID]); err != nil {
			w.log.Warn("save cargo on shutdown failed", "account", accountID, "err", err)
		}
	}
	// The last few seconds of chat, written straight rather than buffered: the
	// loop is ending, so there is no tick left to flush it and no reason to hand
	// it to a goroutine nobody will wait for.
	if len(w.chatBuf) > 0 {
		ctx, cancel := context.WithTimeout(context.Background(), chatTempo)
		if err := w.persist.RecordChat(ctx, w.chatBuf); err != nil {
			w.log.Warn("chat log: last batch lost on shutdown", "linhas", len(w.chatBuf), "err", err)
		}
		cancel()
		w.chatBuf = nil
	}
	// Wait for in-flight disconnect/logout saves so a shutdown never loses one.
	w.saveWG.Wait()
	w.log.Info("world loop stopped", "sessions_saved", saved)
}

// parDeSalvamento tira os DOIS instantâneos — o do personagem e o da carga da
// conta — no MESMO instante do laço, junto com as entregas ainda sem marca.
//
// Tirar os dois juntos é metade do conserto do dupe; a outra metade é gravá-los
// na mesma transação (salvarPar). Instantâneos tirados em momentos diferentes
// descrevem mundos diferentes, e nesse caso a transação única só congelaria a
// divergência em vez de evitá-la.
//
// temCarga é falso quando a conta não tem carga carregada na memória. Aí gravar
// só o personagem é seguro: não há carga em jogo para discordar dele. Loop-only.
func (w *World) parDeSalvamento(s *Session) (personagem CharacterSave, carga CargoSave, unacked []int64, temCarga bool) {
	personagem = w.characterSave(s)
	if s.AccountID == 0 || w.cargo[s.AccountID] == nil {
		return personagem, CargoSave{}, nil, false
	}
	return personagem, w.cargoSave(s.AccountID), append([]int64(nil), w.deliveryUnacked[s.AccountID]...), true
}

// SalvarPar grava personagem e carga na mesma transação, ou só o personagem
// quando a conta não tem carga carregada. Seguro fora do laço: só toca nos
// instantâneos que recebeu.
func SalvarPar(ctx context.Context, p Persistence, personagem CharacterSave, carga CargoSave,
	temCarga bool, unacked []int64,
) error {
	if !temCarga {
		return p.SaveOnShutdown(ctx, personagem)
	}
	return p.SalvarPersonagemComCarga(ctx, personagem, carga, unacked, nil)
}

// SalvarEncenadoComCarga grava um personagem ENCENADO — um instantâneo montado
// pelo handler, que ainda não foi publicado na entidade viva — junto com a carga
// da conta, na mesma transação, e depois chama depois() de volta no laço com o
// erro (nil quando deu certo).
//
// Existe para os caminhos "grava primeiro, publica depois": a Pedra Ideal, o Sub
// Celestial, a criação de guilda. Eles já gravavam o personagem sozinho, e essa
// era a mesma janela de sempre — a carga da memória pode ter ouro que o banco não
// viu. O instantâneo do personagem é o encenado; o da carga é o de agora.
// Loop-only.
func (w *World) SalvarEncenadoComCarga(s *Session, personagem CharacterSave, depois func(*World, *Session, error)) {
	carga, unacked, temCarga := w.CargaParaOPar(s.AccountID)
	p := w.persist
	w.Go(s, func() func(*World, *Session) {
		err := SalvarPar(context.Background(), p, personagem, carga, temCarga, unacked)
		return func(w *World, s *Session) {
			if err == nil && temCarga {
				w.forgetAcked(personagem.AccountID, unacked)
			}
			depois(w, s, err)
		}
	})
}

// CargaParaOPar tira o instantâneo da carga da conta para acompanhar um
// CharacterSave. É para quem grava fora do laço por conta própria (GoDetached) e
// não pode usar o SalvarEncenadoComCarga; junto com SalvarPar e EsqueceEntregues,
// dá a mesma garantia. Loop-only.
func (w *World) CargaParaOPar(accountID int64) (CargoSave, []int64, bool) {
	if accountID == 0 || w.cargo[accountID] == nil {
		return CargoSave{}, nil, false
	}
	return w.cargoSave(accountID), append([]int64(nil), w.deliveryUnacked[accountID]...), true
}

// SaveCharacterAsync persists an in-play character's live state (Carry/Coin/stats)
// without blocking the loop: it captures the CharacterSave in the loop (a value
// copy) and runs the gRPC save in a goroutine. Called on logout/disconnect so
// purchases, gold and progress survive the session. Loop-only (captures state).
func (w *World) SaveCharacterAsync(s *Session) {
	if s == nil || s.Mode != UserPlay || s.AccountID == 0 {
		return
	}
	w.salvarParAsync(s)
}

// salvarParAsync é o corpo do SaveCharacterAsync sem a exigência de UserPlay: ele
// grava o par de qualquer sessão com mochila viva. Loop-only.
func (w *World) salvarParAsync(s *Session) {
	cs, carga, unacked, temCarga := w.parDeSalvamento(s)
	p := w.persist
	w.saveWG.Add(1)
	go func() {
		defer w.saveWG.Done()
		if err := SalvarPar(context.Background(), p, cs, carga, temCarga, unacked); err != nil {
			w.savesFalhados.Add(1)
			w.log.Warn("save character failed", "account", cs.AccountID, "slot", cs.Slot, "err", err)
			return
		}
		if temCarga && len(unacked) > 0 {
			w.GoDetached(func() func(*World) {
				return func(w *World) { w.forgetAcked(cs.AccountID, unacked) }
			})
		}
	}()
}

// LeaveCharacter persists a character that is leaving play and, ONLY AFTER that
// save commits, clears its presence mark.
//
// The order is the whole point, and it is what makes presence worth having on
// top of the control API's ListOnline. Kick returns the moment the session
// closes, but the character's save leaves after that; a panel that treated "no
// longer connected" as "safe to edit" would write on top of a save still in
// flight and lose the edit. Both calls therefore share one goroutine,
// sequentially, instead of being two async hops that can land in either order.
//
// A failed save deliberately KEEPS the mark: a character whose last write did
// not land is exactly the one nobody should be editing.
//
// Safe to call for a session that never entered play: nothing to save, nothing
// to clear.
func (w *World) LeaveCharacter(s *Session) {
	if s == nil || s.Mode != UserPlay || s.AccountID == 0 {
		return
	}
	e := w.entities[s.Conn]
	if e == nil {
		return
	}
	name := e.Name
	cs, carga, unacked, temCarga := w.parDeSalvamento(s)
	if temCarga {
		// A CARGA SAI DA MEMÓRIA AQUI, junto com o instantâneo. O caminho de saída
		// chamava LeaveCharacter e ReleaseCargo em seguida, cada um com a sua
		// goroutine e a sua transação — que é exatamente a janela do dupe. Agora a
		// dupla vai numa transação só, e o ReleaseCargo que vem depois não acha mais
		// carga para gravar e não faz nada.
		//
		// SÓ QUE A CARGA É DA CONTA, não do personagem: se outra sessão da mesma
		// conta ainda estiver com personagem carregado, tirá-la da memória aqui
		// arrancaria o baú debaixo de quem continua jogando. Hoje isto só é chamado
		// na desconexão, então não acontece; se acontecer, é erro alto e a carga
		// fica, porque uma carga sumida calada é muito pior.
		if outra := w.outraSessaoComMochilaViva(cs.AccountID, s); outra != nil {
			w.log.Error("LeaveCharacter com outra sessão viva na conta: a carga NÃO foi retirada da memória",
				"account", cs.AccountID, "saindo", s.Conn, "ficando", outra.Conn)
			temCarga = false
			carga = CargoSave{}
			unacked = nil
		} else {
			delete(w.cargo, cs.AccountID)
			delete(w.deliveryPlaced, cs.AccountID)
			delete(w.deliveryUnacked, cs.AccountID)
		}
	}
	p := w.persist
	w.holdAccount(cs.AccountID)
	w.saveWG.Add(1)
	go func() {
		defer w.saveWG.Done()
		defer w.releaseAccountLater(cs.AccountID)
		if err := SalvarPar(context.Background(), p, cs, carga, temCarga, unacked); err != nil {
			w.savesFalhados.Add(1)
			w.log.Warn("save character failed", "account", cs.AccountID, "slot", cs.Slot, "err", err)
			return
		}
		if name == "" {
			return
		}
		if err := p.SetCharacterPresence(context.Background(), name, false); err != nil {
			w.log.Warn("clear presence failed", "character", name, "err", err)
		}
	}()
}

// SaveCharacterThen persists the character and runs then (back in the loop) only
// after the save commits. Use it where the client may immediately re-read the
// character from the DB (logout to character selection): deferring the
// confirmation until the save lands prevents the reload racing the write. then
// always runs, even when there is nothing to save.
func (w *World) SaveCharacterThen(s *Session, then func(*World, *Session)) {
	if s == nil || s.Mode != UserPlay || s.AccountID == 0 {
		then(w, s)
		return
	}
	cs, carga, unacked, temCarga := w.parDeSalvamento(s)
	p := w.persist
	w.Go(s, func() func(*World, *Session) {
		err := SalvarPar(context.Background(), p, cs, carga, temCarga, unacked)
		return func(w *World, s *Session) {
			if err != nil {
				w.log.Warn("save character failed", "account", cs.AccountID, "slot", cs.Slot, "err", err)
			} else if temCarga {
				w.forgetAcked(cs.AccountID, unacked)
			}
			then(w, s)
		}
	})
}

// characterSave snapshots a session's in-world entity into a CharacterSave. Only
// world-authoritative fields are captured (see CharacterSave). Loop-only.
func (w *World) characterSave(s *Session) CharacterSave {
	e := w.entities[s.Conn]
	return w.CharacterSaveFor(s, e)
}

// CharacterSaveFor snapshots a staged entity without publishing it to the world.
// Handlers use it for persistence-first operations such as kingdom cape purchases.
func (w *World) CharacterSaveFor(s *Session, e *Entity) CharacterSave {
	cs := CharacterSave{AccountID: s.AccountID, Slot: s.Slot}
	if e == nil {
		return cs
	}
	cs.Clan, cs.GuildID, cs.GuildLevel, cs.Soul, cs.Fame = e.Clan, e.Guild, e.GuildLevel, e.Soul, e.Fame
	cs.ClassMaster = e.ClassMaster
	cs.CelLv40, cs.CelLv90, cs.CelCircle = e.CelLv40, e.CelLv90, e.CelCircle
	cs.ArchLv355, cs.ArchLv370 = e.ArchLv355, e.ArchLv370
	cs.MortalLevel, cs.CelestialArchLevel, cs.ArchCristal = e.MortalLevel, e.CelestialArchLevel, e.ArchCristal
	cs.NightmareTickets, cs.KefraTicket = e.NightmareTickets, e.KefraTicket
	cs.SubCelestialGuardada, cs.SubCelestialLevel = e.SubCelestialGuardada, e.SubCelestialLevel
	cs.SubCelestialAtivo, cs.CelestialReset = e.SubCelestialAtivo, e.CelestialReset
	cs.TerraMistica = e.TerraMistica
	cs.NewbieQuest = e.NewbieQuest
	cs.MolarGargula = e.MolarGargula
	cs.Citizen = e.Citizen
	cs.LastCity = e.LastCity
	cs.SaveX, cs.SaveY = e.SaveX, e.SaveY
	cs.Level, cs.Exp, cs.Coin = e.Level, e.Exp, e.Coin
	cs.Str, cs.Int, cs.Dex, cs.Con = e.Str, e.Int, e.Dex, e.Con
	cs.HP, cs.MaxHP = e.HP, e.MaxHP
	cs.MP, cs.MaxMP = e.MP, e.MaxMP
	cs.DivineEnd = e.DivineEnd // 0 once the buff has expired (cleared by the tick sweep)
	cs.ScoreBonus, cs.SpecialBonus = e.ScoreBonus, e.SpecialBonus
	cs.LearnedSkill, cs.SecLearnedSkill, cs.BaseSpecial = e.LearnedSkill, e.SecLearnedSkill, e.BaseSpecial
	cs.SkillBar, cs.ShortSkill = e.SkillBar, s.ShortSkill
	cs.PKPoint, cs.Guilty, cs.CurKill, cs.TotKill = e.PKPoint, e.Guilty, e.CurKill, e.TotKill
	for _, a := range e.Affect {
		// Divine persists separately as the wall-clock DivineEnd; empty slots drop.
		if a.Type == 0 || a.Type == AffectDivine {
			continue
		}
		cs.Affects = append(cs.Affects, a)
	}
	cs.Carry = w.savedItems(e.Carry[:])
	cs.Equip = w.savedItems(e.Equip[:])
	return cs
}

// savedItems flattens a positional item array into the non-empty SavedItem
// slots, stamping a serial on anything that deserves one and lacks it.
//
// SAVE IS THE CHOKEPOINT, and that is why the stamping lives here rather than
// at the twenty-odd places an item can be created. Every one of those paths
// ends at a save eventually, so one place catches them all — including the
// items that already existed before any of this, which get an identity the
// first time their owner logs out.
//
// It writes the serial back into the live array (items is a slice over the
// entity's own storage, not a copy) so the number stays with the item for the
// rest of the session. Without that the next save would mint a second number
// for the same item, and the original and its copy would never match.
//
// Loop-only.
func (w *World) savedItems(items []Item) []SavedItem {
	var out []SavedItem
	for i := range items {
		if items[i].Empty() {
			continue
		}
		items[i] = w.marcar(items[i])
		it := items[i]
		out = append(out, SavedItem{
			Slot:  i,
			Index: it.Index,
			Eff1:  it.Effects[0].Effect, EffV1: it.Effects[0].Value,
			Eff2: it.Effects[1].Effect, EffV2: it.Effects[1].Value,
			Eff3: it.Effects[2].Effect, EffV3: it.Effects[2].Value,
			ExpiresAt:  it.ExpiresAt,
			Serial:     it.Serial,
			AnuncioRMT: it.AnuncioRMT,
		})
	}
	return out
}

// Cargo returns the account's loaded warehouse, or nil if none is loaded (e.g.
// the account is not logged in, or LoadCargo failed). Loop-only.
func (w *World) Cargo(accountID int64) *CargoState {
	if accountID == 0 {
		return nil
	}
	return w.cargo[accountID]
}

// SetCargo installs an account's loaded warehouse in the store. Called from the
// loop right after a successful account login. Loop-only.
func (w *World) SetCargo(accountID int64, st *CargoState) {
	if accountID == 0 || st == nil {
		return
	}
	st.AccountID = accountID
	w.cargo[accountID] = st
}

// ApplyDeliveries places the account's already-fetched pending delivery_queue
// grants (donate web shop, issue #34) into its cargo. pending is loaded in the
// same off-loop round-trip as the account login itself (dbclient.AccountLogin),
// so draining the mailbox costs no extra backend round-trip. Called from the loop
// right after the cargo is installed at login: places each item in the next free
// cargo slot and persists the cargo + acks the delivered queue rows in one
// backend transaction, off the loop. Loop-only.
//
// A grant that finds no free slot is HELD, not dropped: its row stays 'pending'
// and the next login or deliver-now tries it again. These are paid items. One
// that vanished because the warehouse happened to be full is a card
// chargeback, and chargebacks in volume get the merchant account closed —
// waiting for room costs nothing by comparison.
//
// It returns how the mailbox split — delivered into the warehouse, and held for
// want of a free slot. The admin panel's deliver-now reports both, so a
// moderator can tell the player to make room instead of reporting a delivery
// that did not happen.
// MensagemEntregaPresa é o que o jogador lê quando parte da entrega não coube.
//
// UMA FUNÇÃO E NÃO DUAS FRASES, e o motivo é concreto: os dois caminhos que drenam a
// caixa postal — o login e a entrega imediata pedida pelo site — avisavam com textos
// diferentes, e um deles não avisava nada. Texto de jogador escrito em dois lugares
// vira dois textos, e o que se corrige num dia continua errado no outro.
//
// E ela diz O QUE FAZER, porque o aviso sem a ação vira chamado: "não couberam" sozinho
// faz a pessoa contar os itens, achar que sumiu, e abrir ticket. Dói mais no pacote
// grande, que é justamente o que enche o baú — o maior deles ocupa 69 dos 128 espaços,
// porque baú de sorteio não empilha.
//
// "Entre de novo" é o certo nos dois lugares: o que ficou preso é retentado no próximo
// dreno, e os drenos são o login e a entrega imediata. Não há terceiro.
func MensagemEntregaPresa(presos int) string {
	if presos <= 0 {
		return ""
	}
	return fmt.Sprintf("%d item(ns) nao couberam no bau da conta. "+
		"Libere espaco e entre de novo para receber o resto.", presos)
}

func (w *World) ApplyDeliveries(s *Session, pending []Delivery) (delivered, held int) {
	if s == nil || s.AccountID == 0 || len(pending) == 0 {
		return 0, 0
	}
	accountID := s.AccountID
	cargo := w.cargo[accountID]
	if cargo == nil {
		return 0, 0
	}
	placed := w.deliveryPlaced[accountID]
	if placed == nil {
		placed = make(map[int64]bool)
		w.deliveryPlaced[accountID] = placed
	}
	var deliveredIDs []int64
	for _, d := range pending {
		if placed[d.ID] {
			// Already in this cargo from an earlier drain whose list was fetched
			// first; its ack may simply not have committed yet.
			continue
		}
		if w.AddToCargo(cargo, d.Item) >= 0 {
			deliveredIDs = append(deliveredIDs, d.ID)
			placed[d.ID] = true
		} else {
			held++
		}
	}
	if held > 0 {
		w.log.Warn("donate deliveries held: cargo full", "account", accountID, "count", held)
	}
	w.log.Info("drained donate deliveries", "account", accountID, "delivered", len(deliveredIDs), "held", held)
	if len(deliveredIDs) == 0 {
		// Nothing moved: the cargo is unchanged and every row is still pending.
		return 0, held
	}

	w.deliveryUnacked[accountID] = append(w.deliveryUnacked[accountID], deliveredIDs...)
	w.saveCargoAcking(accountID)
	return len(deliveredIDs), held
}

// LimpaSlotsVendidos esvazia os slots do baú cujo anúncio em dinheiro real já foi
// vendido, e devolve quantos saíram. Loop-only.
//
// É a outra ponta da preguiça do escrow. Quando o pagamento entra, o item NÃO sai
// do baú na hora: a marca (migração 0104) fica, e item marcado é intocável — não
// se move, não se altera, não se refina, não se vende. Então tirá-lo pode esperar
// o vendedor aparecer, e é por isso que o comprador nunca espera pelo vendedor:
// o que ele recebe vem da fotografia do anúncio.
//
// ESVAZIA, e nunca devolve. É a diferença entre esta marca e a de um anúncio
// cancelado: aquele item volta para as mãos do dono, este já foi pago e entregue
// a outra pessoa. Devolver aqui criaria a segunda cópia que o escrow inteiro
// existe para impedir.
//
// A CONFERÊNCIA DA MARCA antes de apagar não é zelo: a lista foi lida FORA do
// laço, e entre a leitura e esta passada o slot pode ter mudado de dono lógico.
// Um slot vazio, ou com item sem marca, é deixado em paz — apagar um item que
// não era o vendido seria tirar do jogador uma coisa que ele nunca vendeu.
func (w *World) LimpaSlotsVendidos(s *Session, slots []int16) int {
	if s == nil || s.AccountID == 0 || len(slots) == 0 {
		return 0
	}
	cargo := w.cargo[s.AccountID]
	if cargo == nil {
		return 0
	}
	saiu := 0
	for _, slot := range slots {
		if slot < 0 || int(slot) >= MaxCargo {
			continue
		}
		it := cargo.Items[slot]
		if it.Empty() || it.AnuncioRMT == 0 {
			// Já foi tirado numa passada anterior cujo save ainda não tinha
			// chegado ao banco quando esta lista foi lida. Repetir é normal.
			continue
		}
		w.log.Info("escrow: item vendido sai do bau", "account", s.AccountID,
			"slot", slot, "anuncio", it.AnuncioRMT, "item", it.Index)
		cargo.Items[slot] = Item{}
		saiu++
	}
	if saiu > 0 {
		w.saveCargoAcking(s.AccountID)
	}
	return saiu
}

// SoltaMarcasDeEscrow tira do baú da CONTA as marcas dos slots dados, devolvendo
// o item ao dono.
//
// É a operação oposta à LimpaSlotsVendidos, e a diferença é tudo: aquela ESVAZIA
// o slot, porque o item já foi pago e entregue a outra pessoa; esta só tira o
// cadeado, porque a venda não aconteceu e o item nunca deixou de ser do dono.
// Trocar uma pela outra apagaria o item de alguém que não vendeu nada.
//
// Trabalha por CONTA e não por sessão: o vendedor pode estar na tela de
// personagem quando isto roda, e o baú é da conta de qualquer forma. Se ele nem
// estiver em jogo, não há o que fazer aqui — a marca continua no banco e a faxina
// do login a encontra.
//
// Só solta slot que AINDA carrega a marca daquele anúncio. A lista vem de fora do
// laço e pode chegar depois de o slot ter mudado; soltar um cadeado que já é de
// outro anúncio libertaria um item que está à venda agora.
func (w *World) SoltaMarcasDeEscrow(accountID int64, marcas map[int16]int64) int {
	if accountID == 0 || len(marcas) == 0 {
		return 0
	}
	cargo := w.cargo[accountID]
	if cargo == nil {
		return 0
	}
	soltos := 0
	for slot, anuncio := range marcas {
		if slot < 0 || int(slot) >= MaxCargo {
			continue
		}
		it := cargo.Items[slot]
		if it.Empty() || it.AnuncioRMT != anuncio {
			continue
		}
		w.log.Info("escrow: cadeado solto, o item volta ao dono", "account", accountID,
			"slot", slot, "anuncio", anuncio, "item", it.Index)
		cargo.Items[slot].AnuncioRMT = 0
		soltos++
	}
	if soltos > 0 {
		w.saveCargoAcking(accountID)
	}
	return soltos
}

// SoltaMarcasMortas tira as marcas dos slots que a faxina apontou, sem conferir
// contra qual anúncio elas apontam.
//
// A conferência que a SoltaMarcasDeEscrow faz não serve aqui, e é por um motivo
// e não por preguiça: a faxina responde "este slot tem cadeado morto", e o id do
// anúncio morto é justamente o que o laço não conhece. O que protege esta versão
// é a origem da lista — ela vem de uma consulta que já excluiu todo anúncio vivo,
// vendido e com cobrança aberta.
func (w *World) SoltaMarcasMortas(accountID int64, slots []int16) int {
	if accountID == 0 || len(slots) == 0 {
		return 0
	}
	cargo := w.cargo[accountID]
	if cargo == nil {
		return 0
	}
	soltos := 0
	for _, slot := range slots {
		if slot < 0 || int(slot) >= MaxCargo {
			continue
		}
		it := cargo.Items[slot]
		if it.Empty() || it.AnuncioRMT == 0 {
			continue
		}
		w.log.Info("escrow: faxina soltou um cadeado morto", "account", accountID,
			"slot", slot, "anuncio", it.AnuncioRMT, "item", it.Index)
		cargo.Items[slot].AnuncioRMT = 0
		soltos++
	}
	if soltos > 0 {
		w.saveCargoAcking(accountID)
	}
	return soltos
}

// SalvaCargo grava o baú da conta agora, sem esperar o próximo momento de save.
//
// Existe para encurtar janelas: quando o laço acabou de escrever no baú uma coisa
// cuja contraparte já está no banco — a marca de um anúncio recém-criado, por
// exemplo —, cada segundo entre as duas é um segundo em que uma queda deixa as
// duas metades em desacordo. Loop-only.
func (w *World) SalvaCargo(accountID int64) { w.saveCargoAcking(accountID) }

// saveCargoAcking persists the account cargo together with every placed-but-
// unacked delivery id, in one transaction, and forgets the ids once it commits.
// A failed save keeps them, so the next cargo save of the account — another
// drain, a character switch, the logout — tries the pair again. Loop-only.
func (w *World) saveCargoAcking(accountID int64) {
	// SE A CONTA TEM PERSONAGEM EM JOGO, o personagem vai junto. Gravar só a carga
	// enquanto o dono está jogando é a metade que duplica: o item que saiu da carga
	// já está na mochila da memória, e a mochila do banco ainda não sabe.
	if s := w.personagemVivoNaConta(accountID); s != nil {
		// salvarParAsync, e não SaveCharacterAsync: aquele recusa sessão fora de
		// UserPlay, e aqui a sessão pode estar em UserWaitDB. Cair no guard faria
		// esta gravação simplesmente não acontecer — pior que o dupe, porque some.
		w.salvarParAsync(s)
		return
	}
	cs := w.cargoSave(accountID)
	ids := append([]int64(nil), w.deliveryUnacked[accountID]...)
	p := w.persist
	w.GoDetached(func() func(*World) {
		e := p.SaveCargoWithDeliveries(context.Background(), cs, ids, nil)
		return func(w *World) {
			if e != nil {
				w.log.Warn("save cargo with deliveries failed", "account", accountID, "err", e)
				return
			}
			w.forgetAcked(accountID, ids)
		}
	})
}

// personagemVivoNaConta devolve a sessão desta conta que tem MOCHILA VIVA na
// memória, ou nil. É ela que decide se uma gravação de carga precisa levar o
// personagem junto.
//
// O CRITÉRIO É A ENTIDADE, E NÃO O MODO, e a diferença é um dupe.
//
// A primeira versão perguntava por Mode == UserPlay. Só que o personagem continua
// vivo na memória em UserWaitDB, que é justamente o modo de quem está no meio de
// uma ida ao banco — criar guilda, promover, e as outras operações em w.Go. Uma
// entrega que chegasse nessa janela veria "ninguém jogando" e gravaria a carga
// SOZINHA, com a mochila do banco atrasada. É a janela mais provável de todas: a
// pessoa saca da carga e cria a guilda. O mesmo valeria para o UserCharWait se
// houver mochila viva nele.
//
// Loop-only.
func (w *World) personagemVivoNaConta(accountID int64) *Session {
	if accountID == 0 {
		return nil
	}
	for _, s := range w.sessions {
		if s != nil && s.AccountID == accountID && w.temMochilaViva(s) {
			return s
		}
	}
	return nil
}

// outraSessaoComMochilaViva é a personagemVivoNaConta ignorando uma sessão — a que
// está saindo. Loop-only.
func (w *World) outraSessaoComMochilaViva(accountID int64, exceto *Session) *Session {
	for _, s := range w.sessions {
		if s != nil && s != exceto && s.AccountID == accountID && w.temMochilaViva(s) {
			return s
		}
	}
	return nil
}

// temMochilaViva diz se a sessão tem um personagem carregado cuja mochila pode
// discordar da carga da conta.
//
// Entidade DOCADA não conta: ela é o resto de um personagem que já voltou para a
// tela de seleção e já foi gravado. Gravá-la de novo seria republicar estado
// velho por cima de um instantâneo mais novo. Loop-only.
func (w *World) temMochilaViva(s *Session) bool {
	if s == nil || s.AccountID == 0 {
		return false
	}
	e := w.entities[s.Conn]
	return e != nil && e.Mode != MobUserDock
}

// EsqueceEntregues é o forgetAcked para quem grava o par por conta própria: as
// marcas só saem da lista depois que a transação que as escreveu confirmou.
// Loop-only.
func (w *World) EsqueceEntregues(accountID int64, ids []int64) { w.forgetAcked(accountID, ids) }

// forgetAcked drops ids from the account's unacked list after their mark
// committed. Loop-only.
func (w *World) forgetAcked(accountID int64, ids []int64) {
	pend := w.deliveryUnacked[accountID]
	if len(pend) == 0 {
		return
	}
	done := make(map[int64]bool, len(ids))
	for _, id := range ids {
		done[id] = true
	}
	kept := pend[:0]
	for _, id := range pend {
		if !done[id] {
			kept = append(kept, id)
		}
	}
	if len(kept) == 0 {
		delete(w.deliveryUnacked, accountID)
		return
	}
	w.deliveryUnacked[accountID] = kept
}

// saveCargoFor is the cargo write every other path uses: the plain SaveCargo,
// or — when the account has placed deliveries whose mark has not committed —
// SaveCargoWithDeliveries with those ids, so the items never reach the database
// without their rows leaving 'pending'. Safe off the loop: it only touches the
// snapshot and ids it is given.
func saveCargoFor(ctx context.Context, p Persistence, cs CargoSave, unacked []int64) error {
	if len(unacked) == 0 {
		return p.SaveCargo(ctx, cs)
	}
	return p.SaveCargoWithDeliveries(ctx, cs, unacked, nil)
}

// cargoSave snapshots an account's warehouse into a CargoSave. Loop-only.
func (w *World) cargoSave(accountID int64) CargoSave {
	cs := CargoSave{AccountID: accountID}
	if c := w.cargo[accountID]; c != nil {
		cs.Coin = c.Coin
		cs.Items = w.savedItems(c.Items[:])
	}
	return cs
}

// ReleaseCargo persists an account's warehouse and removes it from the store. It
// is called when the account session ends (disconnect/logout) so the in-memory
// vault does not leak and the latest deposits survive. The save runs off the loop
// (tracked by saveWG, like SaveCharacterAsync) so a shutdown never loses it.
// Loop-only (snapshots state before going async).
func (w *World) ReleaseCargo(accountID int64) {
	if accountID == 0 || w.cargo[accountID] == nil {
		return
	}
	cs := w.cargoSave(accountID)
	// The cargo leaves memory, so the drain bookkeeping goes with it: this save
	// either commits the cargo AND the pending marks, or neither — and a next
	// login then finds the rows 'pending' and the items not in the saved cargo,
	// and delivers them exactly once.
	unacked := w.deliveryUnacked[accountID]
	delete(w.cargo, accountID)
	delete(w.deliveryPlaced, accountID)
	delete(w.deliveryUnacked, accountID)
	w.holdAccount(accountID)
	w.saveWG.Add(1)
	go func() {
		defer w.saveWG.Done()
		defer w.releaseAccountLater(accountID)
		if err := saveCargoFor(context.Background(), w.persist, cs, unacked); err != nil {
			w.savesFalhados.Add(1)
			w.log.Warn("save cargo failed", "account", cs.AccountID, "err", err)
		}
	}()
}

// holdAccount counts one more teardown save of accountID in flight. Loop-only.
func (w *World) holdAccount(accountID int64) {
	if accountID != 0 {
		w.quitSaves[accountID]++
	}
}

// releaseAccountLater undoes one holdAccount once the save has returned, landed
// or failed. It is called from the save goroutine, so the decrement rides back
// into the loop. A failed save releases too: the write that did not land is
// lost either way, and holding the account would lock its owner out until a
// restart. (The presence mark is what records the failure — LeaveCharacter.)
func (w *World) releaseAccountLater(accountID int64) {
	if accountID == 0 {
		return
	}
	w.GoDetached(func() func(*World) {
		return func(w *World) {
			if w.quitSaves[accountID]--; w.quitSaves[accountID] <= 0 {
				delete(w.quitSaves, accountID)
			}
		}
	})
}

// AccountSession returns the session that already holds accountID — one past
// account login, at the character screen or in play — other than except, or
// nil. Loop-only.
func (w *World) AccountSession(accountID int64, except *Session) *Session {
	if accountID == 0 {
		return nil
	}
	for _, s := range w.sessions {
		if s != nil && s != except && s.AccountID == accountID {
			return s
		}
	}
	return nil
}

// AccountSaving reports whether a closed session of accountID still has
// teardown saves in flight. Loop-only.
func (w *World) AccountSaving(accountID int64) bool {
	return w.quitSaves[accountID] > 0
}

// send queues an outbound message to the session's writer goroutine. It never
// blocks the loop: if the session's queue is full (a slow/stuck client), the
// session is dropped instead of stalling the whole world (head-of-line safety).
func (w *World) enqueue(s *Session, h protocol.Header, payload []byte) {
	if w.hiddenFrom(s, h, payload) {
		return
	}
	h.ClientTick = w.cfg.Now()
	s.noteSend(h, len(payload))
	if w.cfg.LogSends {
		w.log.Info("send packet", "conn", s.Conn, "type", formatSendType(h.Type), "id", h.ID, "len", len(payload))
	}
	select {
	case s.out <- outFrame{header: h, payload: payload}:
		// Track writer backpressure: a growing queue means the socket (or the
		// client) is not draining what the loop produces. Warn on each new
		// high-water mark past half capacity — bounded, and it precedes the
		// hard "queue full" drop below.
		if d := len(s.out); d > s.outHighWater {
			s.outHighWater = d
			if d >= w.cfg.OutBuffer/2 {
				w.log.Warn("session out queue high", "conn", s.Conn, "depth", d, "cap", w.cfg.OutBuffer)
			}
		}
	default:
		w.log.Warn("session out queue full; dropping connection", "conn", s.Conn)
		w.dropSession(s)
	}
}

// SessionMode returns a session's mode (test/observability helper; loop-only).
func (w *World) SessionMode(conn int) (Mode, bool) {
	if conn < 0 || conn >= MaxUser || w.sessions[conn] == nil {
		return UserEmpty, false
	}
	return w.sessions[conn].Mode, true
}

// SkipCheckTick is the sentinel a client sends when its own tick must not be
// used for timing checks (Basedef.h:232). The original replaces it with server
// time; any other value it echoes back untouched.
const SkipCheckTick = 235543242

// SendEcho queues a frame that must reach the client carrying the ClientTick it
// ARRIVED with, rather than the server clock every other message is stamped with.
//
// The attack reply needs this. The original edits the client's own frame in place
// and multicasts that same buffer, replacing the tick only when it is the
// SKIPCHECKTICK sentinel (_MSG_Attack.cpp:1745-1750) — so an attack comes back
// stamped with the tick the client sent. That is how the client recognises the
// reply as the answer to its OWN swing and takes the attacker status out of it.
// Stamped with server time it reads as somebody else's attack: the damage still
// lands, but the experience in it is not the reader's to take, which is why the
// exp bar never moved and no gain floated. Loop-only, like enqueue.
func (w *World) SendEcho(s *Session, h protocol.Header, payload []byte) {
	if w.hiddenFrom(s, h, payload) {
		return
	}
	if h.ClientTick == 0 || h.ClientTick == SkipCheckTick {
		h.ClientTick = w.cfg.Now()
	}
	s.noteSend(h, len(payload))
	if w.cfg.LogSends {
		w.log.Info("send packet", "conn", s.Conn, "type", formatSendType(h.Type), "id", h.ID, "len", len(payload))
	}
	select {
	case s.out <- outFrame{header: h, payload: payload}:
	default:
		w.log.Warn("session out queue full; dropping connection", "conn", s.Conn)
		w.dropSession(s)
	}
}

// ActiveSessions counts non-nil sessions (loop-only helper).
func (w *World) ActiveSessions() int {
	n := 0
	for _, s := range w.sessions {
		if s != nil {
			n++
		}
	}
	return n
}
