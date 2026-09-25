//go:build integration

// O repasse ao vendedor nasce LÍQUIDO, e nunca sai cheio quando a taxa é desconhecida.
//
//	W2PP_TEST_DSN=postgres://postgres:dev@localhost:5432/postgres go test -tags=integration ./internal/store/
package store

import (
	"context"
	"errors"
	"testing"
)

// taxaZero é a taxa CONHECIDA e igual a zero, usada pelos testes que existiam antes da
// 0131 e que afirmam valores.
//
// Conhecida e zero deixa o líquido igual ao bruto, então nenhuma afirmação antiga muda
// de sentido. E é diferente de nulo de propósito: nulo faria todos aqueles repasses
// nascerem SEGURADOS, e testes sobre a fila de pagar passariam a falar de um estado que
// não é o que eles estão medindo.
func taxaZero() *int64 {
	z := int64(0)
	return &z
}

func taxaDe(centavos int64) *int64 { return &centavos }

// repasseDaCobranca lê a linha crua do repasse. Os testes de dinheiro leem a COLUNA, e
// não o que uma função devolve: uma função pode calcular certo e gravar errado, e é
// exatamente esse o erro que já custou um buraco de dinheiro neste sistema.
func repasseDaCobranca(ctx context.Context, t *testing.T, s *Store, cobrancaID int64) (liquido int64, bruto *int64, status int16) {
	t.Helper()
	if err := s.pool.QueryRow(ctx, `
		SELECT valor_centavos, bruto_centavos, status
		  FROM rmt_repasse WHERE cobranca_id = $1`, cobrancaID).
		Scan(&liquido, &bruto, &status); err != nil {
		t.Fatalf("lendo o repasse da cobranca %d: %v", cobrancaID, err)
	}
	return liquido, bruto, status
}

// COM A TAXA CONHECIDA, O VENDEDOR RECEBE O BRUTO MENOS A TAXA, e o bruto fica gravado.
//
// Este é o conserto: antes o repasse nascia com o valor CHEIO e a casa bancava a taxa
// em silêncio. Não era uma decisão, era a ausência do dado.
func TestRepasseNasceLiquidoEGuardaOBruto(t *testing.T) {
	s, ctx := freshStore(t)
	v := montaVenda(ctx, t, s, "taxa_liquido")

	res, venda, err := s.ConfirmarCobrancaRMT(ctx, v.ref, dentroDoPrazo(), HoraDaProcessadora,
		precoEmCentavos, taxaDe(137))
	if err != nil {
		t.Fatal(err)
	}
	if res != CobrancaConfirmada {
		t.Fatalf("resultado = %v, queria confirmada", res)
	}

	liquido, bruto, status := repasseDaCobranca(ctx, t, s, venda.CobrancaID)
	if liquido != precoEmCentavos-137 {
		t.Errorf("liquido = %d, queria %d (bruto %d menos a taxa 137)",
			liquido, precoEmCentavos-137, precoEmCentavos)
	}
	if bruto == nil || *bruto != precoEmCentavos {
		t.Errorf("bruto gravado = %v, queria %d", bruto, precoEmCentavos)
	}
	if status != repassePendente {
		t.Errorf("status = %d, queria pendente: com a taxa conhecida nada segura", status)
	}
}

// SEM A TAXA, O REPASSE SEGURA — e NÃO sai com o valor cheio.
//
// As duas outras saídas possíveis eram piores. Pagar o bruto faz a casa perder a taxa
// sem ninguém notar, porque um repasse de valor cheio tem a cara de um repasse certo.
// Não criar o repasse deixa o vendedor invisível, que é o bug que a 0124 consertou.
func TestSemTaxaORepasseSeguraEmVezDeSairCheio(t *testing.T) {
	s, ctx := freshStore(t)
	v := montaVenda(ctx, t, s, "taxa_segura")

	_, venda, err := s.ConfirmarCobrancaRMT(ctx, v.ref, dentroDoPrazo(), HoraDaProcessadora,
		precoEmCentavos, nil)
	if err != nil {
		t.Fatal(err)
	}

	_, _, status := repasseDaCobranca(ctx, t, s, venda.CobrancaID)
	if status != repasseSemTaxa {
		t.Fatalf("status = %d, queria %d (segurado por taxa desconhecida)", status, repasseSemTaxa)
	}

	// E NÃO APARECE NA FILA DE PAGAR. É a metade que vale dinheiro: um estado novo que
	// a fila enxergasse por engano pagaria o valor cheio de qualquer forma, e o estado
	// não teria servido para nada.
	fila, err := s.RepassesAPagar(ctx, 50)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range fila {
		if r.CobrancaID == venda.CobrancaID {
			t.Errorf("o repasse segurado apareceu na fila de pagar por %d centavos", r.ValorCentavos)
		}
	}
}

// TAXA QUE COME A VENDA INTEIRA TAMBÉM SEGURA. Líquido zero ou negativo não é
// pagamento: zero manda a ponte transferir nada, e negativo seria um saque ao contrário.
//
// Com o preço mínimo em R$ 1,00, uma taxa que empata com a venda é cenário alcançável,
// e não hipótese de laboratório.
func TestTaxaQueComeAVendaSegura(t *testing.T) {
	for _, taxa := range []int64{precoEmCentavos, precoEmCentavos + 1} {
		s, ctx := freshStore(t)
		v := montaVenda(ctx, t, s, "taxa_come")

		_, venda, err := s.ConfirmarCobrancaRMT(ctx, v.ref, dentroDoPrazo(), HoraDaProcessadora,
			precoEmCentavos, taxaDe(taxa))
		if err != nil {
			t.Fatal(err)
		}
		liquido, _, status := repasseDaCobranca(ctx, t, s, venda.CobrancaID)
		if status != repasseSemTaxa {
			t.Errorf("taxa %d: status = %d, queria segurado", taxa, status)
		}
		if liquido <= 0 {
			t.Errorf("taxa %d: gravou liquido %d, que nao e pagamento nenhum", taxa, liquido)
		}
	}
}

// A PORTA DE SAÍDA FUNCIONA: informada a taxa, o repasse vira pendente com o líquido.
//
// Um estado que segura dinheiro sem saída é armadilha e não proteção — o vendedor
// esperaria um dia que não chega. É por isso que a saída nasce no mesmo commit.
func TestInformarATaxaLiberaORepasseComOLiquido(t *testing.T) {
	s, ctx := freshStore(t)
	v := montaVenda(ctx, t, s, "taxa_informa")
	_, venda, err := s.ConfirmarCobrancaRMT(ctx, v.ref, dentroDoPrazo(), HoraDaProcessadora,
		precoEmCentavos, nil)
	if err != nil {
		t.Fatal(err)
	}

	liberou, err := s.InformarTaxaDaCobranca(ctx, venda.CobrancaID, 89, AtorDoRepasse{Nome: "hanna"})
	if err != nil {
		t.Fatal(err)
	}
	if !liberou {
		t.Fatal("nao liberou o repasse segurado")
	}

	liquido, _, status := repasseDaCobranca(ctx, t, s, venda.CobrancaID)
	if status != repassePendente || liquido != precoEmCentavos-89 {
		t.Errorf("status = %d, liquido = %d; queria pendente e %d",
			status, liquido, precoEmCentavos-89)
	}

	// A taxa fica na COBRANÇA, que é onde o fato do dinheiro que entrou pertence.
	var taxa *int64
	if err := s.pool.QueryRow(ctx, `SELECT taxa_centavos FROM rmt_cobranca WHERE id = $1`,
		venda.CobrancaID).Scan(&taxa); err != nil {
		t.Fatal(err)
	}
	if taxa == nil || *taxa != 89 {
		t.Errorf("taxa na cobranca = %v, queria 89", taxa)
	}

	// REPETIR NÃO MEXE DE NOVO. A segunda chamada acha um repasse que já não está
	// segurado e devolve false: sem isso, um clique duplo no painel subtrairia a taxa
	// duas vezes do mesmo dinheiro.
	deNovo, err := s.InformarTaxaDaCobranca(ctx, venda.CobrancaID, 89, AtorDoRepasse{Nome: "hanna"})
	if err != nil {
		t.Fatal(err)
	}
	if deNovo {
		t.Error("a segunda chamada disse que liberou de novo")
	}
	if liquido2, _, _ := repasseDaCobranca(ctx, t, s, venda.CobrancaID); liquido2 != liquido {
		t.Errorf("o liquido mudou na repeticao: %d -> %d", liquido, liquido2)
	}
}

// TAXA IMPOSSÍVEL NA PORTA DE SAÍDA NÃO LIBERA NADA. O repasse fica segurado, que é
// onde ele deve ficar enquanto o número não fizer sentido.
func TestInformarTaxaImpossivelNaoLibera(t *testing.T) {
	s, ctx := freshStore(t)
	v := montaVenda(ctx, t, s, "taxa_impossivel")
	_, venda, err := s.ConfirmarCobrancaRMT(ctx, v.ref, dentroDoPrazo(), HoraDaProcessadora,
		precoEmCentavos, nil)
	if err != nil {
		t.Fatal(err)
	}

	for _, taxa := range []int64{-1, precoEmCentavos, precoEmCentavos + 500} {
		liberou, err := s.InformarTaxaDaCobranca(ctx, venda.CobrancaID, taxa, AtorDoRepasse{Nome: "hanna"})
		if !errors.Is(err, ErrTaxaImpossivel) {
			t.Errorf("taxa %d: erro = %v, queria ErrTaxaImpossivel", taxa, err)
		}
		if liberou {
			t.Errorf("taxa %d: liberou", taxa)
		}
		if _, _, status := repasseDaCobranca(ctx, t, s, venda.CobrancaID); status != repasseSemTaxa {
			t.Errorf("taxa %d: status = %d, queria continuar segurado", taxa, status)
		}
	}
}

// E O VENDEDOR VÊ O DINHEIRO, com o motivo dizendo que alguém está olhando.
//
// Deixá-lo fora da soma faria a página mostrar zero com dinheiro a receber — a
// invisibilidade que RepasseEsperandoCadastro existe para não repetir. O total é o
// BRUTO, porque sem a taxa não há líquido; é o motivo que impede a tela de prometer
// aquele número como valor final.
func TestVendedorVeORepasseSeguradoComoEsperaGente(t *testing.T) {
	s, ctx := freshStore(t)
	v := montaVenda(ctx, t, s, "taxa_ve")
	if _, _, err := s.ConfirmarCobrancaRMT(ctx, v.ref, dentroDoPrazo(), HoraDaProcessadora,
		precoEmCentavos, nil); err != nil {
		t.Fatal(err)
	}

	total, motivo, err := s.RepasseDoVendedor(ctx, v.vendedor)
	if err != nil {
		t.Fatal(err)
	}
	if total != precoEmCentavos {
		t.Errorf("total = %d, queria %d", total, precoEmCentavos)
	}
	if motivo != EsperaGente {
		t.Errorf("motivo = %d, queria EsperaGente", motivo)
	}
}

// O SEGURADO TRAVA A TROCA DE CHAVE, como o pendente e o incerto.
//
// É o oposto do recusado, e a diferença importa: na recusa o problema PODE ser a chave,
// então corrigi-la é o conserto. Aqui a chave está boa e o que falta é um número nosso —
// deixar trocar abriria o desvio exato que a trava existe para impedir: vender, esperar
// o repasse segurar, e apontar o dinheiro para outra chave antes de ele sair.
func TestRepasseSeguradoTravaATrocaDeChave(t *testing.T) {
	s, ctx := freshStore(t)
	v := montaVenda(ctx, t, s, "taxa_trava")
	if _, _, err := s.ConfirmarCobrancaRMT(ctx, v.ref, dentroDoPrazo(), HoraDaProcessadora,
		precoEmCentavos, nil); err != nil {
		t.Fatal(err)
	}

	err := s.SalvarChavePix(ctx, v.vendedor, "outra-chave@exemplo.com", ChavePixEmail, "11144477735")
	if !errors.Is(err, ErrVendaEmCurso) {
		t.Errorf("erro = %v, queria ErrVendaEmCurso: o segurado nao pode deixar desviar", err)
	}
}
