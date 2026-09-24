// Package rmtrepasse paga o vendedor.
//
// É a última ponta do mercado em dinheiro real, e a única em que o servidor MANDA
// dinheiro embora em vez de receber. Isso muda o que "errar" significa: nas outras
// pontas um erro deixa alguém esperando, e aqui ele pode pagar duas vezes — o que não
// se desfaz e sai do bolso de quem administra.
//
// TRÊS REGRAS, e todas existem por causa disso:
//
//  1. A TRAVA É NOSSA. A ponte é idempotente pela referência, e a nossa referência é
//     por TENTATIVA — então ela não sabe que duas tentativas são a mesma dívida, e
//     pagaria as duas. Quem impede é o estado PENDENTE, conferido com a linha travada.
//  2. O INCERTO NUNCA É REENVIADO. Quando a chamada sai e a resposta não volta, o
//     pagamento PODE ter acontecido, e não há consulta de saque para desempatar. Só uma
//     pessoa que foi ao painel resolve.
//  3. A REFERÊNCIA É GRAVADA ANTES DA CHAMADA. Se a resposta se perder, é por ela que
//     se descobre o que foi mandado.
package rmtrepasse

import (
	"context"
	"errors"
	"log/slog"

	"github.com/jeanluca/w2pp-openwyd/internal/store"
	"github.com/jeanluca/w2pp-openwyd/webserver/internal/ponte"
)

// Ponte é o pedaço do cliente que este pacote usa.
type Ponte interface {
	Repassar(ctx context.Context, referencia string, centavos int64,
		chavePix, tipoChave, documento string) (ponte.RespostaRepasse, error)
}

// Banco é o que o pagamento precisa do banco.
type Banco interface {
	RepassesAPagar(ctx context.Context, limite int) ([]store.RepasseAPagar, error)
	// RepassesEsperandoCadastro conta quem tem dinheiro a receber e ainda não cadastrou
	// chave ou CPF. Não entra na fila de pagar, e por isso precisa de contador próprio.
	RepassesEsperandoCadastro(ctx context.Context) (int, error)
	AbrirTentativa(ctx context.Context, repasseID int64, liberadoPor string) (store.TentativaDeRepasse, error)
	FecharTentativa(ctx context.Context, tentativaID int64, resultado store.EstadoRepasse,
		identifierSaque string, httpSyncpay *int32, codigo, texto string) error
	MarcarRepasseEnviado(ctx context.Context, id int64, identifierSaque string, enviadoCentavos int64) error
	MarcarRepasseIncerto(ctx context.Context, id int64, motivo string) error
	MarcarRepasseRecusado(ctx context.Context, id int64, httpSyncpay *int32, codigo, texto string) error
}

// Servico paga as dívidas pendentes.
type Servico struct {
	ponte Ponte
	banco Banco
	log   *slog.Logger
}

// Novo monta o serviço.
func Novo(p Ponte, b Banco, log *slog.Logger) *Servico {
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	return &Servico{ponte: p, banco: b, log: log}
}

// Rodada é o que uma varredura produziu.
//
// QUATRO NÚMEROS E NÃO UM, e a razão é o que alguém faz com eles. O primeiro impulso foi
// contar só os pagos; mas aí uma rodada com três incertos e nenhum pago sairia como
// "nada aconteceu" — que é a pior frase possível para três pagamentos que talvez tenham
// saído. Sumir da contagem é o mesmo silêncio que o número errado, com outra cara.
type Rodada struct {
	// Aceitos é o que a ponte aceitou, e só isso. Não é "pago": o dinheiro só chega
	// quando o aviso de saque confirmar.
	Aceitos int
	// Recusados tem CERTEZA de que nada saiu. Param na fila da staff.
	Recusados int
	// Incertos PODEM ter pago, e ninguém sabe. É o número que precisa gritar.
	Incertos int
	// Falhas são os erros desta varredura, e não estados da dívida.
	Falhas int
	// EsperandoCadastro é quanta gente tem dinheiro a receber e ainda não disse para
	// onde mandar.
	//
	// NÃO É FALHA e não é erro de ninguém, e mesmo assim precisa aparecer: essas linhas
	// não entram na fila de pagar, então antes deste contador uma varredura com dez
	// delas dizia "nada a fazer". A pessoa esperava o dinheiro dela e o log dizia que
	// estava tudo certo.
	EsperandoCadastro int
}

// PagarPendentes tenta pagar as dívidas que estão esperando.
//
// UMA POR VEZ, e sem paralelismo: o volume é de poucas por dia, e o ganho de paralelizar
// não paga o risco. Cada chamada aqui move dinheiro de verdade.
//
// O ERRO DE UMA NÃO PARA AS OUTRAS. Uma chave inválida de um vendedor não pode segurar o
// pagamento de quem está atrás dele na fila — mas o erro sai no log, com o id, porque
// uma fila que engole falhas é uma fila que ninguém percebe que parou.
func (s *Servico) PagarPendentes(ctx context.Context, limite int) Rodada {
	var out Rodada
	fila, err := s.banco.RepassesAPagar(ctx, limite)
	if err != nil {
		s.log.Error("repasse: nao consegui ler a fila", "err", err)
		return out
	}
	if n, err := s.banco.RepassesEsperandoCadastro(ctx); err != nil {
		s.log.Error("repasse: nao consegui contar quem espera cadastro", "err", err)
	} else {
		out.EsperandoCadastro = n
	}
	for _, r := range fila {
		estado, err := s.pagarUm(ctx, r)
		if err != nil {
			out.Falhas++
			s.log.Error("repasse: falhou", "repasse", r.ID,
				"vendedor", r.VendedorConta, "err", err)
			continue
		}
		switch estado {
		case store.RepasseEnviado:
			out.Aceitos++
		case store.RepasseRecusado:
			out.Recusados++
		case store.RepasseIncerto:
			out.Incertos++
		}
	}
	return out
}

// Registrar escreve o resultado da rodada, e escolhe o NÍVEL pelo que ele significa.
//
// O incerto sai em WARN mesmo quando tudo o mais correu bem, e é essa a diferença que
// importa: cada incerto é um vendedor que talvez já tenha o dinheiro e talvez não, e só
// uma pessoa resolve. Um número desses numa linha de INFO, no meio de outras, é um
// número que ninguém vai ver.
//
// E a rodada VAZIA não escreve nada. A varredura roda a cada dois minutos: uma linha por
// rodada encheria o log de "não fiz nada" e afogaria as que dizem alguma coisa.
func (r Rodada) Registrar(log *slog.Logger) {
	if r == (Rodada{}) {
		return
	}
	args := []any{
		"aceitos", r.Aceitos, "recusados", r.Recusados,
		"incertos", r.Incertos, "falhas", r.Falhas,
		"esperando_cadastro", r.EsperandoCadastro,
	}
	if r.Incertos > 0 {
		log.Warn("repasse: rodada COM INCERTOS; alguem precisa olhar o painel", args...)
		return
	}
	log.Info("repasse: rodada", args...)
}

// pagarUm faz uma tentativa e grava o que voltou.
//
// Devolve o estado em que a dívida ficou, e erro nulo em todos eles: o incerto e a
// recusa foram TRATADOS, e não são falha desta varredura. O que muda entre eles é o que
// o log tem de dizer, e quem precisa agir.
func (s *Servico) pagarUm(ctx context.Context, r store.RepasseAPagar) (store.EstadoRepasse, error) {
	tipo := ponte.TipoDeChaveNaPonte(int16(r.TipoChave))
	if tipo == "" {
		// RECUSA ANTES DE MANDAR. Um tipo vazio no corpo é 400 na ponte, e o 400 não
		// diz qual era o tipo — aqui dá para dizer. E isto não é erro do vendedor: é
		// um tipo de chave que o nosso mapa não conhece, ou seja, defeito nosso.
		s.log.Error("repasse: tipo de chave que eu nao sei traduzir",
			"repasse", r.ID, "tipo", int16(r.TipoChave))
		http := int32(0)
		return store.RepasseRecusado, s.banco.MarcarRepasseRecusado(ctx, r.ID, &http, "TIPO_DESCONHECIDO",
			"o servidor nao sabe traduzir este tipo de chave")
	}

	// A TENTATIVA NASCE ANTES DA CHAMADA, com a referência gravada. Se a resposta se
	// perder, é esta linha que diz o que foi mandado — e ela é também a trava: só nasce
	// se a dívida estiver PENDENTE.
	t, err := s.banco.AbrirTentativa(ctx, r.ID, "")
	if err != nil {
		return 0, err
	}

	resp, err := s.ponte.Repassar(ctx, t.Referencia, r.ValorCentavos,
		r.ChavePix, tipo, r.Documento)

	// O INCERTO VEM COMO ERRO, e é o caso que não pode ser confundido com os outros.
	//
	// A chamada saiu e a resposta não voltou: o pagamento PODE ter acontecido. A linha
	// sai de pendente para nunca mais ser reenviada sozinha, e vai para a fila de
	// gente. Tratar isto como "falhou, tenta de novo" é como se paga duas vezes.
	if errors.Is(err, ponte.ErrIncerta) {
		s.log.Error("repasse: INCERTO, pode ter pago; ninguem reenvia isto",
			"repasse", r.ID, "vendedor", r.VendedorConta, "referencia", t.Referencia, "err", err)
		if e := s.banco.FecharTentativa(ctx, t.ID, store.RepasseIncerto, "", nil, "", err.Error()); e != nil {
			return 0, e
		}
		return store.RepasseIncerto, s.banco.MarcarRepasseIncerto(ctx, r.ID, err.Error())
	}
	if err != nil {
		// Erro de transporte ou recusa da porta (400, 401, 413): nada saiu, e a
		// referência desta tentativa continua queimada do lado da ponte. A dívida fica
		// PENDENTE e a próxima varredura abre a tentativa seguinte, com referência
		// nova — que é seguro porque nada saiu.
		if e := s.banco.FecharTentativa(ctx, t.ID, store.RepasseRecusado, "", nil, "", err.Error()); e != nil {
			return 0, e
		}
		return 0, err
	}

	switch resp.Estado {
	case "aceito", "repetido":
		// REPETIDO É SUCESSO e não anomalia: quer dizer que esta MESMA tentativa já
		// tinha sido mandada e a ponte devolveu o id da primeira vez, sem chamar a
		// processadora de novo. É a idempotência funcionando depois de a nossa chamada
		// anterior ter se perdido.
		if err := s.banco.FecharTentativa(ctx, t.ID, store.RepasseEnviado,
			resp.ChaveGateway, nil, "", ""); err != nil {
			return 0, err
		}
		s.log.Info("repasse: aceito pela ponte", "repasse", r.ID,
			"vendedor", r.VendedorConta, "saque", resp.ChaveGateway, "estado", resp.Estado)
		return store.RepasseEnviado, s.banco.MarcarRepasseEnviado(ctx, r.ID, resp.ChaveGateway, r.ValorCentavos)

	case "recusado":
		// CERTEZA de que nada saiu. É o único resultado em que tentar de novo é seguro,
		// e ele para na fila da staff porque alguém tem de olhar o motivo.
		//
		// O httpSyncpay nulo diz que quem recusou foi a PRÓPRIA PONTE — teto, ou a
		// trava do saque desligada — e isso não é culpa do vendedor. Enquanto a trava
		// estiver fechada, TODA linha cai aqui assim.
		s.log.Warn("repasse: recusado", "repasse", r.ID, "vendedor", r.VendedorConta,
			"http_syncpay", resp.HTTPSyncpay, "codigo", resp.CodigoSyncpay, "motivo", resp.Motivo)
		if err := s.banco.FecharTentativa(ctx, t.ID, store.RepasseRecusado, "",
			resp.HTTPSyncpay, resp.CodigoSyncpay, resp.Motivo); err != nil {
			return 0, err
		}
		return store.RepasseRecusado, s.banco.MarcarRepasseRecusado(ctx, r.ID, resp.HTTPSyncpay,
			resp.CodigoSyncpay, resp.Motivo)

	default:
		// Estado que este código não conhece. TRATADO COMO INCERTO, e não como recusa,
		// e a escolha é deliberada: uma recusa libera a dívida para uma tentativa nova,
		// e mandar de novo o que talvez tenha sido pago é o erro que não se desfaz.
		//
		// O caro aqui é o certo: uma linha parada esperando gente custa tempo; uma
		// linha paga duas vezes custa dinheiro.
		s.log.Error("repasse: estado que eu nao conheco; tratando como incerto",
			"repasse", r.ID, "estado", resp.Estado)
		if err := s.banco.FecharTentativa(ctx, t.ID, store.RepasseIncerto, resp.ChaveGateway,
			resp.HTTPSyncpay, resp.CodigoSyncpay, "estado desconhecido: "+resp.Estado); err != nil {
			return 0, err
		}
		return store.RepasseIncerto, s.banco.MarcarRepasseIncerto(ctx, r.ID, "estado desconhecido: "+resp.Estado)
	}
}
