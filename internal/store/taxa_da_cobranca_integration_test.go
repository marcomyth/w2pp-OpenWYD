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
	// O LÍQUIDO SAI DA TAXA DA CASA, e não da taxa da processadora que este teste
	// informa. A conta prova a diferença: com desconto duplo daria 4533
	// (5000 − 137 − 330), e o que se espera é 4670 (5000 − 330). A taxa da
	// processadora virou custo da casa em 25/09/2026.
	if liquido != liquidoDaVendaDeTeste() {
		t.Errorf("liquido = %d, queria %d (bruto %d menos a taxa DA CASA)",
			liquido, liquidoDaVendaDeTeste(), precoEmCentavos)
	}
	if bruto == nil || *bruto != precoEmCentavos {
		t.Errorf("bruto gravado = %v, queria %d", bruto, precoEmCentavos)
	}
	if status != repassePendente {
		t.Errorf("status = %d, queria pendente: com a taxa conhecida nada segura", status)
	}
}

// SEM A TAXA DA PROCESSADORA, O REPASSE NÃO SEGURA MAIS — e este teste MUDOU DE LADO.
//
// ELE DIZIA O CONTRÁRIO, e estava certo na regra de então: o líquido era o bruto menos a
// taxa da processadora, então sem ela não havia líquido, e pagar o bruto faria a casa
// perder a taxa sem ninguém notar.
//
// A regra mudou em 25/09/2026: quem decide o líquido é a taxa DA CASA, conhecida antes de
// o anúncio subir. Não há mais o que esperar, e segurar aqui deixaria o vendedor
// aguardando um número que já não interessa a ele.
//
// O ESTADO repasseSemTaxa CONTINUA EXISTINDO para as linhas antigas, e o teste da porta
// de saída delas está logo abaixo.
func TestSemTaxaDaProcessadoraORepasseNaoSegura(t *testing.T) {
	s, ctx := freshStore(t)
	v := montaVenda(ctx, t, s, "taxa_nao_segura")

	_, venda, err := s.ConfirmarCobrancaRMT(ctx, v.ref, dentroDoPrazo(), HoraDaProcessadora,
		precoEmCentavos, nil)
	if err != nil {
		t.Fatal(err)
	}

	liquido, _, status := repasseDaCobranca(ctx, t, s, venda.CobrancaID)
	if status == repasseSemTaxa {
		t.Fatal("a venda nova ficou segurada esperando a taxa da processadora, que ja nao decide nada")
	}
	if status != repassePendente {
		t.Errorf("status = %d, queria pendente", status)
	}
	if liquido != liquidoDaVendaDeTeste() {
		t.Errorf("liquido = %d, queria %d", liquido, liquidoDaVendaDeTeste())
	}

	// E APARECE NA FILA DE PAGAR, que é a outra metade: um repasse pendente que a fila
	// não enxergasse seria dinheiro parado sem ninguém saber.
	id := idDoRepasse(ctx, t, s, venda.CobrancaID)
	fila, err := s.FilaDePagamentoAMao(ctx, 50)
	if err != nil {
		t.Fatal(err)
	}
	achou := false
	for _, r := range fila {
		if r.ID == id {
			achou = true
		}
	}
	if !achou {
		t.Error("o repasse nao apareceu na fila de pagar")
	}
}

// TAXA ABSURDA DA PROCESSADORA TAMBÉM NÃO SEGURA MAIS, e este teste mudou de lado pelo
// mesmo motivo do anterior.
//
// Ele existia porque uma taxa igual ou maior que a venda daria líquido zero ou negativo —
// nenhum dos dois é pagamento. Com a taxa da casa decidindo, o líquido não depende mais
// desse número, e um valor absurdo vindo da processadora é problema de MARGEM da casa, e
// não do vendedor: ele recebe o que foi prometido.
//
// O que este teste passa a provar é justamente isso: o vendedor não é punido por um
// número que não é dele.
func TestTaxaAbsurdaDaProcessadoraNaoAfetaOVendedor(t *testing.T) {
	for _, taxa := range []int64{precoEmCentavos, precoEmCentavos + 1} {
		s, ctx := freshStore(t)
		v := montaVenda(ctx, t, s, "taxa_absurda")

		_, venda, err := s.ConfirmarCobrancaRMT(ctx, v.ref, dentroDoPrazo(), HoraDaProcessadora,
			precoEmCentavos, taxaDe(taxa))
		if err != nil {
			t.Fatal(err)
		}
		liquido, _, status := repasseDaCobranca(ctx, t, s, venda.CobrancaID)
		if status != repassePendente {
			t.Errorf("taxa %d: status = %d, queria pendente", taxa, status)
		}
		if liquido != liquidoDaVendaDeTeste() {
			t.Errorf("taxa %d: liquido = %d, queria %d: a taxa da processadora nao sai do "+
				"bolso do vendedor", taxa, liquido, liquidoDaVendaDeTeste())
		}
	}
}

// A PORTA DE SAÍDA DAS LINHAS ANTIGAS CONTINUA FUNCIONANDO, e este teste NÃO foi apagado
// quando a regra mudou.
//
// Vendas novas já não nascem seguradas, mas as que foram gravadas sob a regra anterior
// estão no banco de produção agora, esperando. Apagar este teste junto com a regra velha
// deixaria aquelas linhas sem caminho de saída e ninguém saberia — o vendedor esperaria
// um dia que não chega.
//
// O líquido aqui sai do desconto da PROCESSADORA de propósito: é a conta da regra antiga,
// que é a regra sob a qual aquelas linhas foram criadas.
func TestInformarATaxaLiberaORepasseComOLiquido(t *testing.T) {
	s, ctx := freshStore(t)
	v := montaVenda(ctx, t, s, "taxa_informa")
	_, venda, err := s.ConfirmarCobrancaRMT(ctx, v.ref, dentroDoPrazo(), HoraDaProcessadora,
		precoEmCentavos, nil)
	if err != nil {
		t.Fatal(err)
	}

	// A LINHA SEGURADA É MONTADA À MÃO, e antes ela nascia assim sozinha.
	//
	// Desde 25/09/2026 uma venda nova NUNCA nasce em repasseSemTaxa: quem decide o
	// líquido é a taxa da casa, conhecida antes do anúncio. Mas as linhas gravadas sob a
	// regra ANTIGA continuam no banco de produção, e a porta de saída delas tem de
	// continuar funcionando — um estado que segura dinheiro sem saída é armadilha, não
	// proteção.
	//
	// Por isso o estado é escrito por SQL, num arquivo de TESTE: uma função de produção
	// capaz de empurrar um repasse de volta para segurado seria exatamente o caminho
	// morto que este PR fecha.
	if _, err := s.pool.Exec(ctx, `
		UPDATE rmt_repasse SET status = $2, valor_centavos = $3
		 WHERE cobranca_id = $1`, venda.CobrancaID, repasseSemTaxa, precoEmCentavos); err != nil {
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
	// O erro é PRÓPRIO do repasse, e não o da cobrança aberta: os dois pedem coisas
	// diferentes de quem vende — a cobrança aberta passa sozinha quando a compra
	// fechar, o repasse só quando o dinheiro sair.
	if !errors.Is(err, ErrRepasseEmCurso) {
		t.Errorf("erro = %v, queria ErrRepasseEmCurso: o segurado nao pode deixar desviar", err)
	}
}
