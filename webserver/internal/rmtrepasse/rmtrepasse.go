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

// PagarPendentes tenta pagar as dívidas que estão esperando.
//
// `pagos` conta só o que a ponte ACEITOU. O incerto e a recusa não entram em nenhum dos
// dois números: eles foram tratados, saíram da fila, e não são falha desta varredura —
// mas também não são pagamento, e contá-los faria o log dizer que a gente pagou alguém
// que talvez não tenha recebido. Quem quer saber deles olha a fila da staff.
//
// UMA POR VEZ, e sem paralelismo: o volume é de poucas por dia, e o ganho de paralelizar
// não paga o risco. Cada chamada aqui move dinheiro de verdade.
//
// O ERRO DE UMA NÃO PARA AS OUTRAS. Uma chave inválida de um vendedor não pode segurar o
// pagamento de quem está atrás dele na fila — mas o erro sai no log, com o id, porque
// uma fila que engole falhas é uma fila que ninguém percebe que parou.
func (s *Servico) PagarPendentes(ctx context.Context, limite int) (pagos, falhas int) {
	fila, err := s.banco.RepassesAPagar(ctx, limite)
	if err != nil {
		s.log.Error("repasse: nao consegui ler a fila", "err", err)
		return 0, 0
	}
	for _, r := range fila {
		enviou, err := s.pagarUm(ctx, r)
		if err != nil {
			falhas++
			s.log.Error("repasse: falhou", "repasse", r.ID,
				"vendedor", r.VendedorConta, "err", err)
			continue
		}
		if enviou {
			pagos++
		}
	}
	return pagos, falhas
}

// pagarUm faz uma tentativa e grava o que voltou.
//
// Devolve enviou=true SÓ quando a ponte aceitou. O incerto e a recusa voltam com
// enviou=false e erro nulo: eles foram tratados e não são falha, mas também não são
// pagamento — e a diferença entre as duas coisas é o que o log precisa dizer.
func (s *Servico) pagarUm(ctx context.Context, r store.RepasseAPagar) (enviou bool, err error) {
	tipo := ponte.TipoDeChaveNaPonte(int16(r.TipoChave))
	if tipo == "" {
		// RECUSA ANTES DE MANDAR. Um tipo vazio no corpo é 400 na ponte, e o 400 não
		// diz qual era o tipo — aqui dá para dizer. E isto não é erro do vendedor: é
		// um tipo de chave que o nosso mapa não conhece, ou seja, defeito nosso.
		s.log.Error("repasse: tipo de chave que eu nao sei traduzir",
			"repasse", r.ID, "tipo", int16(r.TipoChave))
		http := int32(0)
		return false, s.banco.MarcarRepasseRecusado(ctx, r.ID, &http, "TIPO_DESCONHECIDO",
			"o servidor nao sabe traduzir este tipo de chave")
	}

	// A TENTATIVA NASCE ANTES DA CHAMADA, com a referência gravada. Se a resposta se
	// perder, é esta linha que diz o que foi mandado — e ela é também a trava: só nasce
	// se a dívida estiver PENDENTE.
	t, err := s.banco.AbrirTentativa(ctx, r.ID, "")
	if err != nil {
		return false, err
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
			return false, e
		}
		return false, s.banco.MarcarRepasseIncerto(ctx, r.ID, err.Error())
	}
	if err != nil {
		// Erro de transporte ou recusa da porta (400, 401, 413): nada saiu, e a
		// referência desta tentativa continua queimada do lado da ponte. A dívida fica
		// PENDENTE e a próxima varredura abre a tentativa seguinte, com referência
		// nova — que é seguro porque nada saiu.
		if e := s.banco.FecharTentativa(ctx, t.ID, store.RepasseRecusado, "", nil, "", err.Error()); e != nil {
			return false, e
		}
		return false, err
	}

	switch resp.Estado {
	case "aceito", "repetido":
		// REPETIDO É SUCESSO e não anomalia: quer dizer que esta MESMA tentativa já
		// tinha sido mandada e a ponte devolveu o id da primeira vez, sem chamar a
		// processadora de novo. É a idempotência funcionando depois de a nossa chamada
		// anterior ter se perdido.
		if err := s.banco.FecharTentativa(ctx, t.ID, store.RepasseEnviado,
			resp.ChaveGateway, nil, "", ""); err != nil {
			return false, err
		}
		s.log.Info("repasse: aceito pela ponte", "repasse", r.ID,
			"vendedor", r.VendedorConta, "saque", resp.ChaveGateway, "estado", resp.Estado)
		return true, s.banco.MarcarRepasseEnviado(ctx, r.ID, resp.ChaveGateway, r.ValorCentavos)

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
			return false, err
		}
		return false, s.banco.MarcarRepasseRecusado(ctx, r.ID, resp.HTTPSyncpay,
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
			return false, err
		}
		return false, s.banco.MarcarRepasseIncerto(ctx, r.ID, "estado desconhecido: "+resp.Estado)
	}
}
