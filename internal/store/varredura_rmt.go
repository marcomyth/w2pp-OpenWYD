package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// O QUE AS VARREDURAS DO WEBSERVER PRECISAM LER.
//
// Elas moram no webserver e não no dbserver por um motivo só: é o webserver que tem
// as credenciais da ponte. Perguntar à processadora antes de vencer uma cobrança é
// uma chamada de rede autenticada, e o dbserver não fala com ela.

// CobrancaParaConferir é uma cobrança aberta que já tem código Pix — quer dizer,
// uma que alguém PODE ter pago.
type CobrancaParaConferir struct {
	ID         int64
	Identifier string
	ExpiraEm   time.Time
}

// CobrancasParaConferir lista as cobranças abertas que têm identifier.
//
// SÓ AS COM IDENTIFIER, e essa é a linha divisória de todo este desenho. O
// identifier nasce junto com o código Pix; sem ele não existe código, e sem código
// ninguém conseguiu pagar. Uma cobrança sem identifier pode vencer sem se perguntar
// nada a ninguém, porque não há pagamento possível para descobrir.
//
// As com identifier são o contrário: cada uma delas é um pagamento que pode estar
// entrando neste instante.
//
// ORDENADAS PELO PRAZO, as mais perto de vencer primeiro. Quando o limite corta a
// lista, o que fica de fora é o que ainda tem tempo — e o que tinha pressa foi
// conferido.
func (s *Store) CobrancasParaConferir(ctx context.Context, limite int) ([]CobrancaParaConferir, error) {
	if limite <= 0 {
		limite = 100
	}
	rows, err := s.pool.Query(ctx, `
		SELECT id, identifier_syncpay, expira_em
		  FROM rmt_cobranca
		 WHERE status = $1 AND identifier_syncpay IS NOT NULL
		 ORDER BY expira_em
		 LIMIT $2`, cobrancaAberta, limite)
	if err != nil {
		return nil, fmt.Errorf("store: cobrancas para conferir: %w", err)
	}
	defer rows.Close()
	var lista []CobrancaParaConferir
	for rows.Next() {
		var c CobrancaParaConferir
		if err := rows.Scan(&c.ID, &c.Identifier, &c.ExpiraEm); err != nil {
			return nil, fmt.Errorf("store: cobrancas para conferir: %w", err)
		}
		lista = append(lista, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: cobrancas para conferir: %w", err)
	}
	return lista, nil
}

// VencerCobrancaConferida fecha por prazo UMA cobrança que acabou de ser conferida.
//
// Existe separada da varredura do dbserver porque a ordem é o conteúdo da regra:
// pergunta-se à processadora ANTES de vencer, e só vence quem a resposta disse que
// não foi pago. A varredura de lá não pode fazer isso — ela não fala com a ponte.
//
// AS MESMAS GUARDAS DA VARREDURA, e elas não são repetição por desconfiança: entre a
// consulta e este UPDATE cabe um pagamento chegando pelo webhook. Se a cobrança
// deixou de estar aberta, ou ganhou uma divergência de valor nesse intervalo, o
// UPDATE não encontra a linha e nada acontece — que é exatamente o certo.
//
// Devolve o anúncio afetado quando venceu, e zero quando não havia o que vencer.
func (s *Store) VencerCobrancaConferida(ctx context.Context, cobrancaID int64) (int64, error) {
	var anuncio int64
	err := s.pool.QueryRow(ctx, `
		UPDATE rmt_cobranca SET status = $2, encerrada_em = now()
		 WHERE id = $1 AND status = $3 AND expira_em <= now()
		   AND valor_divergente_centavos IS NULL
		RETURNING anuncio_id`, cobrancaID, cobrancaExpirada, cobrancaAberta).Scan(&anuncio)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, nil
		}
		return 0, fmt.Errorf("store: vencer cobranca conferida %d: %w", cobrancaID, err)
	}
	return anuncio, nil
}

// JanelaDaCobrancaMorta é por quanto tempo depois do prazo uma cobrança vencida
// continua sendo consultada.
//
// QUARENTA E OITO HORAS É CHUTE, e fica escrito que é: ninguém mediu quanto tempo um
// código Pix da SyncPay continua pagável depois de vencido. Sabe-se que ele NÃO dá
// para cancelar, então a janela real é a vida do código, e ela é o que o primeiro
// teste com dinheiro de verdade tem de medir. Até lá, dois dias é generoso o
// bastante para cobrir a pessoa que pagou e foi dormir.
const JanelaDaCobrancaMorta = 48 * time.Hour

// CobrancasMortasParaConferir lista as cobranças VENCIDAS que ainda podem receber
// dinheiro.
//
// O BURACO QUE ELA TAPA: depois que a cobrança vence, o código Pix continua pagável
// — não há como cancelá-lo na processadora. Se o aviso do site se perder, ninguém
// mais pergunta nada sobre aquele pagamento, porque a conferência das abertas já não
// enxerga essa linha. O dinheiro entra na conta da Hanna sem virar linha nenhuma: nem
// pagamento sem item, nem órfão. Some.
//
// Continuar perguntando por um tempo é o que transforma esse silêncio em
// PAGA_SEM_ITEM e, daí, em devolução — que é o caminho que já existe.
//
// AS MAIS RECENTES PRIMEIRO, ao contrário da fila das abertas. Aqui não há prazo
// correndo contra ninguém; o que importa é que a que acabou de vencer, que é a que
// tem mais chance de receber um pagamento atrasado, seja perguntada antes de a lista
// ser cortada pelo limite.
func (s *Store) CobrancasMortasParaConferir(ctx context.Context, janela time.Duration,
	limite int,
) ([]CobrancaParaConferir, error) {
	if janela <= 0 {
		janela = JanelaDaCobrancaMorta
	}
	if limite <= 0 {
		limite = 100
	}
	rows, err := s.pool.Query(ctx, `
		SELECT id, identifier_syncpay, expira_em
		  FROM rmt_cobranca
		 WHERE status = $1 AND identifier_syncpay IS NOT NULL
		   AND expira_em > now() - $2::interval
		 ORDER BY expira_em DESC
		 LIMIT $3`, cobrancaExpirada, janela, limite)
	if err != nil {
		return nil, fmt.Errorf("store: cobrancas mortas para conferir: %w", err)
	}
	defer rows.Close()
	var lista []CobrancaParaConferir
	for rows.Next() {
		var c CobrancaParaConferir
		if err := rows.Scan(&c.ID, &c.Identifier, &c.ExpiraEm); err != nil {
			return nil, fmt.Errorf("store: cobrancas mortas para conferir: %w", err)
		}
		lista = append(lista, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: cobrancas mortas para conferir: %w", err)
	}
	return lista, nil
}

// ReembolsoParaPedir é uma devolução que devemos e ainda não pedimos.
type ReembolsoParaPedir struct {
	CobrancaID    int64
	Identifier    string
	Referencia    string
	ValorCentavos int64
}

// ReembolsosParaPedir lista o que está em PENDENTE — devemos e ninguém pediu ainda.
//
// PENDENTE E MAIS NADA. Recusado espera uma pessoa, incerto pode já ter sido criado,
// e pedido está em análise: pedir de novo em qualquer um dos três criaria um segundo
// pedido sobre o mesmo dinheiro, e o comprador receberia o dobro do que pagou.
//
// SEM IDENTIFIER NÃO ENTRA na lista, porque não dá para pedir: a processadora acha o
// pagamento pelo id dela. Essas linhas ficariam invisíveis, então quem chama também
// pergunta quantas são — ver ReembolsosSemIdentifier.
func (s *Store) ReembolsosParaPedir(ctx context.Context, limite int) ([]ReembolsoParaPedir, error) {
	if limite <= 0 {
		limite = 50
	}
	rows, err := s.pool.Query(ctx, `
		SELECT id, identifier_syncpay, referencia_externa, valor_centavos
		  FROM rmt_cobranca
		 WHERE reembolso_status = $1 AND identifier_syncpay IS NOT NULL
		 ORDER BY id
		 LIMIT $2`, reembolsoPendente, limite)
	if err != nil {
		return nil, fmt.Errorf("store: reembolsos para pedir: %w", err)
	}
	defer rows.Close()
	var lista []ReembolsoParaPedir
	for rows.Next() {
		var r ReembolsoParaPedir
		if err := rows.Scan(&r.CobrancaID, &r.Identifier, &r.Referencia, &r.ValorCentavos); err != nil {
			return nil, fmt.Errorf("store: reembolsos para pedir: %w", err)
		}
		lista = append(lista, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: reembolsos para pedir: %w", err)
	}
	return lista, nil
}

// ReembolsosSemIdentifier conta as devoluções que a varredura NÃO consegue pedir.
//
// Existe pela lição que o repasse já custou uma vez: uma fila que exclui linhas por
// falta de um campo faz essas linhas sumirem de todo contador, e o log passa a dizer
// "nada a fazer" enquanto o dinheiro de alguém está parado. O número tem de aparecer
// em algum lugar, mesmo que ninguém saiba ainda como consertá-lo.
func (s *Store) ReembolsosSemIdentifier(ctx context.Context) (int, error) {
	var n int
	err := s.pool.QueryRow(ctx, `
		SELECT count(*) FROM rmt_cobranca
		 WHERE reembolso_status = $1 AND identifier_syncpay IS NULL`, reembolsoPendente).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("store: reembolsos sem identifier: %w", err)
	}
	return n, nil
}

// CobrancasVencidasSemConferir conta as que passaram do prazo, têm identifier, e
// continuam abertas.
//
// É O ALARME DE QUE O WEBSERVER NÃO ESTÁ VARRENDO. Com ele no ar o número fica em
// zero ou perto disso, porque a varredura confere e vence em segundos. Crescendo, ele
// diz que as cobranças pararam de ser conferidas — e o efeito visível é o item do
// vendedor continuar preso no baú depois do prazo.
//
// A ESCOLHA É DELIBERADA E É A TROCA QUE ELA REPRESENTA: item preso se conserta, e
// dinheiro entregue a quem não devia não. Por isso o dbserver deixou de vencer essas
// linhas às cegas em vez de continuar vencendo e arriscar soltar o item de quem pagou
// no último segundo.
func (s *Store) CobrancasVencidasSemConferir(ctx context.Context) (int, error) {
	var n int
	err := s.pool.QueryRow(ctx, `
		SELECT count(*) FROM rmt_cobranca
		 WHERE status = $1 AND identifier_syncpay IS NOT NULL AND expira_em <= now()`,
		cobrancaAberta).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("store: cobrancas vencidas sem conferir: %w", err)
	}
	return n, nil
}
