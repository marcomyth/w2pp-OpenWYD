// Command adminserver is the staff admin panel: account search, role, VIP and
// audit, served as plain HTTP with an embedded UI.
//
// It is deliberately a SEPARATE service, not a surface bolted onto webServer.
// The game services run in production with players connected; a panel that lives
// inside one of them cannot be rolled back without rolling back the game too.
// Standalone, the whole feature is deletable: nothing in tmServer, dbServer,
// binServer or webServer imports it or knows it exists, and the only shared
// resource is the database — where every change this feature needs is additive.
//
// Plain HTTP rather than gRPC for the same reason. gRPC is right for the internal
// links, but this endpoint is opened in a browser, and browsers do not speak it —
// gRPC here would mean grpc-web plus a proxy, or a second service whose only job
// is translation. Both cost more than they return for a staff-only panel.
//
// It does NOT run migrations. dbServer and webServer share store.Migrate and
// whichever boots first brings the schema up; the panel only reads a schema it
// does not own, which is what keeps deleting the service a complete undo.
//
// Usage:
//
//	adminserver [-addr :8080] -dsn <postgres-url> [-session-ttl 2h] [-insecure-cookies]
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"

	webv1 "github.com/jeanluca/w2pp-openwyd/api/web/v1"

	"github.com/jeanluca/w2pp-openwyd/adminserver/internal/accounts"
	"github.com/jeanluca/w2pp-openwyd/adminserver/internal/audit"
	"github.com/jeanluca/w2pp-openwyd/adminserver/internal/donate"
	"github.com/jeanluca/w2pp-openwyd/adminserver/internal/entrega"
	"github.com/jeanluca/w2pp-openwyd/adminserver/internal/gamedata"
	"github.com/jeanluca/w2pp-openwyd/adminserver/internal/jogo"
	"github.com/jeanluca/w2pp-openwyd/adminserver/internal/panel"
	"github.com/jeanluca/w2pp-openwyd/adminserver/internal/personagem"
	"github.com/jeanluca/w2pp-openwyd/adminserver/internal/plataforma"
	"github.com/jeanluca/w2pp-openwyd/adminserver/internal/session"
	"github.com/jeanluca/w2pp-openwyd/adminserver/internal/siteapi"
	"github.com/jeanluca/w2pp-openwyd/internal/acesso"
	"github.com/jeanluca/w2pp-openwyd/internal/npcgener"
	"github.com/jeanluca/w2pp-openwyd/internal/npctemplate"
	"github.com/jeanluca/w2pp-openwyd/internal/secret"
	"github.com/jeanluca/w2pp-openwyd/internal/store"
)

// Shutdown budget. Long enough to finish an in-flight request, short enough that
// the platform's own stop timeout never fires first and turns a clean stop into a
// kill.
const shutdownTimeout = 10 * time.Second

// HTTP server timeouts. Go's defaults are unlimited, which on a public endpoint
// lets a slow or abandoned client hold a connection open indefinitely — and this
// service, unlike the internal ones, is reachable from the internet.
const (
	readHeaderTimeout = 5 * time.Second
	readTimeout       = 15 * time.Second
	writeTimeout      = 30 * time.Second
	idleTimeout       = 60 * time.Second
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	// O VERBO VEM ANTES DAS FLAGS, e é lido à mão de propósito: o flag padrão do Go pararia
	// no primeiro argumento que não começa com "-", e um subcomando misturado com as flags
	// do servidor faria "criar-usuario" virar um servidor subindo com argumento estranho.
	if len(os.Args) > 1 && os.Args[1] == criarUsuarioCmd {
		if err := criarUsuario(logger, os.Args[2:], os.Stdin); err != nil {
			logger.Error(criarUsuarioCmd+" falhou", "err", err)
			os.Exit(1)
		}
		return
	}
	if err := run(logger); err != nil {
		logger.Error("adminserver failed", "err", err)
		os.Exit(1)
	}
}

// run serves HTTP until the process receives SIGINT/SIGTERM, then drains.
func run(logger *slog.Logger) error {
	addr := flag.String("addr", defaultAddr(), "HTTP listen address")
	dsn := flag.String("dsn", envOr("DATABASE_URL", os.Getenv("W2PP_DB_DSN")), "PostgreSQL DSN (or DATABASE_URL)")
	sessionTTL := flag.Duration("session-ttl", 2*time.Hour, "how long a staff session stays valid")
	// Browsers drop a Secure cookie sent over plain HTTP, so local development
	// on http://localhost cannot log in without this. Off by default: the flag
	// has to be asked for, never assumed.
	insecureCookies := flag.Bool("insecure-cookies", false, "omit the Secure flag on the session cookie (local HTTP only)")
	// Optional. Without it the item pages are hidden rather than broken, so the
	// panel still runs against nothing but the database.
	webAddr := flag.String("webserver", os.Getenv("W2PP_WEBSERVER"), "webServer gRPC address for the item pages (empty = hide them)")
	jogoAddr := flag.String("tmserver", os.Getenv("W2PP_TMSERVER_CONTROL"), "tmServer control address for the live pages: who is online, kick, notice (empty = hide them). Needs W2PP_CONTROL_TOKEN to match the tmServer's")
	// The player site's API. A second listener, meant for the private network
	// only; empty leaves it off and the panel exactly as it was.
	siteAddr := flag.String("site-api", os.Getenv("SITE_API_ADDR"), "private listen address for the player site's API, e.g. :8090 (empty = off). Needs W2PP_PAINEL_TOKEN_SITE")
	// The content tree, for the block recipes (Zonas de caça): the form starts
	// from NPCGener.txt and checks each name against npc/. The image bakes it at
	// /Release, which is the default; empty or missing hides the recipe pages.
	contentDir := flag.String("content", envOr("W2PP_CONTENT", "/Release"), "game content tree (Release/) for the block recipe pages (empty = hide them)")
	flag.Parse()

	// A MESMA tranca dos outros dois, pelo mesmo pacote. Valor desconhecido não sobe:
	// um painel que sobe com a criação de conta aberta, num servidor que deveria estar
	// trancado, é o erro que ninguém procura.
	acessoRestrito, err := acesso.Restrito()
	if err != nil {
		return err
	}
	logger.Info(acesso.Frase(acessoRestrito))

	if *dsn == "" {
		return fmt.Errorf("-dsn (or DATABASE_URL) is required")
	}
	// Refused before anything is dialled. Serving the site's API without its key
	// would mean either an open door or one that refuses everyone, and the
	// half-configured state is the one somebody has to notice.
	chaveSite := os.Getenv("W2PP_PAINEL_TOKEN_SITE")
	if *siteAddr != "" && chaveSite == "" {
		return fmt.Errorf("SITE_API_ADDR is set but W2PP_PAINEL_TOKEN_SITE is empty; " +
			"refusing to start. Fill the key first, or empty SITE_API_ADDR to turn the site API off")
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// A CHAVE QUE DESLIGA O CAMINHO ANTIGO. Vazia vale DESLIGADA, ou seja, os dois caminhos
	// convivem — que é o que tem de valer na estreia. Valor que este código não entende é
	// ERRO e o serviço não sobe: ligada por engano tranca todo mundo para fora, e desligada
	// por engano deixa entrar quem já entrava. As duas merecem um erro alto em vez de um
	// padrão adivinhado.
	soUsuarioDoPainel, err := acesso.Ler(os.Getenv("W2PP_PAINEL_SO_USUARIO"))
	if err != nil {
		return fmt.Errorf("W2PP_PAINEL_SO_USUARIO: %w", err)
	}
	logger.Info(fraseDoLoginDoPainel(soUsuarioDoPainel))

	gens, moldeExiste := carregarNPCGener(*contentDir, logger)

	pool, err := store.Pool(ctx, *dsn)
	if err != nil {
		return fmt.Errorf("connect: %w", err)
	}
	defer pool.Close()

	var game panel.GameData
	if *webAddr != "" {
		// Insecure credentials: this link stays on the platform's private
		// network, exactly like tmServer's to dbServer. Give it mTLS the day
		// that link gets it, not before — a lone service with certificates the
		// others lack is a maintenance trap, not a security gain.
		// The key the web-api checks (webserver/internal/authz). Empty is the
		// migration step and nothing more: the web-api lets an unkeyed caller
		// through only while IT has no key configured either, so both sides can
		// be updated before either starts enforcing.
		chave := os.Getenv("W2PP_WEB_TOKEN_PAINEL")
		conn, err := grpc.NewClient(*webAddr,
			grpc.WithTransportCredentials(insecure.NewCredentials()),
			grpc.WithUnaryInterceptor(mandaChaveWeb(chave)))
		if err != nil {
			return fmt.Errorf("webserver dial: %w", err)
		}
		defer func() { _ = conn.Close() }()
		game = gamedata.New(conn)
		logger.Info("webServer wired", "addr", *webAddr, "chave", chave != "")
		if chave == "" {
			logger.Warn("sem W2PP_WEB_TOKEN_PAINEL: as páginas de item e NPC vão " +
				"parar de funcionar assim que o web-api passar a exigir a chave")
		}
	} else {
		logger.Warn("no webServer configured; item pages are hidden",
			"configuration", "W2PP_WEBSERVER")
	}

	// The link to the running game. Off unless an address is given, and refused
	// without a token rather than dialled and failing on every call: the panel
	// would show a Servidor tab that only ever reports a rejection.
	var live panel.Live
	var blocos panel.BlocosDoJogo // same link; a separate field so a nil stays a nil interface
	if *jogoAddr != "" {
		token := os.Getenv("W2PP_CONTROL_TOKEN")
		if token == "" {
			return fmt.Errorf("-tmserver is set but W2PP_CONTROL_TOKEN is empty; " +
				"the game server refuses every call without it")
		}
		// O TOKEN COM CARA DE ENDEREÇO NÃO LIGA O LINK, e o erro diz o que trocar.
		//
		// Foi o defeito de 24/09/2026: a variável do token apontava para a do endereço,
		// o serviço subia anunciando o link ligado, e o tmServer recusava toda chamada.
		//
		// E NOTE A DIFERENÇA para o token VAZIO logo acima, que derruba o boot: aqui o
		// painel SOBE. Não é descuido. O painel sem o link perde as páginas do jogo e
		// continua servindo conta, VIP, bloqueio e auditoria — e uma variável trocada
		// não pode tirar do ar o lugar de onde se conserta a variável trocada.
		if secret.TokenComCaraDeEndereco(token, *jogoAddr) {
			logger.Error("W2PP_CONTROL_TOKEN parece o ENDEREÇO e não o segredo; "+
				"as páginas do jogo ficam desligadas. Aponte a variável para o "+
				"W2PP_CONTROL_TOKEN do serviço tmserver, e não para o endereço",
				"token", secret.Impressao(token), "addr", *jogoAddr)
		} else {
			conn, cerr := grpc.NewClient(*jogoAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
			if cerr != nil {
				return fmt.Errorf("tmserver dial: %w", cerr)
			}
			defer func() { _ = conn.Close() }()
			cliente := jogo.New(conn, token)
			live, blocos = cliente, cliente
			logger.Info("live game link enabled", "addr", *jogoAddr, "token", secret.Impressao(token))
			if secret.TokenFraco(token) {
				logger.Warn("o token de controle e CURTO; troque por um aleatorio de 32 bytes ou mais",
					"minimo", secret.TamanhoMinimoDoToken)
			}
		}
	} else {
		logger.Info("live game pages disabled",
			"configuration", "W2PP_TMSERVER_CONTROL + W2PP_CONTROL_TOKEN")
	}

	// Hosting API, for the game-server status card and its restart button. The
	// project and environment ids are injected into every service by the
	// platform; only the token and the game service's id have to be set by hand.
	var plat panel.Platform
	platCfg := plataforma.Config{
		// Prefer RAILWAY_PROJECT_TOKEN: it reaches this project only, while an
		// account token reaches every project its owner has — and this value
		// lives in the environment of a service published on the internet.
		ProjectToken:  os.Getenv("RAILWAY_PROJECT_TOKEN"),
		Token:         os.Getenv("RAILWAY_API_TOKEN"),
		ProjectID:     os.Getenv("RAILWAY_PROJECT_ID"),
		EnvironmentID: os.Getenv("RAILWAY_ENVIRONMENT_ID"),
		ServiceID:     os.Getenv("W2PP_TMSERVER_SERVICE_ID"),
	}
	if platCfg.Ready() {
		plat = plataforma.New(platCfg)
		logger.Info("hosting API wired", "service", platCfg.ServiceID)
	} else {
		logger.Warn("no hosting API configured; the restart card is hidden",
			"configuration", "RAILWAY_API_TOKEN + W2PP_TMSERVER_SERVICE_ID")
	}

	// Shared with the site API: a staff member who changes their password through
	// the site must lose their panel sessions, as the panel's own reset does.
	sessoes := session.New(*sessionTTL)

	handler, err := panel.New(panel.Config{
		Platform: plat,
		Accounts: store.New(pool),
		// QUEM ADMINISTRA (0130). Sempre montado: a tabela existe desde a migração, e
		// deixar isto opcional só criaria um jeito de subir o painel sem a tela que a
		// Hanna pediu.
		Painel: store.New(pool),
		// O contador das quatro filas, para o distintivo do menu.
		FilasDeDinheiro: store.New(pool),
		// SoUsuarioDoPainel desliga o login por conta de jogo com cargo. A Hanna liga
		// quando tiver criado os usuários dela; ligar antes trancaria para fora a única
		// pessoa que poderia criá-los.
		SoUsuarioDoPainel: soUsuarioDoPainel,
		GameData:          game,
		Writer:            accounts.New(pool),
		Entregas:          entrega.New(pool),
		Personagens:       personagem.New(pool),
		Eventos:           store.New(pool),
		MesaXP:            store.New(pool),
		Masmorras:         store.New(pool),
		Quests:            store.New(pool),
		Spawn:             store.New(pool),
		Receitas:          store.New(pool),
		NPCGener:          gens,
		MoldeExiste:       moldeExiste,
		Combate:           store.New(pool),
		BonusDrop:         store.New(pool),
		Maquinas:          store.New(pool),
		MesaDrops:         store.New(pool),
		Repasses:          store.New(pool),
		FilasRMT:          store.New(pool),
		Passe:             store.New(pool),
		Guildas:           store.New(pool),
		Carteira:          donate.New(pool),
		Trocas:            store.New(pool),
		Censo:             store.New(pool),
		Chat:              store.New(pool),
		Jogo:              live,
		Blocos:            blocos,
		Audit:             audit.New(pool),
		Sessions:          sessoes,
		Logger:            logger,
		SecureOnly:        !*insecureCookies,
		// A MESMA variável do jogo e do site: um servidor trancado para entrar e
		// aberto para cadastrar seria a porta que ninguém lembra de fechar.
		SemCadastro: acessoRestrito,
	})
	if err != nil {
		return fmt.Errorf("build panel: %w", err)
	}

	srv := &http.Server{
		Addr:              *addr,
		Handler:           handler.Routes(),
		ReadHeaderTimeout: readHeaderTimeout,
		ReadTimeout:       readTimeout,
		WriteTimeout:      writeTimeout,
		IdleTimeout:       idleTimeout,
	}

	// The site's API never shares the public listener or its mux: its routes
	// exist only here, and the staff routes do not exist here at all.
	var siteSrv *http.Server
	if *siteAddr != "" {
		// One Store for the three reads the site does through it: credentials,
		// the event switches and the drop ladders. They are the same rows the
		// staff screens read, so a second decoder here could only disagree.
		st := store.New(pool)
		api, err := siteapi.New(siteapi.Config{
			Chave:       chaveSite,
			Contas:      accounts.New(pool),
			Credenciais: st,
			Eventos:     st,
			Taxas:       st,
			Masmorras:   st,
			Leitura:     siteapi.NovoLeitor(pool),
			Carteira:    donate.New(pool),
			Entregas:    entrega.New(pool),
			Jogo:        live,
			Audit:       audit.New(pool),
			Sessoes:     sessoes,
			Logger:      logger,
		})
		if err != nil {
			return fmt.Errorf("build site api: %w", err)
		}
		siteSrv = &http.Server{
			Addr:              *siteAddr,
			Handler:           api.Routes(),
			ReadHeaderTimeout: readHeaderTimeout,
			ReadTimeout:       readTimeout,
			WriteTimeout:      writeTimeout,
			IdleTimeout:       idleTimeout,
		}
	} else {
		logger.Info("site API disabled", "configuration", "SITE_API_ADDR + W2PP_PAINEL_TOKEN_SITE")
	}

	errCh := make(chan error, 2)
	go func() {
		logger.Info("adminserver listening", "addr", *addr, "session_ttl", *sessionTTL,
			"secure_cookies", !*insecureCookies)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- fmt.Errorf("serve: %w", err)
			return
		}
		errCh <- nil
	}()
	if siteSrv != nil {
		go func() {
			logger.Info("site API listening", "addr", *siteAddr, "game_link", live != nil)
			if err := siteSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
				errCh <- fmt.Errorf("site api serve: %w", err)
				return
			}
			errCh <- nil
		}()
	}

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		logger.Info("shutting down")
		shutCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()
		if siteSrv != nil {
			if err := siteSrv.Shutdown(shutCtx); err != nil {
				return fmt.Errorf("site api shutdown: %w", err)
			}
		}
		if err := srv.Shutdown(shutCtx); err != nil {
			return fmt.Errorf("shutdown: %w", err)
		}
		return <-errCh
	}
}

// defaultAddr honours the port the platform assigns. Railway (and most PaaS)
// inject PORT and route to it; binding a hardcoded port there means the health
// probe hits a closed socket and the deploy is marked failed with the process
// running fine.
func defaultAddr() string {
	if p := os.Getenv("PORT"); p != "" {
		return ":" + p
	}
	return ":8080"
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// mandaChaveWeb attaches the panel's key to every call to the web-api.
//
// An empty key sends no header at all, rather than an empty one: the web-api
// refuses a blank key on purpose, and sending one would turn "not configured
// yet" into a confusing authentication failure instead of the pass-through the
// migration step needs.
func mandaChaveWeb(chave string) grpc.UnaryClientInterceptor {
	return func(ctx context.Context, method string, req, reply any,
		cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption,
	) error {
		if chave != "" {
			ctx = metadata.AppendToOutgoingContext(ctx, webv1.TokenHeader, chave)
		}
		return invoker(ctx, method, req, reply, cc, opts...)
	}
}

// fraseDoLoginDoPainel escreve no boot qual caminho de login está valendo.
//
// OS DOIS ESTADOS SÃO ESCRITOS, pelo mesmo motivo das outras trancas deste sistema: um log
// que só fala quando a chave liga faz do silêncio duas coisas diferentes — "está desligada"
// e "esta versão nem tem a chave" —, e é isso que alguém precisa distinguir quando não
// consegue entrar.
func fraseDoLoginDoPainel(soUsuario bool) string {
	if soUsuario {
		return "login do painel: SÓ usuário do painel; conta de jogo com cargo NÃO entra mais"
	}
	return "login do painel: usuário do painel E conta de jogo com cargo (caminho antigo ainda ligado)"
}

// carregarNPCGener reads the file's blocks and builds the template check for the
// recipe pages. A missing tree is not an error: the pages are hidden and every
// other one works, as with the other optional dependencies.
func carregarNPCGener(dir string, logger *slog.Logger) ([]npcgener.Generator, panel.MoldeExiste) {
	if dir == "" {
		return nil, nil
	}
	gens, err := npcgener.Load(filepath.Join(dir, "TMsrv", "run", "NPCGener.txt"))
	if err != nil {
		logger.Warn("NPCGener not loaded; the block recipe pages are hidden", "content", dir, "err", err)
		return nil, nil
	}
	logger.Info("NPCGener loaded for the block recipe pages", "blocks", len(gens))
	return gens, func(name string) (string, bool) {
		res, err := npctemplate.Resolve(dir, name)
		return res.Name, err == nil
	}
}
