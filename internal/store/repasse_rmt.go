package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// Estados do repasse (0124). Os números são os da coluna.
const (
	repassePendente int16 = 1
	repasseEnviado  int16 = 2
	repassePago     int16 = 3
	repasseRecusado int16 = 4
	repasseIncerto  int16 = 5
)

// EstadoRepasse é o que aconteceu com a dívida com o vendedor.
type EstadoRepasse int16

const (
	// RepassePendente: ninguém tentou pagar ainda.
	RepassePendente EstadoRepasse = EstadoRepasse(repassePendente)
	// RepasseEnviado: a ponte ACEITOU e devolveu o id do saque. Vira pago quando o
	// aviso de saque chegar.
	RepasseEnviado EstadoRepasse = EstadoRepasse(repasseEnviado)
	// RepassePago: o dinheiro chegou na conta do vendedor.
	RepassePago EstadoRepasse = EstadoRepasse(repassePago)
	// RepasseRecusado: nada saiu, com certeza. É o único estado de falha em que
	// tentar de novo é seguro.
	RepasseRecusado EstadoRepasse = EstadoRepasse(repasseRecusado)
	// RepasseEsperandoCadastro NÃO é um status do banco: é o que a fila devolve para
	// uma dívida PENDENTE cujo vendedor não tem chave ou não tem documento.
	//
	// Existe porque essas linhas eram INVISÍVEIS: o JOIN da fila de pagar as excluía, e
	// elas não apareciam em contador nenhum — nem pago, nem recusado, nem incerto. Uma
	// pessoa com dinheiro a receber ficava esperando para sempre, e o log dizia que
	// estava tudo certo.
	//
	// Não vira status porque não é um estado da DÍVIDA: ela está pendente e correta. O
	// que falta é do lado do vendedor, e muda no instante em que ele cadastrar.
	RepasseEsperandoCadastro EstadoRepasse = -1
	// RepasseIncerto: a chamada saiu e a resposta não voltou. PODE TER PAGO.
	//
	// NUNCA se reenvia daqui, e é por isso que ele é um estado e não um erro: pagar
	// duas vezes não se desfaz, e não existe consulta de saque para desempatar. Só uma
	// pessoa olhando o painel da processadora resolve.
	RepasseIncerto EstadoRepasse = EstadoRepasse(repasseIncerto)
)

// ErrRepasseInexistente é mexer num repasse que não existe.
var ErrRepasseInexistente = errors.New("store: repasse inexistente")

// RepasseAPagar é uma dívida com um vendedor, do jeito que quem vai pagar precisa.
type RepasseAPagar struct {
	ID            int64
	CobrancaID    int64
	Referencia    string
	VendedorConta int64
	ValorCentavos int64
	ChavePix      string
	TipoChave     TipoChavePix
	// Documento vai INTEIRO, e é o único lugar do código que o lê assim. A rota de
	// repasse da ponte o exige, e é essa exigência que justifica a leitura — a nota da
	// 0120 diz que só o repasse e a staff podem.
	Documento string
}

// AbrirRepasse cria a dívida com o vendedor, DENTRO da transação que está marcando a
// cobrança como paga.
//
// Recebe a tx e não o pool de propósito: fora da transação abre a janela em que o item
// sai, o dinheiro entra, e a dívida não fica escrita. Uma queda ali deixaria a venda
// completa e o vendedor invisível — e ninguém saberia procurar por ele, porque não
// haveria linha nenhuma dizendo que havia dívida.
//
// IDEMPOTENTE pelo índice único sobre a cobrança: a confirmação repetida encontra a
// linha que já existe em vez de criar uma segunda dívida pela mesma venda. Vale mais
// aqui do que em qualquer outro lugar, porque pagar duas vezes não se desfaz.
func abrirRepasse(ctx context.Context, tx pgx.Tx, venda VendaRMT, valorCentavos int64) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO rmt_repasse (cobranca_id, vendedor_conta, valor_centavos, status)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (cobranca_id) DO NOTHING`,
		venda.CobrancaID, venda.VendedorConta, valorCentavos, repassePendente)
	if err != nil {
		return fmt.Errorf("store: abrindo o repasse da cobranca %d: %w", venda.CobrancaID, err)
	}
	return nil
}

// RepassesAPagar lista as dívidas que ninguém tentou pagar ainda, mais antiga primeiro.
//
// Traz a chave e o DOCUMENTO INTEIRO porque é o que a rota da ponte exige. É a única
// consulta do código que lê o documento sem máscara, e ela existe só para isso.
//
// Repasse sem chave cadastrada NÃO aparece: o JOIN o exclui. Não é esquecimento — é que
// não há para onde mandar, e uma linha sem destino na fila de pagamento faria quem
// paga tropeçar numa por uma. Essas ficam pendentes até o vendedor cadastrar, que é o
// que a tela dele pede.
func (s *Store) RepassesAPagar(ctx context.Context, limite int) ([]RepasseAPagar, error) {
	if limite <= 0 {
		limite = 50
	}
	rows, err := s.pool.Query(ctx, `
		SELECT r.id, r.cobranca_id, c.referencia_externa, r.vendedor_conta,
		       r.valor_centavos, d.chave, d.tipo, coalesce(d.documento, '')
		  FROM rmt_repasse r
		  JOIN rmt_cobranca c   ON c.id = r.cobranca_id
		  JOIN rmt_recebedor d  ON d.account_id = r.vendedor_conta
		 WHERE r.status = $1 AND d.documento IS NOT NULL
		 ORDER BY r.criado_em, r.id
		 LIMIT $2`, repassePendente, limite)
	if err != nil {
		return nil, fmt.Errorf("store: repasses a pagar: %w", err)
	}
	defer rows.Close()
	var fila []RepasseAPagar
	for rows.Next() {
		var r RepasseAPagar
		var tipo int16
		if err := rows.Scan(&r.ID, &r.CobrancaID, &r.Referencia, &r.VendedorConta,
			&r.ValorCentavos, &r.ChavePix, &tipo, &r.Documento); err != nil {
			return nil, fmt.Errorf("store: repasses a pagar: %w", err)
		}
		r.TipoChave = TipoChavePix(tipo)
		fila = append(fila, r)
	}
	return fila, rows.Err()
}

// MarcarRepasseEnviado registra que a ponte ACEITOU, com o id do saque.
//
// SÓ SAI DE PENDENTE, e a condição no WHERE é o que impede o pior caso: dois
// trabalhadores pegando a mesma linha, os dois mandando, e o vendedor recebendo duas
// vezes. Quem perder a corrida não muda nada e vê que não mudou.
func (s *Store) MarcarRepasseEnviado(ctx context.Context, id int64, identifierSaque string, enviadoCentavos int64) error {
	return s.mudaRepasse(ctx, id, `
		UPDATE rmt_repasse
		   SET status = $2, identifier_saque = $3, enviado_centavos = $4, enviado_em = now()
		 WHERE id = $1 AND status = $5`,
		id, repasseEnviado, identifierSaque, enviadoCentavos, repassePendente)
}

// MarcarRepasseIncerto é a resposta que não se sabe: a chamada saiu e não voltou.
//
// A linha sai de pendente para NUNCA MAIS ser reenviada automaticamente. Ela vai para a
// fila de gente, e é a fila mais urgente das duas — cada linha é um vendedor que talvez
// já tenha o dinheiro e talvez não.
func (s *Store) MarcarRepasseIncerto(ctx context.Context, id int64, motivo string) error {
	return s.mudaRepasse(ctx, id, `
		UPDATE rmt_repasse
		   SET status = $2, recusa_texto = $3, enviado_em = now()
		 WHERE id = $1 AND status = $4`,
		id, repasseIncerto, motivo, repassePendente)
}

// MarcarRepasseRecusado registra a recusa COM CERTEZA de que nada saiu.
//
// httpSyncpay nulo quer dizer que quem recusou foi a própria ponte — teto, ou a trava
// do saque desligada —, e isso NÃO é culpa do vendedor. Enquanto a trava estiver
// fechada, toda linha cai aqui com nulo, e tratar isso como "sua chave está errada"
// mandaria a pessoa mexer no que estava certo.
func (s *Store) MarcarRepasseRecusado(ctx context.Context, id int64, httpSyncpay *int32, codigo, texto string) error {
	return s.mudaRepasse(ctx, id, `
		UPDATE rmt_repasse
		   SET status = $2, recusa_http = $3, recusa_codigo = $4, recusa_texto = $5
		 WHERE id = $1 AND status = $6`,
		id, repasseRecusado, httpSyncpay, codigo, texto, repassePendente)
}

// MarcarRepassePago fecha a dívida, com o que de fato chegou na conta do vendedor.
//
// SÓ SAI DE ENVIADO. Um aviso de saque que chegue para uma linha pendente ou recusada
// não fecha nada: ou o aviso é de outro saque, ou a nossa linha está errada, e as duas
// hipóteses pedem gente em vez de uma escrita calada.
//
// chegouCentavos é o que a processadora diz ter depositado, e a diferença para o
// enviado é a taxa do saque. É o único jeito de saber a taxa: não há consulta de saque.
func (s *Store) MarcarRepassePago(ctx context.Context, identifierSaque string, chegouCentavos int64) error {
	tag, err := s.pool.Exec(ctx, `
		UPDATE rmt_repasse
		   SET status = $2, chegou_centavos = $3, pago_em = now()
		 WHERE identifier_saque = $1 AND status = $4`,
		identifierSaque, repassePago, chegouCentavos, repasseEnviado)
	if err != nil {
		return fmt.Errorf("store: marcando pago o saque %q: %w", identifierSaque, err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("saque %q: %w", identifierSaque, ErrRepasseInexistente)
	}
	return nil
}

// mudaRepasse aplica uma transição e reclama quando ela não pegou.
//
// Zero linhas afetadas NÃO é sucesso silencioso: quer dizer que a linha já saiu do
// estado que a transição esperava — outro trabalhador pegou, ou uma pessoa mexeu. Em
// dinheiro, seguir adiante achando que mudou é como se paga duas vezes.
func (s *Store) mudaRepasse(ctx context.Context, id int64, sql string, args ...any) error {
	tag, err := s.pool.Exec(ctx, sql, args...)
	if err != nil {
		return fmt.Errorf("store: mudando o repasse %d: %w", id, err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("repasse %d: %w", id, ErrRepasseInexistente)
	}
	return nil
}

// RepasseNaFila é uma linha que precisa de gente.
type RepasseNaFila struct {
	ID            int64
	CobrancaID    int64
	VendedorConta int64
	ValorCentavos int64
	Estado        EstadoRepasse
	RecusaHTTP    *int32
	RecusaCodigo  string
	RecusaTexto   string
	CriadoEm      time.Time
}

// RepassesQuePrecisamDeGente lista o que travou: o recusado, o incerto, e o que espera
// o vendedor cadastrar.
//
// OS TRÊS JUNTOS e não em consultas separadas, porque quem olha a fila quer saber "o que
// está parado" — e separá-los faria alguém olhar uma e esquecer as outras. O estado vem
// na linha, e o incerto é o que se olha primeiro.
//
// O TERCEIRO É O QUE ERA INVISÍVEL: uma dívida pendente cujo vendedor não tem chave ou
// não tem documento não aparecia em lugar nenhum. O JOIN da fila de pagar a excluía, e
// nenhum contador a mencionava. A pessoa ficava esperando o dinheiro dela para sempre e
// o log dizia que estava tudo certo.
func (s *Store) RepassesQuePrecisamDeGente(ctx context.Context) ([]RepasseNaFila, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT r.id, r.cobranca_id, r.vendedor_conta, r.valor_centavos, r.status,
		       r.recusa_http, coalesce(r.recusa_codigo, ''), coalesce(r.recusa_texto, ''),
		       r.criado_em
		  FROM rmt_repasse r
		 WHERE r.status IN ($1, $2) AND r.resolvido_em IS NULL
		UNION ALL
		-- As pendentes sem destino. O LEFT JOIN é o que as acha: com o JOIN normal elas
		-- desapareceriam, que é exatamente o que estava acontecendo.
		SELECT r.id, r.cobranca_id, r.vendedor_conta, r.valor_centavos, $4,
		       NULL, '', 'o vendedor ainda nao cadastrou chave pix ou CPF',
		       r.criado_em
		  FROM rmt_repasse r
		  LEFT JOIN rmt_recebedor d ON d.account_id = r.vendedor_conta
		 WHERE r.status = $3 AND (d.account_id IS NULL OR d.documento IS NULL)
		 ORDER BY 5 DESC, 9`,
		repasseRecusado, repasseIncerto, repassePendente, int16(RepasseEsperandoCadastro))
	if err != nil {
		return nil, fmt.Errorf("store: repasses parados: %w", err)
	}
	defer rows.Close()
	var fila []RepasseNaFila
	for rows.Next() {
		var r RepasseNaFila
		var status int16
		if err := rows.Scan(&r.ID, &r.CobrancaID, &r.VendedorConta, &r.ValorCentavos,
			&status, &r.RecusaHTTP, &r.RecusaCodigo, &r.RecusaTexto, &r.CriadoEm); err != nil {
			return nil, fmt.Errorf("store: repasses parados: %w", err)
		}
		r.Estado = EstadoRepasse(status)
		fila = append(fila, r)
	}
	return fila, rows.Err()
}

// AtorDoRepasse é quem, da staff, resolveu uma linha à mão.
type AtorDoRepasse struct {
	Nome string
}

// ResolverIncertoComoPago é a staff dizendo, depois de olhar o painel da processadora,
// que o dinheiro SAIU.
//
// SÓ SAI DE INCERTO, e é por isso que ela existe separada das outras transições: o
// incerto é TERMINAL PARA A MÁQUINA. Nada o reenvia — nem a varredura, nem um botão de
// "tentar de novo" —, porque reenviar um pagamento que pode ter saído é a única coisa
// deste sistema que não se desfaz. A única saída é uma pessoa que foi olhar.
//
// Quem resolveu fica gravado. Numa disputa, a pergunta é "quem disse que pagou, e
// quando", e um estado que muda sem dono não responde.
func (s *Store) ResolverIncertoComoPago(ctx context.Context, id int64, ator AtorDoRepasse, chegouCentavos int64) error {
	return s.mudaRepasse(ctx, id, `
		UPDATE rmt_repasse
		   SET status = $2, chegou_centavos = $3, pago_em = now(),
		       resolvido_em = now(), resolvido_por = $4
		 WHERE id = $1 AND status = $5`,
		id, repassePago, chegouCentavos, ator.Nome, repasseIncerto)
}

// ResolverIncertoComoNaoPago é a staff dizendo que o dinheiro NÃO saiu, e que a dívida
// continua de pé.
//
// Volta para PENDENTE de propósito, e essa é a diferença entre esta função e a de cima:
// aqui a pessoa CONFERIU que nada saiu, então reenviar deixa de ser perigoso e vira a
// coisa certa. É a única porta de volta para a fila de pagar.
//
// A referência da ponte, porém, ficou travada para sempre naquele incerto. Quem
// reenviar terá de usar outra, e é por isso que este caminho registra o motivo: sem ele,
// o reenvio seguinte pareceria uma cobrança nova sem explicação.
func (s *Store) ResolverIncertoComoNaoPago(ctx context.Context, id int64, ator AtorDoRepasse, nota string) error {
	return s.mudaRepasse(ctx, id, `
		UPDATE rmt_repasse
		   SET status = $2, recusa_texto = $3, resolvido_em = now(), resolvido_por = $4,
		       enviado_em = NULL
		 WHERE id = $1 AND status = $5`,
		id, repassePendente, nota, ator.Nome, repasseIncerto)
}

// ResolverRecusa tira da fila uma recusa que uma pessoa já tratou.
//
// Volta para PENDENTE porque a recusa tem CERTEZA de que nada saiu — é o único estado
// de falha em que tentar de novo é seguro. O caso comum é o vendedor ter corrigido a
// chave, ou a trava do saque ter sido ligada.
func (s *Store) ResolverRecusa(ctx context.Context, id int64, ator AtorDoRepasse) error {
	return s.mudaRepasse(ctx, id, `
		UPDATE rmt_repasse
		   SET status = $2, resolvido_em = now(), resolvido_por = $3,
		       recusa_http = NULL, recusa_codigo = NULL, recusa_texto = NULL
		 WHERE id = $1 AND status = $4`,
		id, repassePendente, ator.Nome, repasseRecusado)
}

// RepassesEsperandoCadastro conta as dívidas pendentes cujo vendedor não tem chave ou
// não tem documento.
//
// EXISTE SÓ PARA O NÚMERO APARECER. Essas linhas não entram na fila de pagar — não há
// para onde mandar —, e antes disso elas também não entravam em contador nenhum: uma
// varredura com dez delas dizia "nada a fazer". A pessoa ficava esperando o dinheiro
// dela e o log dizia que estava tudo certo.
//
// Consulta separada e não um JOIN na fila de pagar, porque são coisas diferentes: a
// fila é "o que eu vou tentar mandar agora", e isto é "quanta gente está esperando o
// próprio cadastro". Misturá-las faria a fila de pagar carregar linhas que ela nunca
// vai processar.
func (s *Store) RepassesEsperandoCadastro(ctx context.Context) (int, error) {
	var n int
	err := s.pool.QueryRow(ctx, `
		SELECT count(*) FROM rmt_repasse r
		  LEFT JOIN rmt_recebedor d ON d.account_id = r.vendedor_conta
		 WHERE r.status = $1 AND (d.account_id IS NULL OR d.documento IS NULL)`,
		repassePendente).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("store: contando repasses sem cadastro: %w", err)
	}
	return n, nil
}
