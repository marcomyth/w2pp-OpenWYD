// Command webserver is the web-api: the gRPC edge the Next.js BFF calls
// server-side (web-platform-plan.md). It owns the web platform's account flows
// (sign-up, credential check) over the same `account` table and argon2id hashing
// as dbServer, but is a SEPARATE service from dbServer's legacy AccountService.
//
// Usage:
//
//	webserver [-addr :7600] -dsn <postgres-url> [-tls-cert … -tls-key … -tls-ca …] [-content <Release/>]
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	webv1 "github.com/jeanluca/w2pp-openwyd/api/web/v1"
	"github.com/jeanluca/w2pp-openwyd/internal/secure"
	"github.com/jeanluca/w2pp-openwyd/internal/store"
	"github.com/jeanluca/w2pp-openwyd/webserver/internal/account"
	"github.com/jeanluca/w2pp-openwyd/webserver/internal/attributemap"
	"github.com/jeanluca/w2pp-openwyd/webserver/internal/authz"
	"github.com/jeanluca/w2pp-openwyd/webserver/internal/characters"
	"github.com/jeanluca/w2pp-openwyd/webserver/internal/dailyreward"
	"github.com/jeanluca/w2pp-openwyd/webserver/internal/doacaovarredura"
	"github.com/jeanluca/w2pp-openwyd/webserver/internal/donaterevenue"
	"github.com/jeanluca/w2pp-openwyd/webserver/internal/donateshop"
	"github.com/jeanluca/w2pp-openwyd/webserver/internal/donatetopup"
	"github.com/jeanluca/w2pp-openwyd/webserver/internal/droptool"
	"github.com/jeanluca/w2pp-openwyd/webserver/internal/grpcsrv"
	"github.com/jeanluca/w2pp-openwyd/webserver/internal/itemcatalog"
	"github.com/jeanluca/w2pp-openwyd/webserver/internal/itemicons"
	"github.com/jeanluca/w2pp-openwyd/webserver/internal/itemstatadmin"
	"github.com/jeanluca/w2pp-openwyd/webserver/internal/jogo"
	"github.com/jeanluca/w2pp-openwyd/webserver/internal/mobspawns"
	"github.com/jeanluca/w2pp-openwyd/webserver/internal/mobtemplateadmin"
	"github.com/jeanluca/w2pp-openwyd/webserver/internal/mobtemplates"
	"github.com/jeanluca/w2pp-openwyd/webserver/internal/mountgrowth"
	"github.com/jeanluca/w2pp-openwyd/webserver/internal/npcadmin"
	"github.com/jeanluca/w2pp-openwyd/webserver/internal/npctemplates"
	"github.com/jeanluca/w2pp-openwyd/webserver/internal/ponte"
	"github.com/jeanluca/w2pp-openwyd/webserver/internal/ranking"
	"github.com/jeanluca/w2pp-openwyd/webserver/internal/rmtpagamento"
	"github.com/jeanluca/w2pp-openwyd/webserver/internal/rmtrepasse"
	"github.com/jeanluca/w2pp-openwyd/webserver/internal/rmtvarredura"
	"github.com/jeanluca/w2pp-openwyd/webserver/internal/worldevent"
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	if err := run(logger); err != nil {
		logger.Error("webserver failed", "err", err)
		os.Exit(1)
	}
}

// run applies migrations and serves the gRPC AccountWebService until the process
// receives SIGINT/SIGTERM, then stops gracefully. It shares store.Migrate with
// dbServer, so booting either service brings the schema up to date.
func run(logger *slog.Logger) error {
	addr := flag.String("addr", ":7600", "gRPC listen address")
	dsn := flag.String("dsn", envOr("W2PP_DB_DSN", ""), "PostgreSQL DSN (or W2PP_DB_DSN)")
	tlsCert := flag.String("tls-cert", os.Getenv("W2PP_TLS_CERT"), "server certificate (PEM)")
	tlsKey := flag.String("tls-key", os.Getenv("W2PP_TLS_KEY"), "server private key (PEM)")
	tlsCA := flag.String("tls-ca", os.Getenv("W2PP_TLS_CA"), "client CA (PEM) for mTLS")
	contentDir := flag.String("content", os.Getenv("W2PP_CONTENT"), "path to the Release/ content tree (empty = skip; ListMerchantTemplates/ListItemCatalog return empty lists and the UI falls back to manual entry)")
	iconManifestPath := flag.String("item-icons-manifest", os.Getenv("W2PP_ITEM_ICONS_MANIFEST"), "generated item-icon manifest (empty = fallback-only)")
	flag.Parse()

	if *dsn == "" {
		return fmt.Errorf("-dsn (or W2PP_DB_DSN) is required")
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	pool, err := store.Pool(ctx, *dsn)
	if err != nil {
		return fmt.Errorf("connect: %w", err)
	}
	defer pool.Close()
	if err := store.Migrate(ctx, pool); err != nil {
		return err
	}

	creds, err := secure.ServerCreds(secure.Config{CertFile: *tlsCert, KeyFile: *tlsKey, CAFile: *tlsCA})
	if err != nil {
		return err
	}

	// Who may call what (webserver/internal/authz). Until this existed the only
	// gate was mutual TLS, which answers "is the caller one of our services" and
	// not "which one, and may it do this" — and it degrades to no gate at all
	// when no certificate is configured. Most of the services below administer
	// the game, so that difference stops being free the moment a player-facing
	// site becomes a second caller.
	chaves := authz.Chaves{
		Painel: os.Getenv("W2PP_WEB_TOKEN_PAINEL"),
		Site:   os.Getenv("W2PP_WEB_TOKEN_SITE"),
		// A chave de SISTEMA abre só o que nenhuma pessoa dispara: hoje, o aviso
		// de pagamento que o site repassa. Separada da do site porque o caller é
		// diferente em espécie — ver authz.servicosDeSistema.
		Sistema: os.Getenv("W2PP_WEB_TOKEN_SISTEMA"),
	}
	// A PONTE, que é quem fala com a processadora.
	//
	// Sem as quatro variáveis o cliente fica DESLIGADO e o boot avisa, sem derrubar
	// o servidor. É o mesmo desenho do saldo e da cobrança no jogo, e pelo mesmo
	// motivo: um caminho recusando em voz alta é melhor do que um servidor que não
	// sobe — e a maior parte do que este processo serve não tem nada a ver com
	// dinheiro real.
	cfgPonte := ponte.Config{
		URL:      os.Getenv("PONTE_URL"),
		Segredo:  os.Getenv("PONTE_SEGREDO"),
		CertPEM:  os.Getenv("PONTE_CERT_CLIENTE"),
		ChavePEM: os.Getenv("PONTE_CHAVE_CLIENTE"),
	}
	var clientePonte *ponte.Cliente
	if cliente, err := ponte.Novo(cfgPonte); err != nil {
		// O erro diz QUAIS variáveis faltam, por nome e nunca por valor. É o que
		// transforma um boot pela metade numa tarefa de dois minutos.
		logger.Warn("ponte desligada: a venda por dinheiro real não vai cobrar nem pagar",
			"motivo", err)
	} else {
		clientePonte = cliente
		logger.Info("ponte ligada", "url", cfgPonte.URL)
		// UMA CHAMADA ASSINADA NO BOOT, e ela existe porque o segredo e o
		// certificado são SELADOS na Railway: ninguém consegue relê-los para
		// conferir se o bloco foi colado inteiro. A única prova é uma chamada que
		// funcione.
		//
		// Ver ponte.Sonda para por que ela NÃO usa o /saude: aquele é GET e não é
		// assinado, então passaria com o segredo errado e provaria só metade.
		//
		// Em segundo plano e sem derrubar nada: o resto do web-api não tem nada a
		// ver com dinheiro real, e uma ponte fora do ar não pode impedir o
		// cadastro de conta de subir.
		//
		// O aviso separa as duas falhas porque os consertos são diferentes: erro
		// de TLS é certificado ou chave; 401 é segredo ou relógio.
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			if err := cliente.Sonda(ctx); err != nil {
				logger.Warn("ponte reprovou no teste do boot. erro de TLS = "+
					"PONTE_CERT_CLIENTE/PONTE_CHAVE_CLIENTE; http 401 = "+
					"PONTE_SEGREDO (ou relógio fora da janela de 5 min); "+
					"http 404 = PONTE_URL", "err", err)
				return
			}
			logger.Info("ponte respondeu ao teste do boot: mTLS e assinatura conferem")
		}()
	}
	_ = clientePonte // usado mais abaixo, junto com o registro dos serviços

	if !chaves.Configurada() {
		// Loud, and every boot, because the quiet version of this line is how
		// the hole survived: the server starts, looks healthy, and serves
		// DeleteNpc to whoever reaches the port. It still starts, because this
		// build has to be deployable to a running server BEFORE the keys exist
		// on either side; the build that refuses comes after.
		logger.Warn("web-api sem chave: quem alcançar esta porta pode criar e apagar " +
			"NPC, mudar preço e atributo de item e mexer em saldo de donate. " +
			"Configure W2PP_WEB_TOKEN_PAINEL, e a mesma no painel.")
	}
	srv := grpc.NewServer(
		grpc.Creds(creds),
		grpc.UnaryInterceptor(authz.Interceptor(chaves)),
		grpc.StreamInterceptor(authz.StreamInterceptor(chaves)),
	)
	st := store.New(pool)
	npcAdmin := npcadmin.New(st)
	npcAdmin.SetLogger(logger)
	mobTemplateAdmin := mobtemplateadmin.New(st)
	itemStatAdmin := itemstatadmin.New(st)
	mountGrowthAdmin := mountgrowth.New(st)
	donate := donateshop.New(st)
	dailyRwd := dailyreward.New(st)
	topup := donatetopup.New(st).ComLog(logger)
	revenue := donaterevenue.New(st)
	attrMap := attributemap.New(st, *contentDir)
	worldEvents := worldevent.New(st)
	// Stays zero-valued without -content: ItemCatalogService then serves an
	// empty list, the same graceful degradation the pickers already have.
	var itemCatalog itemcatalog.Catalog
	var iconManifest itemicons.Manifest
	if *iconManifestPath != "" {
		iconManifest, err = itemicons.Load(*iconManifestPath)
		if err != nil {
			return err
		}
		logger.Info("loaded item icon manifest", "version", iconManifest.PackVersion,
			"mapped", iconManifest.MappedItems, "icons", iconManifest.DistinctIcons)
	} else {
		logger.Warn("item icon manifest not configured; catalog is fallback-only",
			"configuration", "W2PP_ITEM_ICONS_MANIFEST")
	}
	// Where each template spawns, shared by the mob editor and the drop report.
	var spawnOrigins mobspawns.Index
	if *contentDir != "" {
		templates, npcStats, err := npctemplates.Scan(*contentDir, logger)
		if err != nil {
			logger.Warn("npc template scan failed; merchant picker will be empty", "content", *contentDir, "err", err)
		} else {
			logger.Info("scanned merchant npc templates", "count", len(templates), "layouts", npcStats)
			npcAdmin.SetTemplates(templates)
		}

		catalog, err := itemcatalog.Scan(*contentDir)
		if err != nil {
			logger.Warn("item catalog scan failed; item picker will be empty", "content", *contentDir, "err", err)
		} else {
			if *iconManifestPath != "" {
				itemcatalog.ApplyIcons(&catalog, iconManifest)
			}
			withIconKey, withIconURL := catalog.IconCoverage()
			logger.Info("scanned item catalog", "count", len(catalog.Items), "version", catalog.Version,
				"icon_pack_version", catalog.IconPackVersion, "items_with_icon_key", withIconKey,
				"items_with_icon_url", withIconURL)
			npcAdmin.SetItemCatalog(catalog)
			itemCatalog = catalog

			// Index by item index so the stat editor can seed a new override
			// from the catalog in one lookup. Built here rather than inside the
			// service because the catalog is immutable after boot: the content
			// tree is mounted read-only.
			porIndice := make(map[int32]itemcatalog.Entry, len(catalog.Items))
			for _, e := range catalog.Items {
				porIndice[e.Index] = e
			}
			itemStatAdmin.SetCatalog(func(idx int32) (itemcatalog.Entry, bool) {
				e, ok := porIndice[idx]
				return e, ok
			})
			// The mount screen names its lineages from the same catalog.
			mountGrowthAdmin.SetCatalog(func(idx int32) (itemcatalog.Entry, bool) {
				e, ok := porIndice[idx]
				return e, ok
			})
		}

		mobTemplates, mobStats, err := mobtemplates.Scan(*contentDir, logger)
		if err != nil {
			logger.Warn("mob template scan failed; template picker will be empty", "content", *contentDir, "err", err)
		} else {
			logger.Info("scanned mob templates", "count", len(mobTemplates), "layouts", mobStats)
			mobTemplateAdmin.SetTemplates(mobTemplates)
		}
		mobTemplateAdmin.SetTemplateReader(func(name string) ([]byte, error) {
			return os.ReadFile(filepath.Join(*contentDir, "TMsrv", "run", "npc", name))
		})

		// Where each template spawns. Only 620 of the ~1991 template files are
		// named by a generator, and the editor cannot otherwise tell the Água
		// gargoyle from the one that spawns nowhere.
		if origins, err := mobspawns.Build(*contentDir, logger); err != nil {
			logger.Warn("mob spawn index failed; the editor will not say where a mob comes from",
				"content", *contentDir, "err", err)
		} else {
			logger.Info("indexed mob spawn origins", "templates", len(origins))
			mobTemplateAdmin.SetOrigins(origins)
			spawnOrigins = origins
		}

		exclusions, err := droptool.LoadContentExclusions(*contentDir)
		if err != nil {
			logger.Warn("drop exclusion scan failed; continuing without exclusions", "content", *contentDir, "err", err)
		}
		drops, dropStats, err := droptool.Scan(*contentDir, logger, droptool.Options{Exclusions: exclusions})
		if err != nil {
			logger.Warn("drop catalog scan failed; DropTool endpoints will be empty", "content", *contentDir, "err", err)
		} else {
			logger.Info("scanned drop catalog", "items", len(drops.Items), "mobs", len(drops.Mobs),
				"drops", len(drops.Drops), "layouts", dropStats)
			npcAdmin.SetDropCatalog(drops)
		}
	}
	// O LINK COM O JOGO É CORTESIA, e por isso ele não impede nada de subir.
	//
	// As duas chamadas que o caminho do pagamento faz nele — entregar agora, liberar
	// a venda agora — só ENCURTAM a espera: quando elas rodam, a venda já está
	// gravada, e o login de cada um faz o mesmo trabalho. Sem o link, a venda
	// acontece igual e demora mais a aparecer.
	//
	// Mesma autenticação e mesma configuração do painel da staff, de propósito:
	// mesmas duas variáveis, mesmo cabeçalho de token do proto. Um canal novo aqui
	// seria uma segunda porta para o servidor de jogo, com a metade da atenção.
	var jogoDoPagamento rmtpagamento.Jogo
	if addr := os.Getenv("W2PP_TMSERVER_CONTROL"); addr != "" {
		token := os.Getenv("W2PP_CONTROL_TOKEN")
		switch token {
		case "":
			// Avisa e NÃO liga. Ligar sem token daria um cliente que o servidor de
			// jogo recusa em toda chamada, e cada recusa sairia como falha de
			// entrega — ruído que esconde a causa, que é uma variável vazia.
			logger.Warn("W2PP_TMSERVER_CONTROL está setado e W2PP_CONTROL_TOKEN está vazio: " +
				"a entrega imediata fica desligada; a venda sai no login")
		default:
			conn, cerr := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
			if cerr != nil {
				logger.Warn("não consegui abrir o link com o servidor de jogo; "+
					"a entrega imediata fica desligada", "addr", addr, "err", cerr)
			} else {
				defer func() { _ = conn.Close() }()
				jogoDoPagamento = jogo.New(conn, token)
				logger.Info("link com o servidor de jogo ligado para a entrega imediata", "addr", addr)
			}
		}
	} else {
		logger.Info("entrega imediata desligada; a venda sai no login de cada um",
			"configuração", "W2PP_TMSERVER_CONTROL + W2PP_CONTROL_TOKEN")
	}

	// O serviço do aviso de pagamento e a criação tardia do Pix, os dois presos à
	// ponte: sem ela não há como consultar nem criar, e ligar qualquer um dos dois
	// sem ela daria um caminho que falha em toda chamada.
	rmtSrv := grpcsrv.NewRmt(st)
	if clientePonte != nil {
		adaptador := rmtpagamento.PonteDeVerdade{Cliente: clientePonte}
		pagamentos := rmtpagamento.Novo(adaptador, st, jogoDoPagamento, logger)
		webv1.RegisterRmtSystemServiceServer(srv, grpcsrv.NewRmtSistema(pagamentos, logger))
		rmtSrv = rmtSrv.ComCriadorDePix(adaptador.CriarPix, nomeDeItem(itemCatalog), logger)

		// O PAGAMENTO AO VENDEDOR, numa varredura de fundo.
		//
		// Varredura e não gatilho na confirmação da venda, de propósito: o repasse pode
		// falhar por coisas que não têm nada a ver com a venda — a chave do vendedor
		// ainda não cadastrada, o teto diário da ponte, a trava do saque desligada — e
		// amarrar o pagamento ao instante da compra faria a venda carregar o risco de
		// todas elas.
		//
		// O intervalo é longo porque a pressa aqui não vale nada: o vendedor espera
		// minutos e ninguém fica bloqueado. O que importa é que a fila ANDE, e ande
		// sozinha, e não que ande rápido.
		repasses := rmtrepasse.Novo(clientePonte, st, logger)
		go varrerRepasses(ctx, repasses, logger)
		logger.Info("repasse ao vendedor ligado", "intervalo", intervaloDoRepasse)

		// NADA VENCE SEM PERGUNTAR, e o que entrou fora do prazo volta.
		//
		// Esta varredura mora aqui e não no dbserver porque as duas coisas que ela faz
		// falam com a ponte, e as credenciais são deste processo. Enquanto ela não
		// existia, a varredura do dbserver vencia a cobrança sem consultar ninguém — e
		// um Pix pago no último segundo, com o aviso atrasado, virava pagamento sem
		// item.
		varredura := rmtvarredura.Nova(rmtvarredura.DoPagamento{Servico: pagamentos},
			clientePonte, st, logger)
		go varrerCobrancas(ctx, varredura)
		logger.Info("conferencia antes de vencer ligada", "intervalo", intervaloDaConferencia)

		// A REDE EMBAIXO DO AVISO DA DOAÇÃO.
		//
		// A doação tinha um caminho só para virar crédito: a processadora avisa o
		// site, o site chama o ConfirmTopupOrder. Um aviso perdido era dinheiro
		// cobrado e Rcoin nunca dado, sem nada aqui capaz de perceber — o servidor
		// nunca tinha visto o id da processadora.
		//
		// Agora o site entrega o id (AttachTopupCharge) e esta varredura pergunta.
		doacoes := doacaovarredura.Nova(doacaovarredura.DaPonte{Cliente: clientePonte},
			doacaovarredura.DoServico{Servico: topup}, st, logger)
		go varrerDoacoes(ctx, doacoes)
		logger.Info("conferencia da doacao ligada",
			"intervalo_novos", intervaloDaConferencia, "intervalo_resto", intervaloDasMortas,
			"janela_de_novo", janelaDoPedidoNovo, "janela", store.JanelaDaCobrancaMorta)
		logger.Info("caminho do pagamento em dinheiro real ligado")
	} else {
		logger.Warn("caminho do pagamento em dinheiro real DESLIGADO: sem a ponte, " +
			"a página da cobrança não gera código e nenhum aviso é processado; " +
			"as cobranças com código NÃO vencem sozinhas e os reembolsos não são pedidos")
	}

	webv1.RegisterAccountWebServiceServer(srv, grpcsrv.New(account.New(st)))
	webv1.RegisterRankingWebServiceServer(srv, grpcsrv.NewRanking(ranking.New(st)))
	webv1.RegisterRmtWebServiceServer(srv, rmtSrv)
	webv1.RegisterCharacterWebServiceServer(srv, grpcsrv.NewCharacters(characters.New(st)))
	webv1.RegisterItemCatalogServiceServer(srv, grpcsrv.NewItemCatalog(itemCatalog))
	npcAdminSrv := grpcsrv.NewNpcAdmin(npcAdmin)
	if spawnOrigins != nil {
		npcAdminSrv.SetOrigins(spawnOrigins) // the drop report's "onde nasce"
	}
	webv1.RegisterNpcAdminServiceServer(srv, npcAdminSrv)
	webv1.RegisterMobTemplateAdminServiceServer(srv, grpcsrv.NewMobTemplateAdmin(mobTemplateAdmin))
	webv1.RegisterItemStatAdminServiceServer(srv, grpcsrv.NewItemStatAdmin(itemStatAdmin))
	webv1.RegisterMountGrowthAdminServiceServer(srv, grpcsrv.NewMountGrowthAdmin(mountGrowthAdmin))
	webv1.RegisterAttributeMapAdminServiceServer(srv, grpcsrv.NewAttributeMapAdmin(attrMap))
	webv1.RegisterDonateAdminServiceServer(srv, grpcsrv.NewDonateAdmin(donate))
	webv1.RegisterDonateShopServiceServer(srv, grpcsrv.NewDonateShop(donate))
	webv1.RegisterDailyRewardAdminServiceServer(srv, grpcsrv.NewDailyRewardAdmin(dailyRwd))
	webv1.RegisterDailyRewardServiceServer(srv, grpcsrv.NewDailyReward(dailyRwd))
	webv1.RegisterDonateTopupServiceServer(srv, grpcsrv.NewDonateTopup(topup).ComLog(logger))
	webv1.RegisterDonateRevenueAdminServiceServer(srv, grpcsrv.NewDonateRevenue(revenue))
	webv1.RegisterWorldEventAdminServiceServer(srv, grpcsrv.NewWorldEventAdmin(worldEvents))

	ln, err := net.Listen("tcp", *addr)
	if err != nil {
		return fmt.Errorf("listen %s: %w", *addr, err)
	}
	logger.Info("webserver serving", "addr", *addr, "mtls", *tlsCert != "",
		"chave_painel", chaves.Painel != "", "chave_site", chaves.Site != "")

	errCh := make(chan error, 1)
	go func() { errCh <- srv.Serve(ln) }()

	select {
	case <-ctx.Done():
		logger.Info("shutting down")
		srv.GracefulStop()
		return nil
	case err := <-errCh:
		if err != nil && !errors.Is(err, grpc.ErrServerStopped) {
			return fmt.Errorf("serve: %w", err)
		}
		return nil
	}
}

// envOr returns the environment value for key, or def when unset.
func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// nomeDeItem monta a busca de nome por índice a partir do catálogo.
//
// UM MAPA E NÃO UMA VARREDURA: a lista tem milhares de entradas e esta função é
// chamada na criação de cada cobrança. Varrer seria barato hoje e é o tipo de coisa
// que ninguém vai reler depois.
//
// Devolve vazio para índice que o catálogo não conhece, e quem chama trata isso
// caindo na descrição genérica: perder o nome do item não pode derrubar uma venda.
func nomeDeItem(c itemcatalog.Catalog) grpcsrv.NomeDeItem {
	if len(c.Items) == 0 {
		return nil
	}
	porIndice := make(map[int32]string, len(c.Items))
	for _, e := range c.Items {
		porIndice[e.Index] = e.DisplayName
	}
	return func(i int32) string { return porIndice[i] }
}

// intervaloDoRepasse é de quanto em quanto tempo a fila de pagamento anda.
//
// LONGO DE PROPÓSITO. A pressa aqui não vale nada: o vendedor espera minutos, ninguém
// fica bloqueado, e cada rodada move dinheiro de verdade. O que importa é que a fila
// ande sozinha, e não que ande rápido — uma varredura curta multiplicaria as chances de
// duas rodadas se cruzarem sem comprar nada em troca.
const intervaloDoRepasse = 2 * time.Minute

// intervaloDaConferencia é de quanto em quanto tempo as cobranças abertas são
// conferidas na processadora.
//
// CURTO, ao contrário do repasse, e por dois motivos que puxam para o mesmo lado. O
// atraso desta varredura entra inteiro no tempo que o item do vendedor fica preso além
// do prazo, porque agora é ela que vence as cobranças com código. E ela é também o
// polling: o comprador que pagou e cujo aviso não chegou espera exatamente um
// intervalo destes para receber.
//
// Vinte segundos contra uma janela de cinco minutos é um vigésimo quinto da janela, e
// o custo é uma consulta por cobrança aberta — que são poucas por construção, uma por
// anúncio.
const intervaloDaConferencia = 20 * time.Second

// intervaloDasMortas é de quanto em quanto tempo as cobranças JÁ VENCIDAS são
// conferidas.
//
// RARO, porque aqui ninguém está esperando na frente de uma tela: o dinheiro que
// entra numa cobrança morta vai ser devolvido de qualquer jeito, e cinco minutos a
// mais no caminho não mudam nada para ninguém. O que não pode é NUNCA perguntar — aí
// o pagamento não vira linha nenhuma.
const intervaloDasMortas = 5 * time.Minute

// varrerCobrancas confere as abertas e pede as devoluções devidas, até o servidor
// parar.
//
// Numa goroutine só, pelo mesmo motivo do repasse: é a trava mais simples contra duas
// rodadas se cruzarem. As duas passadas são sequenciais de propósito — a conferência é
// que produz os reembolsos pendentes, então pedir logo depois de conferir faz a
// devolução sair na mesma rodada em que a dívida nasceu.
func varrerCobrancas(ctx context.Context, v *rmtvarredura.Varredura) {
	t := time.NewTicker(intervaloDaConferencia)
	defer t.Stop()
	// As mortas num relógio próprio, e as duas no MESMO select: uma goroutine só
	// continua sendo a trava mais simples contra duas rodadas se cruzarem.
	mortas := time.NewTicker(intervaloDasMortas)
	defer mortas.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			// Prazo por rodada: uma ponte lenta não pode segurar a varredura para
			// sempre, e a rodada seguinte pega o que sobrou.
			prazo, cancela := context.WithTimeout(ctx, intervaloDaConferencia)
			// Em passos e não aninhado: a ordem importa — a conferência é que produz
			// os reembolsos pendentes que o Devolver pede — e ordem que depende da
			// avaliação dos argumentos de uma chamada é ordem que ninguém lê.
			conferencia := v.Conferir(prazo)
			reembolsos := v.Devolver(prazo)
			v.Registrar(prazo, conferencia, reembolsos)
			cancela()
		case <-mortas.C:
			// O pagamento que chega depois do prazo: o código Pix continua pagável, e
			// sem esta passada um aviso perdido faria o dinheiro entrar sem virar
			// linha nenhuma. Devolver logo em seguida, pelo mesmo motivo de cima — é a
			// conferência que produz a devolução a pedir.
			prazo, cancela := context.WithTimeout(ctx, intervaloDasMortas)
			conferencia := v.ConferirMortas(prazo)
			reembolsos := v.Devolver(prazo)
			v.Registrar(prazo, conferencia, reembolsos)
			cancela()
		}
	}
}

// janelaDoPedidoNovo é até quando um pedido de doação conta como "recém-feito".
//
// DEZ MINUTOS, e o número sai do comportamento de quem paga: quem vai pagar um Pix
// paga nos primeiros minutos, com a tela aberta. Nessa faixa a varredura roda junto
// com a das cobranças, a cada 20 segundos, porque é a faixa em que alguém está
// esperando o crédito aparecer.
//
// Passados os dez minutos o pedido não some da varredura: ele cai na passada rara,
// que o alcança por 48 horas. O que muda é só a pressa.
const janelaDoPedidoNovo = 10 * time.Minute

// varrerDoacoes confere os pedidos de doação pendentes, em duas cadências.
//
// AS DUAS NO MESMO SELECT e numa goroutine só, como as outras: é a trava mais
// simples contra duas rodadas se cruzarem.
//
// Por que duas: a rápida serve quem está com a tela aberta; a rara existe porque o
// código Pix continua pagável depois, e um carrinho abandonado não pode custar uma
// consulta a cada 20 segundos por dois dias.
func varrerDoacoes(ctx context.Context, v *doacaovarredura.Varredura) {
	novos := time.NewTicker(intervaloDaConferencia)
	defer novos.Stop()
	resto := time.NewTicker(intervaloDasMortas)
	defer resto.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-novos.C:
			prazo, cancela := context.WithTimeout(ctx, intervaloDaConferencia)
			v.Registrar(prazo, v.Conferir(prazo, janelaDoPedidoNovo))
			cancela()
		case <-resto.C:
			prazo, cancela := context.WithTimeout(ctx, intervaloDasMortas)
			v.Registrar(prazo, v.Conferir(prazo, store.JanelaDaCobrancaMorta))
			cancela()
		}
	}
}

// varrerRepasses paga a fila de tempos em tempos, até o servidor parar.
//
// Roda numa goroutine só, e essa é a trava mais simples que existe contra duas rodadas
// se cruzarem. A do banco continua valendo — o estado PENDENTE conferido com a linha
// travada —, e ela é a que vale se um dia houver duas réplicas do webserver. Esta aqui
// é a que dispensa pensar no caso comum.
func varrerRepasses(ctx context.Context, s *rmtrepasse.Servico, log *slog.Logger) {
	t := time.NewTicker(intervaloDoRepasse)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			// Prazo por rodada: uma ponte lenta não pode segurar a varredura para
			// sempre, e a rodada seguinte pega o que sobrou.
			prazo, cancela := context.WithTimeout(ctx, intervaloDoRepasse)
			s.PagarPendentes(prazo, 20).Registrar(log)
			cancela()
		}
	}
}
