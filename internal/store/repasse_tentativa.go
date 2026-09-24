package store

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// ErrRepasseNaoPendente é tentar mandar uma dívida que não está esperando envio.
//
// É a recusa que carrega a trava mais importante deste arquivo. Ver ReferenciaDaTentativa.
var ErrRepasseNaoPendente = errors.New("store: o repasse nao esta pendente")

// ReferenciaDaTentativa é a referência que vai ao fio numa tentativa de repasse.
//
// A PONTE DEIXOU DE SER A TRAVA DE "UM PAGAMENTO POR VENDA", e isto é o que esta função
// obriga a lembrar. Enquanto a referência era a da cobrança, a ponte sabia que duas
// chamadas eram a mesma dívida e recusava a segunda sozinha. Com uma referência POR
// TENTATIVA ela não sabe mais: cada tentativa é, para ela, um repasse diferente.
//
// A trava passou para cá, e ela é o estado PENDENTE. Uma tentativa nova só nasce quando:
//
//   - a anterior voltou RECUSADA, que é o único resultado com certeza de que nada saiu;
//   - ou uma PESSOA foi ao painel da processadora, viu que o incerto não pagou, e disse
//     isso — com nome e hora.
//
// Nunca automaticamente depois de um incerto. O teto diário da ponte continua como a
// última rede, e é rede e não trava.
//
// POR QUE A REFERÊNCIA MUDA A CADA TENTATIVA, já que isso custou a trava: a ponte trava
// a referência de um incerto PARA SEMPRE. Reenviar com ela devolve o resultado da
// primeira vez — que é justamente o que ninguém sabe qual foi. Sem referência nova, uma
// dívida que caiu em incerto nunca mais poderia ser paga.
//
// DETERMINÍSTICA de propósito: se a chamada se perder e a MESMA tentativa for repetida,
// sai a mesma referência e a idempotência da ponte funciona. O que muda entre tentativas
// é o número, e só ele.
//
// O prefixo é separação de domínio: sem ele, um id que por acaso coincidisse com outro
// número do sistema geraria a mesma referência para coisas diferentes.
func ReferenciaDaTentativa(repasseID int64, tentativa int) string {
	soma := sha256.Sum256(fmt.Appendf(nil, "w2pp-repasse:%d:%d", repasseID, tentativa))
	// 32 hex = 16 bytes = 128 bits. A ponte valida "32 hex minúsculos", e colisão em
	// 128 bits é desprezível — ainda mais num universo de algumas centenas de linhas.
	return hex.EncodeToString(soma[:16])
}

// TentativaDeRepasse é o que quem vai chamar a ponte precisa saber.
type TentativaDeRepasse struct {
	ID         int64
	RepasseID  int64
	Numero     int
	Referencia string
}

// AbrirTentativa cria a próxima tentativa de uma dívida, gravando a referência ANTES da
// chamada à ponte.
//
// ANTES e não depois, e essa ordem é a única que funciona: se a resposta se perder, é
// por esta linha que se descobre o que foi mandado. Uma referência gravada só no sucesso
// deixaria o incerto — o caso em que ela mais importa — sem registro nenhum.
//
// SÓ ABRE SE A DÍVIDA ESTIVER PENDENTE, e é aqui que mora a trava que a ponte deixou de
// fazer. O FOR UPDATE segura a linha enquanto conta as tentativas: sem ele, dois
// trabalhadores leriam "pendente" ao mesmo tempo e abririam a tentativa 2 duas vezes —
// com referências diferentes, que a ponte pagaria as duas.
//
// liberadoPor fica vazio na primeira tentativa, que é automática, e carrega o nome de
// quem autorizou a partir da segunda. É o que liga o segundo pagamento à decisão que o
// permitiu; sem ele, ele pareceria um bug.
func (s *Store) AbrirTentativa(ctx context.Context, repasseID int64, liberadoPor string) (TentativaDeRepasse, error) {
	var t TentativaDeRepasse
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		var status int16
		err := tx.QueryRow(ctx,
			`SELECT status FROM rmt_repasse WHERE id = $1 FOR UPDATE`, repasseID).Scan(&status)
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("repasse %d: %w", repasseID, ErrRepasseInexistente)
		}
		if err != nil {
			return fmt.Errorf("store: abrindo tentativa do repasse %d: %w", repasseID, err)
		}
		if status != repassePendente {
			// Inclui o INCERTO, e é o caso que mais importa: uma dívida incerta PODE
			// ter sido paga, e abrir uma tentativa nova nela é o caminho para pagar
			// duas vezes.
			return fmt.Errorf("repasse %d, status %d: %w", repasseID, status, ErrRepasseNaoPendente)
		}

		var ultima int
		if err := tx.QueryRow(ctx, `
			SELECT coalesce(max(tentativa), 0) FROM rmt_repasse_tentativa
			 WHERE repasse_id = $1`, repasseID).Scan(&ultima); err != nil {
			return fmt.Errorf("store: contando tentativas do repasse %d: %w", repasseID, err)
		}

		t.RepasseID = repasseID
		t.Numero = ultima + 1
		t.Referencia = ReferenciaDaTentativa(repasseID, t.Numero)

		var por *string
		if liberadoPor != "" {
			por = &liberadoPor
		}
		if err := tx.QueryRow(ctx, `
			INSERT INTO rmt_repasse_tentativa (repasse_id, tentativa, referencia, liberado_por)
			VALUES ($1, $2, $3, $4) RETURNING id`,
			repasseID, t.Numero, t.Referencia, por).Scan(&t.ID); err != nil {
			return fmt.Errorf("store: gravando a tentativa %d do repasse %d: %w",
				t.Numero, repasseID, err)
		}
		return nil
	})
	if err != nil {
		return TentativaDeRepasse{}, err
	}
	return t, nil
}

// FecharTentativa grava o que a ponte respondeu.
//
// A linha da tentativa guarda o resultado CRU de cada envio, e é ela que responde numa
// disputa: "quantas vezes tentaram me pagar, quando, e o que deu em cada uma". O estado
// do repasse é o resumo; isto é o histórico.
func (s *Store) FecharTentativa(ctx context.Context, tentativaID int64, resultado EstadoRepasse,
	identifierSaque string, httpSyncpay *int32, codigo, texto string,
) error {
	var saque *string
	if identifierSaque != "" {
		saque = &identifierSaque
	}
	tag, err := s.pool.Exec(ctx, `
		UPDATE rmt_repasse_tentativa
		   SET resultado = $2, identifier_saque = $3, recusa_http = $4,
		       recusa_codigo = $5, recusa_texto = $6, respondido_em = now()
		 WHERE id = $1 AND resultado IS NULL`,
		tentativaID, int16(resultado), saque, httpSyncpay, codigo, texto)
	if err != nil {
		return fmt.Errorf("store: fechando a tentativa %d: %w", tentativaID, err)
	}
	if tag.RowsAffected() == 0 {
		// Já fechada. Não é sucesso silencioso: quer dizer que alguém respondeu por
		// esta tentativa antes, e em dinheiro seguir adiante achando que mudou é como
		// se paga duas vezes.
		return fmt.Errorf("tentativa %d: %w", tentativaID, ErrRepasseInexistente)
	}
	return nil
}

// TentativasDoRepasse devolve o histórico de uma dívida, da primeira à última.
func (s *Store) TentativasDoRepasse(ctx context.Context, repasseID int64) ([]TentativaDeRepasse, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, repasse_id, tentativa, referencia
		  FROM rmt_repasse_tentativa WHERE repasse_id = $1 ORDER BY tentativa`, repasseID)
	if err != nil {
		return nil, fmt.Errorf("store: tentativas do repasse %d: %w", repasseID, err)
	}
	defer rows.Close()
	var out []TentativaDeRepasse
	for rows.Next() {
		var t TentativaDeRepasse
		if err := rows.Scan(&t.ID, &t.RepasseID, &t.Numero, &t.Referencia); err != nil {
			return nil, fmt.Errorf("store: tentativas do repasse %d: %w", repasseID, err)
		}
		out = append(out, t)
	}
	return out, rows.Err()
}
