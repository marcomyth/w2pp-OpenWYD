package store

import (
	"context"
	"fmt"
)

// MotivoDaEspera é por que o dinheiro de um vendedor ainda não chegou. Os números
// são os do contrato gRPC (PayoutWaitReason).
type MotivoDaEspera int16

const (
	// SemEspera: não há nada a receber. É o que sai quando o total é zero.
	SemEspera MotivoDaEspera = 0
	// EsperaCadastro: falta chave ou falta documento. É a ÚNICA espera que o
	// próprio vendedor resolve.
	EsperaCadastro MotivoDaEspera = 1
	// EsperaPagamento: o cadastro está completo e o pagamento está a caminho.
	EsperaPagamento MotivoDaEspera = 2
	// EsperaGente: recusado ou incerto. Alguém tem de olhar.
	EsperaGente MotivoDaEspera = 3
)

// RepasseDoVendedor responde a única pergunta que um vendedor com dinheiro a
// receber faz: quanto, e por que ainda não chegou.
//
// O QUE CONTA COMO "A RECEBER" são os quatro estados que não são PAGO — pendente,
// enviado, recusado e incerto. RECUSADO entra, e entra porque a recusa não apaga
// a dívida: o dinheiro continua sendo do vendedor, só não achou o caminho. Deixá-lo
// de fora faria o total sumir da tela no exato momento em que a pessoa mais precisa
// ver que ele existe.
//
// O TOTAL É POR CONTA E NÃO POR VENDA, e nada aqui identifica venda, data ou
// comprador. Pelo mesmo motivo da chave mascarada: a tela é pública o bastante para
// um print, e quem olha por cima do ombro não tem de descobrir o que a pessoa vendeu.
// O detalhe é da staff, que pode ser perguntada.
//
// A ORDEM DE PRECEDÊNCIA: cadastro, depois gente, depois pagamento. Cadastro
// ganha por ser o único motivo que a pessoa resolve sozinha — dizer "estamos
// verificando" enquanto o cadastro dela é o que trava a deixaria esperando um dia
// que não chega. E "a caminho" fica por último porque é a resposta mais
// tranquilizadora das três, e dá-la enquanto um repasse está incerto seria
// prometer o que ninguém sabe.
//
// MAS OS DOIS PRIMEIROS NÃO SE CRUZAM HOJE, e vale dizer para ninguém procurar o
// teste que provaria a precedência: GENTE só nasce de uma tentativa, a fila de
// pagar exige documento, e o documento não some depois de gravado. Logo, uma conta
// sem documento nunca tem recusado nem incerto. A ordem está escrita porque a
// escolha tinha de ser feita em algum lugar, e não porque o caso acontece.
func (s *Store) RepasseDoVendedor(ctx context.Context, accountID int64) (int64, MotivoDaEspera, error) {
	var total int64
	var semCadastro, precisaGente, emAndamento int
	err := s.pool.QueryRow(ctx, `
		SELECT COALESCE(sum(r.valor_centavos), 0),
		       count(*) FILTER (WHERE r.status = $2
		                          AND (d.account_id IS NULL
		                               OR d.chave IS NULL
		                               OR d.documento IS NULL)),
		       count(*) FILTER (WHERE r.status IN ($5, $6, $7)),
		       count(*) FILTER (WHERE r.status = $3
		                          OR (r.status = $2
		                              AND d.documento IS NOT NULL
		                              AND d.chave IS NOT NULL))
		  FROM rmt_repasse r
		  LEFT JOIN rmt_recebedor d ON d.account_id = r.vendedor_conta
		 WHERE r.vendedor_conta = $1 AND r.status <> $4`,
		accountID, repassePendente, repasseEnviado, repassePago,
		repasseRecusado, repasseIncerto, repasseSemTaxa).
		Scan(&total, &semCadastro, &precisaGente, &emAndamento)
	if err != nil {
		return 0, SemEspera, fmt.Errorf("store: repasse do vendedor a=%d: %w", accountID, err)
	}

	switch {
	case total == 0:
		return 0, SemEspera, nil
	case semCadastro > 0:
		return total, EsperaCadastro, nil
	case precisaGente > 0:
		// O SEGURADO POR TAXA DESCONHECIDA cai aqui, junto do recusado e do incerto, e
		// é o balde certo: quem resolve é uma pessoa informando a taxa, não o tempo.
		//
		// O total dele é o BRUTO, porque sem a taxa não existe líquido para somar. É
		// mais do que vai cair na conta, pela taxa, e é por isso que ele NÃO pode vir
		// com "a caminho": o motivo diz que alguém está olhando, e é o motivo que
		// impede a tela de prometer aquele número como valor final. A alternativa era
		// deixá-lo fora da soma, e aí o vendedor veria zero com dinheiro a receber —
		// a invisibilidade que RepasseEsperandoCadastro existe para não repetir.
		return total, EsperaGente, nil
	case emAndamento > 0:
		return total, EsperaPagamento, nil
	}
	// Dívida que não caiu em nenhum balde é estado novo que ninguém previu aqui.
	// EsperaGente é a resposta certa para isso: manda olhar, que é o que se faz
	// quando não se sabe. Dizer "a caminho" seria inventar.
	return total, EsperaGente, nil
}
