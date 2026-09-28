package ponte

import (
	"encoding/json"
	"testing"
)

// O DECODIFICADOR TEM DE LER O CAMPO DA PONTE COM O NOME E O TIPO EXATOS.
//
// A ponte calcula a taxa como amount menos net_amount — a V2 real da SyncPay não tem o
// campo `fees` que a documentação prometia — e manda em `taxaCentavos`, inteiro em
// centavos. Na venda de R$ 1,00 de 25/09/2026 deu 80.
//
// Este teste existe porque um nome de campo errado falha CALADO em JSON: o campo some,
// o ponteiro fica nulo, e nulo é o caminho de "não sei a taxa" — que SEGURA o repasse.
// Não haveria erro, nem log, nem exceção. Todo repasse pararia em "taxa desconhecida" e
// a causa seria uma letra maiúscula.
func TestLeATaxaDaPonteComONomeExato(t *testing.T) {
	// Corpo no formato que a ponte manda, com os campos vizinhos junto: um teste que
	// decodifica só o campo de interesse não pega um nome que colide com outro.
	const corpo = `{"estado":"achada","status":"completed","referencia":"abc",
		"valorCentavos":100,"taxaCentavos":80,"pagoEm":null,"devolvidoEm":null}`

	var r RespostaConsulta
	if err := json.Unmarshal([]byte(corpo), &r); err != nil {
		t.Fatalf("nao decodificou: %v", err)
	}
	if r.TaxaCentavos == nil {
		t.Fatal("taxaCentavos veio NULA com o campo presente; o nome no struct nao casa com o da ponte")
	}
	if *r.TaxaCentavos != 80 {
		t.Errorf("taxaCentavos = %d, queria 80", *r.TaxaCentavos)
	}
	if r.ValorCentavos != 100 {
		t.Errorf("valorCentavos = %d, queria 100", r.ValorCentavos)
	}
}

// AUSENTE E NULO SÃO A MESMA COISA, e as duas têm de virar ponteiro nulo — que é o que
// segura o repasse em vez de pagar o valor cheio.
func TestTaxaAusenteOuNulaViramNulo(t *testing.T) {
	casos := map[string]string{
		"campo ausente": `{"estado":"achada","valorCentavos":100}`,
		"campo nulo":    `{"estado":"achada","valorCentavos":100,"taxaCentavos":null}`,
	}
	for nome, corpo := range casos {
		var r RespostaConsulta
		if err := json.Unmarshal([]byte(corpo), &r); err != nil {
			t.Fatalf("%s: nao decodificou: %v", nome, err)
		}
		if r.TaxaCentavos != nil {
			t.Errorf("%s: taxa = %d, queria nulo", nome, *r.TaxaCentavos)
		}
	}
}

// ZERO É UMA AFIRMAÇÃO e chega como zero, não como ausência.
//
// É o espelho do teste de cima, e os dois juntos são o que prende a distinção que vale
// dinheiro: zero paga o valor cheio de propósito, nulo segura.
func TestTaxaZeroChegaComoZeroENaoComoAusencia(t *testing.T) {
	var r RespostaConsulta
	if err := json.Unmarshal([]byte(`{"estado":"achada","taxaCentavos":0}`), &r); err != nil {
		t.Fatal(err)
	}
	if r.TaxaCentavos == nil {
		t.Fatal("taxa ZERO virou ausencia; zero e uma afirmacao e nulo e ignorancia")
	}
	if *r.TaxaCentavos != 0 {
		t.Errorf("taxa = %d, queria 0", *r.TaxaCentavos)
	}
}
