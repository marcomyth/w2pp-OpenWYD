//go:build integration

// Os nove pacotes espelho de R$ 1,00: existem, são só da staff, e entregam
// EXATAMENTE o que o pacote real entrega.
//
//	W2PP_TEST_DSN=postgres://postgres:dev@localhost:5432/postgres go test -tags=integration ./internal/store/
package store

import (
	"context"
	"errors"
	"testing"
)

// osNoveApoiadores são os pacotes que ganharam espelho. O `teste-real` fica fora: ele
// já é de R$ 1,00 e não tem brinde, então espelhá-lo não testaria entrega nenhuma.
var osNoveApoiadores = []string{
	"apoiador-iniciante", "apoiador-bronze", "apoiador-prata",
	"apoiador-ouro", "apoiador-platina", "apoiador-diamante",
	"apoiador-mestre", "apoiador-lenda", "apoiador-supremo",
}

// TestOsEspelhosEntregamOMesmoQueOPacoteReal.
//
// O ESPELHO SÓ VALE SE ENTREGAR O MESMO. O teste do chefe é da ENTREGA dos brindes: um
// espelho com um item de menos, ou com um efeito diferente, faria o teste passar e a
// compra de verdade falhar depois — que é o pior resultado possível, porque ninguém
// olharia de novo.
//
// Por isso a comparação é item por item e efeito por efeito, e não a contagem. Contar
// deixa passar exatamente o erro que mais acontece numa cópia: a linha certa com o
// número errado dentro.
//
// UM freshStore PARA OS TRÊS, de propósito: cada chamada recria o schema inteiro (115
// migrações), e a suíte de integração já é a que estoura o tempo da CI. Os três só leem
// pacote, e as contas que o segundo cria têm nome próprio - não há estado compartilhado
// que um possa estragar para o outro.
func TestOsPacotesEspelho(t *testing.T) {
	s, ctx := freshStore(t)
	t.Run("entregam o mesmo que o pacote real", func(t *testing.T) { osEspelhosEntregamOMesmo(t, s, ctx) })
	t.Run("recusam quem nao e staff", func(t *testing.T) { oEspelhoRecusaQuemNaoEStaff(t, s, ctx) })
	t.Run("nao mexeram no preco do real", func(t *testing.T) { osPacotesReaisNaoMudaramDePreco(t, s, ctx) })
}

func osEspelhosEntregamOMesmo(t *testing.T, s *Store, ctx context.Context) {
	for _, real := range osNoveApoiadores {
		espelho := "teste-" + real
		pr, err := s.LerPacote(ctx, real)
		if err != nil {
			t.Fatalf("%s: %v", real, err)
		}
		pe, err := s.LerPacote(ctx, espelho)
		if err != nil {
			t.Errorf("%s nao existe: %v", espelho, err)
			continue
		}
		if pe.AmountCents != 100 {
			t.Errorf("%s custa %d centavos, quero 100", espelho, pe.AmountCents)
		}
		if !pe.SoStaff {
			t.Errorf("%s nasceu aberto a qualquer conta", espelho)
		}
		if !pe.Ativo {
			t.Errorf("%s nasceu desligado", espelho)
		}
		// Os Rcoins são os do real: espelho que credita menos não prova a entrega.
		if pe.Credits != pr.Credits {
			t.Errorf("%s da %d Rcoins, o real da %d", espelho, pe.Credits, pr.Credits)
		}
		if len(pe.Itens) != len(pr.Itens) {
			t.Errorf("%s tem %d brindes, o real tem %d", espelho, len(pe.Itens), len(pr.Itens))
			continue
		}
		for i := range pr.Itens {
			if pe.Itens[i] != pr.Itens[i] {
				t.Errorf("%s brinde %d = %+v, o real entrega %+v",
					espelho, i, pe.Itens[i], pr.Itens[i])
			}
		}
		// O preço que o site tem de mandar é o DAQUI: a conferência compara o pedido
		// contra esta linha, então R$ 1,00 com os Rcoins do real é o par válido.
		if _, err := s.ConferirPacote(ctx, espelho, true, pr.Credits, 100); err != nil {
			t.Errorf("%s recusou a compra da staff pelo preço certo: %v", espelho, err)
		}
	}
}

// TestOEspelhoRecusaQuemNaoEStaff.
//
// A trava é o `so_staff`, e ela vive no SERVIDOR: o site esconde o pacote, mas esconder
// na tela não é trava — quem chama a RPC direto passa por ela. Sem esta recusa,
// qualquer conta compraria o Supremo por R$ 1,00 enquanto o teste durasse.
//
// Os três cargos aqui são os que o `ContaEhStaff` decide: 'admin' e 'moderator' passam,
// e o resto não. Um cargo novo que precise comprar espelho tem de ser adicionado LÁ, e
// este teste é o que faz isso aparecer.
func oEspelhoRecusaQuemNaoEStaff(t *testing.T, s *Store, ctx context.Context) {
	const espelho = "teste-apoiador-supremo"
	pr, err := s.LerPacote(ctx, "apoiador-supremo")
	if err != nil {
		t.Fatalf("apoiador-supremo: %v", err)
	}

	casos := []struct {
		cargo string
		passa bool
	}{
		{"admin", true},
		{"moderator", true},
		{"", false},
		{"player", false},
	}
	for _, c := range casos {
		id := contaComCargo(ctx, t, s, "espelho-"+c.cargo+"-"+espelho, c.cargo)
		ehStaff, err := s.ContaEhStaff(ctx, id)
		if err != nil {
			t.Fatalf("cargo %q: %v", c.cargo, err)
		}
		if ehStaff != c.passa {
			t.Errorf("cargo %q: ContaEhStaff = %v, quero %v", c.cargo, ehStaff, c.passa)
		}
		_, err = s.ConferirPacote(ctx, espelho, ehStaff, pr.Credits, 100)
		switch {
		case c.passa && err != nil:
			t.Errorf("cargo %q foi recusado no espelho: %v", c.cargo, err)
		case !c.passa && !errors.Is(err, ErrPacoteSoStaff):
			t.Errorf("cargo %q comprou o espelho (erro = %v), e ele e so da staff", c.cargo, err)
		}
	}
}

// osPacotesReaisEstaoAUmReal, E ESTE TESTE JÁ FOI O CONTRÁRIO.
//
// Ele nasceu de manhã prendendo os nove preços de tabela, para impedir que alguém
// baixasse os pacotes reais e o Supremo ficasse a R$ 1,00 para qualquer conta. À tarde a
// Hanna mandou baixar mesmo assim, por escrito e sabendo desse custo (migração 0156).
//
// ENTÃO ELE MUDOU DE LADO, e não foi apagado: agora prende que os nove estão a 100 E que
// os créditos continuam os de sempre. O que ele protege é o mesmo de antes — que o preço
// dos pacotes reais não mude sem alguém decidir — só que o número combinado agora é
// outro. Um teste apagado deixaria a próxima mudança passar calada.
//
// QUANDO OS PREÇOS VOLTAREM (migração nova, quando ela mandar), é aqui que os valores da
// 0123 voltam: 2990, 4990, 9990, 14990, 19990, 29990, 39990, 49990, 79990.
func osPacotesReaisNaoMudaramDePreco(t *testing.T, s *Store, ctx context.Context) {
	// R$ 1,00 nos nove, por decisão da Hanna de 25/09/2026.
	querido := map[string]int64{
		"apoiador-iniciante": 100, "apoiador-bronze": 100, "apoiador-prata": 100,
		"apoiador-ouro": 100, "apoiador-platina": 100, "apoiador-diamante": 100,
		"apoiador-mestre": 100, "apoiador-lenda": 100, "apoiador-supremo": 100,
	}
	// OS CRÉDITOS NÃO MUDARAM, e é a metade que vale dinheiro: baixar o preço e mexer nos
	// Rcoins sem perceber daria pacote barato E menor, ou barato E maior.
	creditos := map[string]int32{
		"apoiador-iniciante": 300, "apoiador-bronze": 575, "apoiador-prata": 1250,
		"apoiador-ouro": 2100, "apoiador-platina": 3000, "apoiador-diamante": 4950,
		"apoiador-mestre": 7000, "apoiador-lenda": 10000, "apoiador-supremo": 20000,
	}
	for id, centavos := range querido {
		p, err := s.LerPacote(ctx, id)
		if err != nil {
			t.Fatalf("%s: %v", id, err)
		}
		if p.AmountCents != centavos {
			t.Errorf("%s custa %d centavos, quero %d", id, p.AmountCents, centavos)
		}
		if p.SoStaff {
			t.Errorf("%s virou so-staff: o pacote de verdade tem de continuar a venda", id)
		}
		if p.Credits != creditos[id] {
			t.Errorf("%s da %d Rcoins, quero %d: o preco mudou, o credito nao devia",
				id, p.Credits, creditos[id])
		}
	}
}
