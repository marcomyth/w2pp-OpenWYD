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
		// DESLIGADO DESDE A 0159, que acabou o teste de entrega. A linha continua
		// existindo de propósito (regra da 0122: pedido antigo tem de achar a linha
		// dele), e o resto da conferência abaixo continua valendo — é ela que garante
		// que o espelho ainda é cópia fiel se alguém religar para um teste novo.
		if pe.Ativo {
			t.Errorf("%s continua ligado: o teste de entrega acabou e a 0159 devia te-lo fechado", espelho)
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
		// E NEM A STAFF COMPRA MAIS, mesmo com o preço e os créditos certos: a recusa
		// vem do desligado, antes da trava de staff. Era aqui que o teste conferia a
		// compra do espelho; agora é aqui que ele confere que ela acabou.
		if _, err := s.ConferirPacote(ctx, espelho, true, pr.Credits, 100); !errors.Is(err, ErrPacoteDesligado) {
			t.Errorf("%s ainda vende para a staff (erro = %v), e o teste de entrega acabou", espelho, err)
		}
	}
}

// oEspelhoRecusaQuemNaoEStaff: a trava de cargo, medida no pacote só-staff que
// continua LIGADO.
//
// A trava é o `so_staff`, e ela vive no SERVIDOR: o site esconde o pacote, mas esconder
// na tela não é trava — quem chama a RPC direto passa por ela.
//
// O ALVO MUDOU PARA O `teste-real` DEPOIS DA 0159, que desligou os nove espelhos. Contra
// um pacote desligado esta medição não diria mais nada: o `ConferirPacote` recusa pelo
// desligado ANTES de olhar o cargo, então todos os quatro casos dariam a mesma resposta
// e o teste passaria com a trava de cargo quebrada. O `teste-real` é só-staff e continua
// ligado, então é nele que a pergunta ainda tem duas respostas possíveis.
//
// Os quatro cargos são os que o `ContaEhStaff` decide: 'admin' e 'moderator' passam, e o
// resto não. Um cargo novo que precise comprar tem de ser adicionado LÁ, e este teste é
// o que faz isso aparecer.
func oEspelhoRecusaQuemNaoEStaff(t *testing.T, s *Store, ctx context.Context) {
	const espelho = "teste-real"
	pr, err := s.LerPacote(ctx, espelho)
	if err != nil {
		t.Fatalf("%s: %v", espelho, err)
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
		_, err = s.ConferirPacote(ctx, espelho, ehStaff, pr.Credits, pr.AmountCents)
		switch {
		case c.passa && err != nil:
			t.Errorf("cargo %q foi recusado no espelho: %v", c.cargo, err)
		case !c.passa && !errors.Is(err, ErrPacoteSoStaff):
			t.Errorf("cargo %q comprou o espelho (erro = %v), e ele e so da staff", c.cargo, err)
		}
	}
}

// osPacotesReaisNaoMudaramDePreco, E ESTE TESTE JÁ MUDOU DE LADO DUAS VEZES NUM DIA.
//
// Ele nasceu de manhã prendendo os nove preços de tabela. À tarde a Hanna mandou baixar
// tudo para R$ 1,00 para testar a entrega (migração 0158) e ele passou a prender o 100.
// À noite o teste acabou e os preços voltaram (migração 0159), então ele volta a prender
// os valores de tabela.
//
// NUNCA FOI APAGADO em nenhuma das duas viradas, e é esse o ponto: o que ele protege não
// é um número, é a regra de que o preço dos pacotes reais não muda sem alguém decidir.
// Um teste apagado nas idas e vindas deixaria a próxima mudança passar calada — e a
// mudança que passa calada, aqui, é o Supremo aberto a R$ 1,00 para o mundo inteiro.
//
// OS VALORES SÃO OS DA 0123, que é onde nasceram. Conferidos contra ela na volta.
func osPacotesReaisNaoMudaramDePreco(t *testing.T, s *Store, ctx context.Context) {
	// Os preços de tabela, de volta em 25/09/2026 pela 0159.
	querido := map[string]int64{
		"apoiador-iniciante": 2990, "apoiador-bronze": 4990, "apoiador-prata": 9990,
		"apoiador-ouro": 14990, "apoiador-platina": 19990, "apoiador-diamante": 29990,
		"apoiador-mestre": 39990, "apoiador-lenda": 49990, "apoiador-supremo": 79990,
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
