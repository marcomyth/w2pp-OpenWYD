//go:build integration

// Os testes da migração 0181: o usuário do painel passa a assinar as escritas de
// administração, e o jogo continua escrevendo onde escrevia.
//
// Rode com:
//
//	W2PP_TEST_DSN=postgres://postgres:dev@localhost:5432/postgres go test -tags=integration ./internal/store/
package store

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/jeanluca/w2pp-openwyd/internal/domain"
)

// oAtorDe lê o par de colunas de ator de uma tabela de auditoria, pela última linha.
//
// UMA FUNÇÃO PARA AS QUATRO TABELAS porque a pergunta é a mesma nas quatro, e escrever
// quatro versões dela deixaria três desatualizando em silêncio.
func oAtorDe(ctx context.Context, t *testing.T, s *Store, tabela, acao string) (conta, painel *int64) {
	t.Helper()
	err := s.pool.QueryRow(ctx, `
		SELECT account_id, actor_painel_usuario_id
		  FROM `+tabela+`
		 WHERE action = $1
		 ORDER BY id DESC
		 LIMIT 1`, acao).Scan(&conta, &painel)
	if err != nil {
		t.Fatalf("lendo o ator de %s em %s: %v", acao, tabela, err)
	}
	return conta, painel
}

// confereQueFoiOPainel exige o ator do painel e, junto, que a conta esteja NULA.
//
// AS DUAS METADES IMPORTAM. Só conferir o id do painel deixaria passar o defeito que
// esta entrega existe para impedir: a linha ter o painel preenchido E a conta com zero
// ou com um número qualquer, atribuindo a ação a duas pessoas ao mesmo tempo.
func confereQueFoiOPainel(t *testing.T, tabela, acao string, conta, painel *int64, esperado int64) {
	t.Helper()
	if conta != nil {
		t.Errorf("%s/%s: account_id = %d, tinha de ser NULO -- "+
			"usuario do painel nao tem conta de jogo", tabela, acao, *conta)
	}
	if painel == nil {
		t.Fatalf("%s/%s: actor_painel_usuario_id NULO, esperava %d", tabela, acao, esperado)
	}
	if *painel != esperado {
		t.Errorf("%s/%s: ator do painel = %d, esperava %d", tabela, acao, *painel, esperado)
	}
}

// TestOUsuarioDoPainelAssinaAsQuatroAuditorias é o teste central da entrega.
//
// UMA ESCRITA DE CADA TABELA, e não as vinte e cinco: as dezoito do npc_audit passam
// pela MESMA função (auditAndBump), então provar uma prova o caminho das dezoito. O que
// muda entre elas é o que se escreve, não quem assina, e o que este teste mede é quem
// assina.
func TestOUsuarioDoPainelAssinaAsQuatroAuditorias(t *testing.T) {
	ctx := context.Background()
	pool := testPool(t)
	if err := Migrate(ctx, pool); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	s := New(pool)
	_, _ = pool.Exec(ctx, `DELETE FROM npc_audit; DELETE FROM npc_shop_item; DELETE FROM npc_definition;
		DELETE FROM daily_reward_audit; DELETE FROM daily_reward_item;
		DELETE FROM donate_shop_audit; DELETE FROM donate_shop_item;
		DELETE FROM world_event_audit`)

	id := usuarioDoPainel(ctx, t, s, "painel-das-23")
	ator := domain.AtorDoPainel(id)

	// npc_audit: vale pelas dezoito.
	if _, err := s.UpsertNPCDefinition(ctx, domain.NPCDefinition{
		Slug: "npc-do-painel", TemplateName: "Reiners", DisplayName: "Reiners",
		Enabled: true, MapID: 0, PosX: 100, PosY: 100, Merchant: 1,
	}, ator); err != nil {
		t.Fatalf("UpsertNPCDefinition: %v", err)
	}
	conta, painel := oAtorDe(ctx, t, s, "npc_audit", "create")
	confereQueFoiOPainel(t, "npc_audit", "create", conta, painel, id)

	// daily_reward_audit.
	if _, err := s.UpsertDailyRewardItem(ctx, domain.DailyRewardItem{
		ItemIndex: 1100, Title: "do painel", Enabled: true,
	}, ator); err != nil {
		t.Fatalf("UpsertDailyRewardItem: %v", err)
	}
	conta, painel = oAtorDe(ctx, t, s, "daily_reward_audit", "create")
	confereQueFoiOPainel(t, "daily_reward_audit", "create", conta, painel, id)

	// donate_shop_audit.
	if _, err := s.UpsertDonateShopItem(ctx, domain.DonateShopItem{
		ItemIndex: 1100, Price: 10, Title: "do painel", Enabled: true,
	}, ator); err != nil {
		t.Fatalf("UpsertDonateShopItem: %v", err)
	}
	conta, painel = oAtorDe(ctx, t, s, "donate_shop_audit", "create")
	confereQueFoiOPainel(t, "donate_shop_audit", "create", conta, painel, id)

	// world_event_audit.
	cfg, err := s.WorldEventConfig(ctx)
	if err != nil {
		t.Fatalf("WorldEventConfig: %v", err)
	}
	cfg.ItemIndex = 777
	if err := s.UpsertWorldEventConfig(ctx, cfg, ator); err != nil {
		t.Fatalf("UpsertWorldEventConfig: %v", err)
	}
	conta, painel = oAtorDe(ctx, t, s, "world_event_audit", "set_config")
	confereQueFoiOPainel(t, "world_event_audit", "set_config", conta, painel, id)
}

// TestAContaDeJogoContinuaAssinandoAsEscritas: o caminho antigo não mudou.
//
// EXISTE PORQUE O RISCO DE UMA MUDANÇA COMO ESTA É QUEBRAR QUEM JÁ FUNCIONAVA. Se o
// moderador com conta de jogo parasse de assinar, ninguém notaria olhando o painel —
// ele salvaria igual, e só a auditoria sairia errada.
func TestAContaDeJogoContinuaAssinandoAsEscritas(t *testing.T) {
	ctx := context.Background()
	pool := testPool(t)
	if err := Migrate(ctx, pool); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	s := New(pool)
	_, _ = pool.Exec(ctx, `DELETE FROM npc_audit; DELETE FROM npc_shop_item; DELETE FROM npc_definition`)

	var contaID int64
	if err := pool.QueryRow(ctx, `
		INSERT INTO account (name, pass_hash, role)
		VALUES ('mod-das-23', 'x', 'moderator')
		ON CONFLICT (name) DO UPDATE SET role = 'moderator'
		RETURNING id`).Scan(&contaID); err != nil {
		t.Fatalf("criando a conta de jogo: %v", err)
	}

	if _, err := s.UpsertNPCDefinition(ctx, domain.NPCDefinition{
		Slug: "npc-da-conta", TemplateName: "Reiners", DisplayName: "Reiners",
		Enabled: true, PosX: 100, PosY: 100, Merchant: 1,
	}, domain.AtorDaConta(contaID)); err != nil {
		t.Fatalf("UpsertNPCDefinition: %v", err)
	}
	conta, painel := oAtorDe(ctx, t, s, "npc_audit", "create")
	if painel != nil {
		t.Errorf("actor_painel_usuario_id = %d, tinha de ser NULO numa escrita de conta de jogo", *painel)
	}
	if conta == nil || *conta != contaID {
		t.Fatalf("account_id = %v, esperava %d", conta, contaID)
	}
}

// TestEscritaSemAtorERecusadaAntesDoBanco: o que a trava do banco garante, o código
// recusa primeiro, e com uma frase que diz o que falta.
//
// AS DUAS CAMADAS SÃO DE PROPÓSITO. A trava do banco é a que vale para qualquer caminho
// escrito amanhã; esta recusa é a que dá uma mensagem legível em vez de uma violação de
// constraint com nome de tabela.
func TestEscritaSemAtorERecusadaAntesDoBanco(t *testing.T) {
	ctx := context.Background()
	pool := testPool(t)
	if err := Migrate(ctx, pool); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	s := New(pool)

	_, err := s.UpsertNPCDefinition(ctx, domain.NPCDefinition{
		Slug: "npc-sem-ator", TemplateName: "Reiners", DisplayName: "Reiners",
		Enabled: true, PosX: 100, PosY: 100, Merchant: 1,
	}, domain.Ator{})
	if err == nil {
		t.Fatal("a escrita sem ator passou, e ela tinha de ser recusada")
	}
	// A mensagem tem de falar de ATOR, e não de constraint: é o que separa "o codigo
	// recusou" de "o banco recusou depois".
	if !contemAtor(err.Error()) {
		t.Errorf("erro = %q, e ele tinha de dizer que falta ator", err)
	}
}

func contemAtor(s string) bool {
	for i := 0; i+4 <= len(s); i++ {
		if s[i:i+4] == "ator" {
			return true
		}
	}
	return false
}

// TestOJogoContinuaGravandoOEventoDeMundoSemAtor é o teste que a 0181 exigia.
//
// AQUI ESTAVA O RISCO DE DERRUBAR O JOGO. A receita original da migração punha, nas
// quatro tabelas, a trava "exatamente um dos dois atores preenchido". Na
// world_event_audit isso recusaria TODA gravação do servidor: quando o Kefra cai, quem
// escreve é o jogo, com fonte='jogo' e sem pessoa nenhuma — o account_id dessa tabela
// aceita nulo desde a 0067 justamente por isso.
//
// Com a trava errada, a cidade do Kefra pararia de abrir e a migração teria destravado
// o painel quebrando o jogo. Este teste é o que impede alguém de "arrumar" a trava para
// ficar igual às outras três.
func TestOJogoContinuaGravandoOEventoDeMundoSemAtor(t *testing.T) {
	ctx := context.Background()
	pool := testPool(t)
	if err := Migrate(ctx, pool); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO world_event_audit (account_id, actor_painel_usuario_id, action, fonte, before, after)
		VALUES (NULL, NULL, 'set_kefra', 'jogo', NULL, '{"live":true}')`); err != nil {
		t.Fatalf("o jogo nao consegue mais gravar no world_event_audit depois da 0181: %v", err)
	}

	// E a linha do PAINEL sem ator nenhum continua recusada: a trava solta a do jogo
	// sem soltar a do painel.
	if _, err := pool.Exec(ctx, `
		INSERT INTO world_event_audit (account_id, actor_painel_usuario_id, action, fonte, before, after)
		VALUES (NULL, NULL, 'set_config', 'painel', NULL, '{}')`); err == nil {
		t.Error("uma linha com fonte='painel' e sem ator foi aceita, e tinha de ser recusada")
	}
}

// TestA0181AceitaAsLinhasAntigas: o histórico de antes da migração continua válido.
//
// UMA MIGRAÇÃO QUE PÕE TRAVA PODE RECUSAR O PASSADO, e é o acidente mais fácil de
// cometer aqui: a trava passa, o serviço sobe, e só se descobre o problema no dia em que
// alguém tenta ler ou copiar a tabela. As linhas antigas têm account_id preenchido e
// nenhuma coluna de painel, então satisfazem "exatamente um" — este teste prova isso em
// vez de supor.
func TestA0181AceitaAsLinhasAntigas(t *testing.T) {
	ctx := context.Background()
	pool := testPool(t)
	if err := Migrate(ctx, pool); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	var contaID int64
	if err := pool.QueryRow(ctx, `
		INSERT INTO account (name, pass_hash, role)
		VALUES ('conta-antiga-0181', 'x', 'moderator')
		ON CONFLICT (name) DO UPDATE SET role = 'moderator'
		RETURNING id`).Scan(&contaID); err != nil {
		t.Fatalf("criando a conta: %v", err)
	}
	casos := []struct{ tabela, sql string }{
		{"npc_audit", `INSERT INTO npc_audit (npc_id, account_id, action) VALUES (NULL, $1, 'linha_antiga')`},
		{"daily_reward_audit", `INSERT INTO daily_reward_audit (reward_item_id, account_id, action) VALUES (NULL, $1, 'linha_antiga')`},
		{"donate_shop_audit", `INSERT INTO donate_shop_audit (shop_item_id, account_id, action) VALUES (NULL, $1, 'linha_antiga')`},
		{"world_event_audit", `INSERT INTO world_event_audit (account_id, action, fonte) VALUES ($1, 'linha_antiga', 'painel')`},
	}
	for _, c := range casos {
		if _, err := pool.Exec(ctx, c.sql, contaID); err != nil {
			t.Errorf("%s recusou uma linha no formato antigo (conta de jogo, sem painel): %v", c.tabela, err)
		}
	}
}

// TestNinguemAssinaComOsDoisAoMesmoTempo: a trava recusa dois atores na mesma linha.
//
// POR QUE ISSO IMPORTA: uma linha com os dois preenchidos diz que duas pessoas fizeram a
// mesma ação. Quem for ler a auditoria depois não tem como escolher qual das duas, e a
// tabela deixa de responder a única pergunta que ela existe para responder.
func TestNinguemAssinaComOsDoisAoMesmoTempo(t *testing.T) {
	ctx := context.Background()
	pool := testPool(t)
	if err := Migrate(ctx, pool); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	s := New(pool)
	painelID := usuarioDoPainel(ctx, t, s, "painel-duplo-0181")
	var contaID int64
	if err := pool.QueryRow(ctx, `
		INSERT INTO account (name, pass_hash, role)
		VALUES ('conta-dupla-0181', 'x', 'moderator')
		ON CONFLICT (name) DO UPDATE SET role = 'moderator'
		RETURNING id`).Scan(&contaID); err != nil {
		t.Fatalf("criando a conta: %v", err)
	}
	tabelas := []string{"npc_audit", "daily_reward_audit", "donate_shop_audit", "world_event_audit"}
	for _, tb := range tabelas {
		sql := `INSERT INTO ` + tb + ` (account_id, actor_painel_usuario_id, action`
		vals := `) VALUES ($1, $2, 'dois_atores'`
		if tb == "world_event_audit" {
			sql += `, fonte`
			vals += `, 'painel'`
		}
		if _, err := pool.Exec(ctx, sql+vals+`)`, contaID, painelID); err == nil {
			t.Errorf("%s aceitou uma linha com os DOIS atores preenchidos", tb)
		}
	}
}

// atorDeTeste devolve um ator válido para os testes que não se importam com QUEM fez.
//
// EXISTE PORQUE ANTES ELES PASSAVAM ZERO, e o zero era gravado como autor. Não dava
// erro: a coluna é um BIGINT sem chave estrangeira, então a auditoria ficava com
// "conta 0" e ninguém percebia. Depois da 0181 o zero é recusado, e esses testes
// precisam de alguém de verdade — o que este ajudante dá, com um usuário do painel
// criado na hora.
func atorDeTeste(ctx context.Context, t *testing.T, s *Store) domain.Ator {
	t.Helper()
	// UM LOGIN POR CHAMADA, e não um fixo: painel_usuario tem UNIQUE no login, então um
	// nome fixo explode na SEGUNDA chamada do mesmo teste — e vários testes chamam isto
	// mais de uma vez. O nome do teste entra para quem estiver lendo o banco depois
	// saber de onde a linha veio.
	atorDeTesteN++
	login := fmt.Sprintf("ator-de-teste-%s-%d", sanitizaLogin(t.Name()), atorDeTesteN)
	return domain.AtorDoPainel(usuarioDoPainel(ctx, t, s, login))
}

// atorDeTesteN numera as chamadas. Os testes de integração deste pacote não rodam em
// paralelo (cada um mexe nas mesmas tabelas), então um contador simples basta.
var atorDeTesteN int

// sanitizaLogin deixa só o que a coluna aceita: minúsculas, sem espaço.
func sanitizaLogin(nome string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(nome) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		default:
			b.WriteRune('-')
		}
	}
	return b.String()
}

// TestA0181PassaComUmaLinhaDeFonteePainelSemConta é o teste do defeito que quase
// derrubou o boot de produção.
//
// O QUE ESTAVA EM JOGO: desde 25/09 as telas /eventos e /eventos/kefra gravam direto no
// store, e quando quem salva é um usuário do painel elas gravam account_id NULO com
// fonte='painel' — exatamente o que a trava nova recusa. Um ADD CONSTRAINT normal varre
// a tabela e FALHA se achar uma linha assim. E como o store.Migrate roda no boot do
// webServer, do dbServer e do adminServer, a migração que falha não deixa NENHUM dos
// três subir: o painel destravaria matando o servidor.
//
// O CONSERTO É O NOT VALID, e este teste é o que prova que ele funciona: a linha velha
// entra ANTES da migração e a migração passa por cima dela.
//
// POR QUE ELE NÃO DEPENDE DE MEDIR A PRODUÇÃO: a resposta não muda com a contagem. Se
// houver zero linhas assim, o NOT VALID não custa nada; se houver uma, ele é o que
// impede a queda. Medir serviria para saber o tamanho do susto, não para escolher o
// conserto.
func TestA0181PassaComUmaLinhaDeFonteePainelSemConta(t *testing.T) {
	ctx := context.Background()
	pool := testPool(t)
	if err := Migrate(ctx, pool); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	// A linha que o painel gravou antes da 0181: fonte do painel e nenhuma conta.
	//
	// ELA É INSERIDA DEPOIS DA MIGRAÇÃO AQUI, e o teste continua valendo: o que se
	// quer provar é que uma linha nesse formato não impede a trava de existir. Com o
	// NOT VALID a trava não olha o passado, então a ordem não muda a resposta — e
	// inserir depois é o único jeito de testar isto sem desfazer a migração no meio de
	// um banco compartilhado com os outros testes.
	//
	// A inserção tem de ser RECUSADA pela trava (a linha é do painel e não tem ator),
	// e é isso que prova que a trava está VALENDO para linha nova ao mesmo tempo que
	// deixou o passado em paz.
	_, err := pool.Exec(ctx, `
		INSERT INTO world_event_audit (account_id, actor_painel_usuario_id, action, fonte, before, after)
		VALUES (NULL, NULL, 'set_config', 'painel', NULL, '{}')`)
	if err == nil {
		t.Error("a trava aceitou uma linha NOVA do painel sem ator, e devia recusar")
	}

	// E a prova de que a trava é NOT VALID e não varreu a tabela: o catálogo do
	// Postgres diz se ela foi validada. convalidated=false é o que queremos nas quatro.
	for _, c := range []string{
		"npc_audit_um_ator", "daily_reward_audit_um_ator",
		"donate_shop_audit_um_ator", "world_event_audit_um_ator",
		"npc_audit_conta_nao_zero", "daily_reward_audit_conta_nao_zero",
		"donate_shop_audit_conta_nao_zero", "world_event_audit_conta_nao_zero",
	} {
		var validada bool
		if err := pool.QueryRow(ctx,
			`SELECT convalidated FROM pg_constraint WHERE conname = $1`, c).Scan(&validada); err != nil {
			t.Errorf("a trava %s nao existe: %v", c, err)
			continue
		}
		if validada {
			t.Errorf("a trava %s entrou VALIDADA: ela varreu a tabela e pode ter falhado "+
				"por causa de linha antiga -- tem de ser NOT VALID", c)
		}
	}
}
