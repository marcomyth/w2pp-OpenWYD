//go:build integration

// Os pacotes de doação: a conferência, os brindes indo para a caixa postal na MESMA
// transação do crédito, e a conta de espaços que a tela do site promete.
//
//	W2PP_TEST_DSN=postgres://postgres:dev@localhost:5432/postgres go test -tags=integration ./internal/store/
package store

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/jeanluca/w2pp-openwyd/internal/domain"
)

// contaSimples cria uma conta com o cargo pedido.
func contaComCargo(ctx context.Context, t *testing.T, s *Store, nome, cargo string) int64 {
	t.Helper()
	var id int64
	if err := s.pool.QueryRow(ctx, `
		INSERT INTO account (name, pass_hash, role) VALUES ($1, 'x', $2) RETURNING id`,
		nome, cargo).Scan(&id); err != nil {
		t.Fatalf("criando a conta %q: %v", nome, err)
	}
	return id
}

// OS NOVE PACOTES E O DE TESTE EXISTEM, com os números que foram LIDOS do site.
//
// Este teste é a única coisa que liga a tabela do servidor ao que a página vende. Se
// alguém mexer num preço aqui sem mexer lá, os dois passam a discordar — e a
// conferência do pedido recusaria toda compra daquele pacote, em produção, sem
// ninguém ter mudado o site.
func TestOsPacotesDoSiteEstaoNaTabela(t *testing.T) {
	s, ctx := freshStore(t)

	// Lidos de marcomyth/w2pp-site, src/config/pacotes.ts:158-195. Os Rcoins são o
	// TOTAL (o 5º argumento), porque o bônus já está dentro dele.
	querido := []struct {
		id       string
		credits  int32
		centavos int64
		soStaff  bool
		brindes  int
	}{
		{"apoiador-iniciante", 300, 2990, false, 4},
		{"apoiador-bronze", 575, 4990, false, 4},
		{"apoiador-prata", 1250, 9990, false, 4},
		{"apoiador-ouro", 2100, 14990, false, 4},
		{"apoiador-platina", 3000, 19990, false, 4},
		{"apoiador-diamante", 4950, 29990, false, 4},
		{"apoiador-mestre", 7000, 39990, false, 5},
		{"apoiador-lenda", 10000, 49990, false, 6},
		{"apoiador-supremo", 20000, 79990, false, 6},
		// Sem brinde, e é a ausência que o define: um brinde de teste entregaria item
		// de verdade num teste de pagamento.
		{"teste-real", 10, 100, true, 0},
	}
	for _, q := range querido {
		p, err := s.LerPacote(ctx, q.id)
		if err != nil {
			t.Errorf("%s: %v", q.id, err)
			continue
		}
		if p.Credits != q.credits || p.AmountCents != q.centavos {
			t.Errorf("%s: %d Rcoins por %d centavos, quero %d por %d",
				q.id, p.Credits, p.AmountCents, q.credits, q.centavos)
		}
		if p.SoStaff != q.soStaff {
			t.Errorf("%s: so_staff = %v, quero %v", q.id, p.SoStaff, q.soStaff)
		}
		if !p.Ativo {
			t.Errorf("%s: nasceu desligado", q.id)
		}
		if len(p.Itens) != q.brindes {
			t.Errorf("%s: %d brindes, quero %d", q.id, len(p.Itens), q.brindes)
		}
	}
}

// A CONTA DE ESPAÇOS, que é o número que a tela do site promete.
//
// Ele tem de vir DAQUI porque a regra de empilhar é do servidor: o site não tem como
// saber que 64 Baús do Apoiador ocupam UM espaço e 64 montarias ocupariam 64.
//
// E o número é calculado pela MESMA função que a entrega usa (pilha.Divide), então a
// promessa da página e o que chega no baú não podem divergir.
func TestEspacosNoBauPorPacote(t *testing.T) {
	s, ctx := freshStore(t)

	// COM OS BAÚS DE SORTEIO EMPILHANDO, cada brinde ocupa UM espaço: os empilháveis
	// cabem numa pilha só (o maior pede 64 e o teto é 120) e os que não empilham vêm em
	// unidade. Então a conta é uma linha por brinde.
	//
	// ESTA TABELA É A MUDANÇA QUE O PR FAZ. Antes dele eram 5, 7, 11, 15, 19, 27, 36,
	// 45 e 69 — porque cada baú ocupava o seu espaço, e o Supremo pedia 69 dos 128 do
	// baú da conta. O teste é o que obriga essa conta a mudar de forma explícita, e é
	// de onde a tela do site tira o número que promete.
	querido := map[string]int{
		"apoiador-iniciante": 4, "apoiador-bronze": 4, "apoiador-prata": 4,
		"apoiador-ouro": 4, "apoiador-platina": 4, "apoiador-diamante": 4,
		"apoiador-mestre": 5, "apoiador-lenda": 6, "apoiador-supremo": 6,
		"teste-real": 0,
	}
	for id, espacos := range querido {
		p, err := s.LerPacote(ctx, id)
		if err != nil {
			t.Fatalf("%s: %v", id, err)
		}
		if got := p.EspacosNoBau(); got != espacos {
			t.Errorf("%s ocupa %d espacos, quero %d", id, got, espacos)
		}
	}
}

// A CONFERÊNCIA RECUSA OS QUATRO CASOS, contra o banco de verdade.
func TestConferirPacoteRecusaOsQuatroCasos(t *testing.T) {
	s, ctx := freshStore(t)

	if _, err := s.ConferirPacote(ctx, "apoiador-inventado", false, 1, 1); !errors.Is(err, ErrPacoteDesconhecido) {
		t.Errorf("id inventado: erro = %v", err)
	}
	if _, err := s.ConferirPacote(ctx, "teste-real", false, 10, 100); !errors.Is(err, ErrPacoteSoStaff) {
		t.Errorf("so-staff para jogador: erro = %v", err)
	}
	if _, err := s.ConferirPacote(ctx, "apoiador-bronze", false, 5750, 4990); !errors.Is(err, ErrPacoteDivergente) {
		t.Errorf("creditos divergentes: erro = %v", err)
	}
	if _, err := s.ConferirPacote(ctx, "apoiador-bronze", false, 575, 490); !errors.Is(err, ErrPacoteDivergente) {
		t.Errorf("preco divergente: erro = %v", err)
	}

	// Desligado: o pacote existe e saiu de venda.
	if _, err := s.pool.Exec(ctx,
		`UPDATE donate_pacote SET ativo = FALSE WHERE id = 'apoiador-bronze'`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ConferirPacote(ctx, "apoiador-bronze", false, 575, 4990); !errors.Is(err, ErrPacoteDesligado) {
		t.Errorf("desligado: erro = %v", err)
	}

	// E o caminho bom, para o teste não provar só recusas: staff comprando o de staff.
	if _, err := s.ConferirPacote(ctx, "teste-real", true, 10, 100); err != nil {
		t.Errorf("staff comprando o pacote de staff: %v", err)
	}
}

// OS BRINDES ENTRAM NA MESMA TRANSAÇÃO DO CRÉDITO, e é isto que o PR existe para
// garantir.
//
// Fora dela abre a janela em que uma queda deixa os Rcoins creditados e os brindes não
// — e aí ninguém sabe o que faltou, porque a ordem já está PAGA e a repetição não
// credita de novo.
func TestConfirmarPacoteEnfileiraOsBrindesComOCredito(t *testing.T) {
	s, ctx := freshStore(t)
	conta := contaComCargo(ctx, t, s, "comprador_pacote", "player")

	if _, err := s.CreateTopupOrder(ctx, domain.TopupOrder{
		ExternalReference: "ref-supremo", AccountID: conta,
		Credits: 20000, AmountCents: 79990, PaymentMethod: 1, PacoteID: "apoiador-supremo",
	}); err != nil {
		t.Fatalf("criando a ordem: %v", err)
	}

	outcome, saldo, err := s.ConfirmTopupOrder(ctx, "ref-supremo")
	if err != nil {
		t.Fatalf("confirmando: %v", err)
	}
	if outcome != TopupConfirmed {
		t.Fatalf("resultado = %v, quero confirmado", outcome)
	}
	if saldo != 20000 {
		t.Errorf("saldo = %d, quero 20000", saldo)
	}

	// UMA LINHA NA FILA POR ESPAÇO NO BAÚ, e a conta é a MESMA que a tela do site
	// promete. Amarrar ao `EspacosNoBau` e não a um número escrito à mão é o que
	// garante que a promessa e a entrega não podem divergir — e é o que faz este teste
	// continuar valendo quando os baús passarem a empilhar, em vez de quebrar por uma
	// mudança que ele deveria acompanhar.
	pacote, err := s.LerPacote(ctx, "apoiador-supremo")
	if err != nil {
		t.Fatal(err)
	}
	entregas := brindesNaFila(ctx, t, s, conta)
	if len(entregas) != pacote.EspacosNoBau() {
		t.Fatalf("%d linhas na fila e a tela promete %d espacos; os dois numeros tem de ser o mesmo",
			len(entregas), pacote.EspacosNoBau())
	}

	// O DRAGÃO VERMELHO VEM COM A DURAÇÃO NÃO INICIADA. Se ele viesse com expires_at,
	// os 15 dias começariam a contar agora — e quem comprasse numa sexta e só entrasse
	// no domingo perderia dois dias que pagou.
	dragao := entregas[0]
	if dragao.ItemIndex != 3991 {
		t.Errorf("o primeiro brinde e %d, quero o Dragao Vermelho 3991", dragao.ItemIndex)
	}
	if dragao.ExpiresAt != 0 {
		t.Errorf("o dragao veio com expires_at = %d; a duracao tinha de ir nao iniciada",
			dragao.ExpiresAt)
	}
	if dragao.Eff1 != 106 || dragao.EffV1 != 15 {
		t.Errorf("a duracao do dragao = eff %d valor %d, quero EF_WDAY 15",
			dragao.Eff1, dragao.EffV1)
	}

	// OS BAÚS SÃO OS ÚLTIMOS, e quantas linhas eles ocupam depende de empilharem ou
	// não — que é exatamente o que o PR da pilha muda. O teste confere a REGRA, e não o
	// número: cada linha tem EF_AMOUNT, e a soma delas dá os 64 que o pacote promete.
	var baus int
	for _, e := range entregas {
		if e.ItemIndex != 3305 {
			continue
		}
		if e.Eff1 != 61 {
			t.Errorf("um bau veio sem EF_AMOUNT: eff %d", e.Eff1)
			continue
		}
		baus += int(e.EffV1)
	}
	if baus != 64 {
		t.Errorf("a soma dos baus entregues = %d, quero os 64 que o pacote promete", baus)
	}
}

// A CONFIRMAÇÃO REPETIDA NÃO ENTREGA DE NOVO. A processadora repete aviso por desenho,
// e um brinde entregue duas vezes é item criado do nada.
func TestConfirmarPacoteDuasVezesNaoDuplicaOsBrindes(t *testing.T) {
	s, ctx := freshStore(t)
	conta := contaComCargo(ctx, t, s, "comprador_repetido", "player")

	if _, err := s.CreateTopupOrder(ctx, domain.TopupOrder{
		ExternalReference: "ref-repetida", AccountID: conta,
		Credits: 300, AmountCents: 2990, PaymentMethod: 1, PacoteID: "apoiador-iniciante",
	}); err != nil {
		t.Fatal(err)
	}

	if _, _, err := s.ConfirmTopupOrder(ctx, "ref-repetida"); err != nil {
		t.Fatal(err)
	}
	outcome, saldo, err := s.ConfirmTopupOrder(ctx, "ref-repetida")
	if err != nil {
		t.Fatal(err)
	}
	if outcome != TopupAlreadyConfirmed {
		t.Errorf("a segunda confirmacao = %v, quero ja-confirmada", outcome)
	}
	if saldo != 300 {
		t.Errorf("saldo = %d, quero 300: creditou duas vezes", saldo)
	}
	// A conta vem do pacote, pelo mesmo motivo do teste de cima: ela muda quando os
	// baús passarem a empilhar, e o teste tem de acompanhar em vez de quebrar.
	iniciante, err := s.LerPacote(ctx, "apoiador-iniciante")
	if err != nil {
		t.Fatal(err)
	}
	if n := len(brindesNaFila(ctx, t, s, conta)); n != iniciante.EspacosNoBau() {
		t.Errorf("%d brindes na fila depois de duas confirmacoes, quero %d",
			n, iniciante.EspacosNoBau())
	}
}

// Doação SEM pacote não enfileira brinde nenhum, e credita normalmente. É toda ordem
// anterior aos pacotes existirem.
func TestConfirmarSemPacoteSoCredita(t *testing.T) {
	s, ctx := freshStore(t)
	conta := contaComCargo(ctx, t, s, "comprador_sem_pacote", "player")

	if _, err := s.CreateTopupOrder(ctx, domain.TopupOrder{
		ExternalReference: "ref-sem-pacote", AccountID: conta,
		Credits: 100, AmountCents: 1000, PaymentMethod: 1,
	}); err != nil {
		t.Fatal(err)
	}
	if _, saldo, err := s.ConfirmTopupOrder(ctx, "ref-sem-pacote"); err != nil || saldo != 100 {
		t.Fatalf("saldo = %d, err = %v", saldo, err)
	}
	if n := len(brindesNaFila(ctx, t, s, conta)); n != 0 {
		t.Errorf("%d brindes numa doacao sem pacote", n)
	}
}

// ContaEhStaff falha fechado: cargo que ela não reconhece vira NÃO-staff.
//
// Errar para o lado de recusar deixa a dona do servidor sem comprar o pacote de teste
// dela, o que ela resolve falando com alguém; errar para o outro lado deixa qualquer
// pessoa comprar R$ 500 de Rcoins por R$ 1,00.
func TestContaEhStaffFalhaFechado(t *testing.T) {
	s, ctx := freshStore(t)

	casos := []struct {
		cargo string
		quer  bool
	}{
		{"admin", true},
		{"moderator", true},
		{"player", false},
		{"", false},
		{"Admin", false},      // caixa diferente NÃO é reconhecida
		{"superadmin", false}, // cargo novo que ninguem acrescentou aqui
	}
	for i, c := range casos {
		conta := contaComCargo(ctx, t, s, "cargo_"+c.cargo+string(rune('a'+i)), c.cargo)
		got, err := s.ContaEhStaff(ctx, conta)
		if err != nil {
			t.Errorf("cargo %q: %v", c.cargo, err)
			continue
		}
		if got != c.quer {
			t.Errorf("cargo %q: staff = %v, quero %v", c.cargo, got, c.quer)
		}
	}

	if _, err := s.ContaEhStaff(ctx, 999999); !errors.Is(err, ErrNotFound) {
		t.Errorf("conta inexistente: erro = %v, quero ErrNotFound", err)
	}
}

// brindesNaFila lê os payloads que a confirmação enfileirou, em ordem.
func brindesNaFila(ctx context.Context, t *testing.T, s *Store, conta int64) []itemPayload {
	t.Helper()
	rows, err := s.pool.Query(ctx, `
		SELECT payload FROM delivery_queue
		 WHERE account_id = $1 AND kind = 'item' AND source LIKE 'donate_pacote:%'
		 ORDER BY id`, conta)
	if err != nil {
		t.Fatalf("lendo a fila: %v", err)
	}
	defer rows.Close()
	var out []itemPayload
	for rows.Next() {
		var carga []byte
		if err := rows.Scan(&carga); err != nil {
			t.Fatalf("lendo a fila: %v", err)
		}
		var p itemPayload
		if err := json.Unmarshal(carga, &p); err != nil {
			t.Fatalf("payload ilegivel: %v", err)
		}
		out = append(out, p)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("lendo a fila: %v", err)
	}
	return out
}
