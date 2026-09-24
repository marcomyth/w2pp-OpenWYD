package ponte

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"
)

const segredoDeTeste = "segredo-da-ponte"

// pontefalsa é um servidor que confere a assinatura DO MESMO JEITO que a ponte
// confere, e não um que sempre aceita.
//
// A diferença é o teste inteiro: um servidor que ignorasse a assinatura passaria
// com o cliente assinando errado, e o erro só apareceria em produção, como um 401
// cuja mensagem não explica nada.
func pontefalsa(t *testing.T, responde func(rota string, corpo []byte) (int, string)) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		corpo, _ := io.ReadAll(r.Body)

		em := r.Header.Get("x-ponte-em")
		if em == "" {
			t.Error("o pedido foi sem o cabecalho do instante")
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		ms, err := strconv.ParseInt(em, 10, 64)
		if err != nil {
			t.Errorf("o instante %q nao e epoch em ms", em)
		}
		if d := time.Since(time.UnixMilli(ms)); d > 5*time.Minute || d < -5*time.Minute {
			t.Errorf("o instante esta fora da janela de cinco minutos: %v", d)
		}

		mac := hmac.New(sha256.New, []byte(segredoDeTeste))
		mac.Write([]byte(em))
		mac.Write([]byte("\n"))
		mac.Write(corpo)
		querido := hex.EncodeToString(mac.Sum(nil))
		if veio := r.Header.Get("x-ponte-assinatura"); !hmac.Equal([]byte(veio), []byte(querido)) {
			t.Errorf("assinatura errada em %s", r.URL.Path)
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if r.Method != http.MethodPost {
			t.Errorf("metodo = %s, quero POST — o corpo assinado e o que protege", r.Method)
		}

		codigo, json := responde(r.URL.Path, corpo)
		w.Header().Set("content-type", "application/json")
		w.WriteHeader(codigo)
		_, _ = io.WriteString(w, json)
	}))
	t.Cleanup(srv.Close)
	return srv
}

// clienteContra monta um cliente apontado para o servidor de mentira.
//
// Sem mTLS: o httptest não é a VPS, e o que este teste mede é a ASSINATURA e a
// leitura das respostas. O mTLS é conferido no teste da configuração, que prova que
// o cliente não sobe sem o par de certificado.
func clienteContra(t *testing.T, url string) *Cliente {
	t.Helper()
	return &Cliente{url: url, segredo: []byte(segredoDeTeste),
		http: &http.Client{Timeout: 5 * time.Second}, agora: time.Now}
}

// A cobrança criada volta com o código, e o pedido vai com o que o contrato manda.
func TestCriarCobrancaAssinaEDevolveOCodigo(t *testing.T) {
	var visto string
	srv := pontefalsa(t, func(rota string, corpo []byte) (int, string) {
		visto = rota + " " + string(corpo)
		return 200, `{"estado":"criada","identifier":"uuid-1","pixCode":"FALSO-0002012"}`
	})

	r, err := clienteContra(t, srv.URL).CriarCobranca(context.Background(), "rmt-abc", 4990, "Espada")
	if err != nil {
		t.Fatalf("criando: %v", err)
	}

	if r.Estado != "criada" || r.Identifier != "uuid-1" || r.PixCode != "FALSO-0002012" {
		t.Errorf("resposta = %+v", r)
	}
	const quero = `/cobranca {"referencia":"rmt-abc","valorCentavos":4990,"descricao":"Espada"}`
	if visto != quero {
		t.Errorf("mandou %s;\nquero    %s", visto, quero)
	}
}

// A CONSULTA DISTINGUE "NÃO PAGO" DE "PAGO NA HORA ZERO".
//
// pagoEm nulo quer dizer "a processadora não disse" — a consulta caiu para a V1,
// que não documenta o campo — e é isso que faz o servidor cair para o próprio
// relógio. Se isso virasse um time.Time zero, ele diria "ano 1", compararia como
// antiquíssimo, e TODO pagamento passaria por dentro do prazo.
func TestConsultaDistingueNaoPagoDePagoNaHoraZero(t *testing.T) {
	t.Run("v2 com a hora", func(t *testing.T) {
		srv := pontefalsa(t, func(string, []byte) (int, string) {
			return 200, `{"estado":"achada","fonte":"v2","status":"completed",
				"valorCentavos":4990,"referencia":"rmt-abc",
				"pagoEm":"2026-09-23T14:32:10Z","devolvidoEm":null}`
		})
		r, err := clienteContra(t, srv.URL).ConsultarCobranca(context.Background(), "uuid-1")
		if err != nil {
			t.Fatal(err)
		}
		if r.PagoEm == nil {
			t.Fatal("a hora veio nula com a v2, que a documenta")
		}
		if got := r.PagoEm.UTC().Format(time.RFC3339); got != "2026-09-23T14:32:10Z" {
			t.Errorf("hora = %s", got)
		}
		if r.Fonte != "v2" {
			t.Errorf("fonte = %q; ela tem de chegar, porque decide de que relogio foi", r.Fonte)
		}
	})

	t.Run("v1 sem a hora", func(t *testing.T) {
		srv := pontefalsa(t, func(string, []byte) (int, string) {
			return 200, `{"estado":"achada","fonte":"v1","status":"completed",
				"valorCentavos":4990,"referencia":"rmt-abc","pagoEm":null}`
		})
		r, err := clienteContra(t, srv.URL).ConsultarCobranca(context.Background(), "uuid-1")
		if err != nil {
			t.Fatal(err)
		}
		if r.PagoEm != nil {
			t.Errorf("hora = %v; nula significa que a processadora nao disse, e e isso "+
				"que faz o servidor usar o relogio dele", r.PagoEm)
		}
	})
}

// O 422 NÃO É FALHA: é recusa PREVISTA, e o estado dela vem no corpo.
func TestRecusaPrevistaChegaNoCorpoENaoComoErro(t *testing.T) {
	srv := pontefalsa(t, func(string, []byte) (int, string) {
		return 422, `{"estado":"recusada","motivo":"acima do teto","httpSyncpay":422}`
	})

	r, err := clienteContra(t, srv.URL).CriarCobranca(context.Background(), "rmt-x", 999999, "x")

	if err != nil {
		t.Fatalf("o 422 virou erro: %v — ele e recusa prevista e o motivo esta no corpo", err)
	}
	if r.Estado != "recusada" || r.Motivo != "acima do teto" {
		t.Errorf("resposta = %+v", r)
	}
}

// O 500 VIRA "NÃO SEI", E NUNCA "A PROCESSADORA DISSE NÃO".
//
// É defeito da ponte. Ler como recusa transformaria um bug nosso em venda negada —
// e, na criação de cobrança, em item solto com uma cobrança possivelmente viva do
// outro lado.
func TestDefeitoDaPonteViraIncertaENaoRecusa(t *testing.T) {
	srv := pontefalsa(t, func(string, []byte) (int, string) {
		return 500, `{"erro":"deu ruim"}`
	})

	_, err := clienteContra(t, srv.URL).CriarCobranca(context.Background(), "rmt-x", 100, "x")

	if !errors.Is(err, ErrIncerta) {
		t.Errorf("erro = %v, quero ErrIncerta: 500 e defeito nosso, nao recusa deles", err)
	}
}

// Erro NOSSO (400, 401, 413) não é incerto: a chamada foi recusada na porta e nada
// aconteceu do outro lado. Confundir com incerta faria o servidor segurar um item
// por uma cobrança que nunca existiu.
func TestErroNossoNaoEIncerto(t *testing.T) {
	for _, codigo := range []int{http.StatusBadRequest, http.StatusUnauthorized,
		http.StatusRequestEntityTooLarge} {
		srv := pontefalsa(t, func(string, []byte) (int, string) { return codigo, `{}` })
		_, err := clienteContra(t, srv.URL).CriarCobranca(context.Background(), "r", 1, "x")
		if err == nil {
			t.Errorf("http %d passou como sucesso", codigo)
			continue
		}
		if errors.Is(err, ErrIncerta) {
			t.Errorf("http %d virou incerta; ele e recusa na porta, e nada aconteceu "+
				"do outro lado", codigo)
		}
	}
}

// A rede caindo é INCERTA, não erro comum: a chamada pode ter saído.
func TestRedeCaindoEIncerta(t *testing.T) {
	srv := pontefalsa(t, func(string, []byte) (int, string) { return 200, `{}` })
	url := srv.URL
	srv.Close() // ninguém atende

	_, err := clienteContra(t, url).CriarCobranca(context.Background(), "r", 1, "x")

	if !errors.Is(err, ErrIncerta) {
		t.Errorf("erro = %v, quero ErrIncerta", err)
	}
}

// SEM AS QUATRO VARIÁVEIS O CLIENTE NÃO SOBE, e o erro DIZ QUAIS faltam.
//
// Dizer quais é o que transforma um boot quebrado em uma tarefa de dois minutos.
// E são NOMES, nunca valores: um log que imprime metade de um segredo para ajudar
// a depurar é um segredo publicado.
func TestSemConfiguracaoNaoSobeEDizOQueFalta(t *testing.T) {
	_, err := Novo(Config{URL: "https://ponte", Segredo: "s"})

	if !errors.Is(err, ErrDesligada) {
		t.Fatalf("erro = %v, quero ErrDesligada", err)
	}
	msg := err.Error()
	for _, nome := range []string{"PONTE_CERT_CLIENTE", "PONTE_CHAVE_CLIENTE"} {
		if !contem(msg, nome) {
			t.Errorf("o erro %q nao diz que falta %s", msg, nome)
		}
	}
	for _, naoQuero := range []string{"PONTE_URL", "PONTE_SEGREDO"} {
		if contem(msg, naoQuero) {
			t.Errorf("o erro %q reclama de %s, que foi preenchida", msg, naoQuero)
		}
	}
}

// E um certificado que não carrega também não sobe, em vez de subir e falhar em
// toda chamada com um erro de TLS que ninguém liga à configuração.
func TestCertificadoInvalidoNaoSobe(t *testing.T) {
	_, err := Novo(Config{URL: "https://ponte", Segredo: "s",
		CertPEM: "isto nao e um certificado", ChavePEM: "nem isto"})

	if err == nil {
		t.Fatal("subiu com um certificado que nao carrega")
	}
	if errors.Is(err, ErrDesligada) {
		t.Error("confundiu certificado invalido com configuracao ausente; sao problemas " +
			"diferentes e o conserto de cada um e outro")
	}
}

func contem(todo, pedaco string) bool {
	for i := 0; i+len(pedaco) <= len(todo); i++ {
		if todo[i:i+len(pedaco)] == pedaco {
			return true
		}
	}
	return false
}

// O PEM CHEGA DOS DOIS JEITOS, e os dois têm de carregar.
//
// Colado no painel da Railway, o bloco pode vir com quebras de linha de verdade ou
// com a sequência de duas letras barra-n. Aceitar só um faria o certificado "não
// carregar" por um motivo invisível — o valor está certo, a formatação é que não —
// e o erro de TLS que sairia disso não aponta para lugar nenhum.
func TestOPemChegaDosDoisJeitos(t *testing.T) {
	comQuebras := "-----BEGIN X-----\nlinha\n-----END X-----"
	comBarraN := "-----BEGIN X-----\nlinha\n-----END X-----"

	if got := string(pemDeVariavel(comQuebras)); got != comQuebras {
		t.Errorf("o PEM com quebras de verdade foi alterado:\n%q", got)
	}
	if got := string(pemDeVariavel(comBarraN)); got != comQuebras {
		t.Errorf("o PEM com barra-n nao virou quebras:\n%q", got)
	}
}
