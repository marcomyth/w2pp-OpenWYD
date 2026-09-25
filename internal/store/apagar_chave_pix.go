package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// ErrRepasseEmCurso é a recusa por dinheiro a caminho da chave.
//
// SEPARADA DE ErrVendaEmCurso, e a separação é o conserto. As duas travas existiam desde
// sempre, mas devolviam o MESMO erro — então a pessoa lia "venda em curso" nos dois casos,
// que pedem coisas diferentes dela: a cobrança aberta passa sozinha quando a compra
// fechar ou vencer, e o repasse só passa quando o dinheiro sair. Quem não distingue os
// dois não sabe se espera cinco minutos ou fala com a equipe.
var ErrRepasseEmCurso = errors.New("store: ha repasse a caminho; a chave nao pode sair agora")

// haCobrancaAberta diz se aquele vendedor tem venda em andamento.
//
// EXTRAÍDA para servir aos dois caminhos — gravar chave e apagar chave — porque a regra é
// a mesma e uma cópia é como as duas saem de sincronia. Recebe a transação: quem chama já
// travou a linha, e uma consulta fora dela responderia sobre um instante diferente.
func haCobrancaAberta(ctx context.Context, tx pgx.Tx, accountID int64) (bool, error) {
	var abertas int
	if err := tx.QueryRow(ctx, `
		SELECT count(*) FROM rmt_cobranca c
		  JOIN rmt_anuncio a ON a.id = c.anuncio_id
		 WHERE a.vendedor_conta = $1 AND c.status = 1`, accountID).Scan(&abertas); err != nil {
		return false, fmt.Errorf("store: contando cobrancas abertas a=%d: %w", accountID, err)
	}
	return abertas > 0, nil
}

// haRepasseEmCurso diz se há dinheiro a caminho da chave daquele vendedor.
//
// SÓ A CONSULTA, e não a decisão de quando aplicá-la — essa fica com quem chama, e a
// diferença entre os dois chamadores é deliberada:
//
// O SalvarChavePix só confere quando a chave MUDA DE VALOR. Gravar a mesma chave não
// desvia nada, e é o que faz um vendedor antigo, cadastrado antes de o CPF ser
// obrigatório, conseguir preencher o documento que falta. Sem essa saída ele cai num nó
// fechado: o repasse fica pendente por falta de documento, e o documento não pode ser
// gravado porque o repasse está pendente.
//
// O APAGAR confere SEMPRE. Apagar não é gravar a mesma chave, é tirá-la — a saída de
// cima não tem sentido ali, e herdá-la deixaria alguém apagar a chave com dinheiro já a
// caminho dela.
//
// OS QUATRO ESTADOS que travam: pendente, enviado, incerto e SEM TAXA.
//
// RECUSADO não trava, pelo motivo de sempre: a recusa mais comum é a chave estar errada,
// e travar a correção prenderia o dinheiro para sempre. PAGO não trava porque acabou.
//
// SEM TAXA (0131) trava, e é o oposto do recusado: ali o problema pode ser a chave, aqui
// ela está boa e o que falta é um número nosso. Deixar mexer abriria o desvio exato que a
// trava existe para impedir — vender, esperar o repasse segurar, e apontar o dinheiro
// para outro lugar antes de ele sair.
//
// O PENDENTE não é filtrado por "com chave", de propósito: quem chega aqui já tem chave,
// e acrescentar o filtro mudaria comportamento que ninguém pediu.
func haRepasseEmCurso(ctx context.Context, tx pgx.Tx, accountID int64) (bool, error) {
	var repasses int
	if err := tx.QueryRow(ctx, `
		SELECT count(*) FROM rmt_repasse
		 WHERE vendedor_conta = $1 AND status IN ($2, $3, $4, $5)`,
		accountID, repassePendente, repasseEnviado, repasseIncerto, repasseSemTaxa).
		Scan(&repasses); err != nil {
		return false, fmt.Errorf("store: contando repasses em curso a=%d: %w", accountID, err)
	}
	return repasses > 0, nil
}

// ApagarChavePix tira a chave e o documento do vendedor, os dois juntos.
//
// OS DOIS JUNTOS porque são um cadastro só: uma chave sem documento não paga — a rota de
// repasse da ponte exige o CPF —, então deixar o documento para trás guardaria dado
// pessoal que já não serve para nada. E guardar dado pessoal que não serve é só aumentar
// o que vaza num incidente.
//
// A ORDEM DAS RECUSAS É COBRANÇA E DEPOIS REPASSE, e ela importa: quando as duas valem, a
// pessoa lê a da venda, que é a que passa sozinha antes. Dizer primeiro "há repasse a
// caminho" mandaria alguém esperar o dinheiro sair quando, na verdade, ainda há uma
// compra aberta que pode nem virar venda.
//
// TUDO NUMA TRANSAÇÃO, com a linha travada, pelo mesmo motivo do SalvarChavePix: entre
// conferir e apagar cabe uma compra nova, e apagar a chave de uma venda que acabou de
// acontecer é exatamente o que as travas existem para impedir.
func (s *Store) ApagarChavePix(ctx context.Context, accountID int64) error {
	return s.inTx(ctx, func(tx pgx.Tx) error {
		var chave string
		var tipo int16
		var doc *string
		err := tx.QueryRow(ctx, `
			SELECT chave, tipo, documento FROM rmt_recebedor
			 WHERE account_id = $1 FOR UPDATE`, accountID).Scan(&chave, &tipo, &doc)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return fmt.Errorf("store: lendo a chave para apagar a=%d: %w", accountID, err)
		}

		// AS TRAVAS VÊM DEPOIS DA LEITURA E ANTES DA ESCRITA, com a linha já travada.
		venda, err := haCobrancaAberta(ctx, tx, accountID)
		if err != nil {
			return err
		}
		if venda {
			return ErrVendaEmCurso
		}
		repasse, err := haRepasseEmCurso(ctx, tx, accountID)
		if err != nil {
			return err
		}
		if repasse {
			return ErrRepasseEmCurso
		}

		// O HISTÓRICO GUARDA SÓ A MÁSCARA, nunca a chave nem o documento inteiros.
		//
		// É o mesmo cuidado do caminho de gravar, e aqui pesa mais: a linha vai
		// sobreviver ao cadastro que ela descreve. Numa disputa, o que se precisa é "a
		// chave terminada em ...9876 foi apagada em tal dia", e não a chave.
		if _, err := tx.Exec(ctx, `
			INSERT INTO rmt_recebedor_historico
				(account_id, tipo_antigo, chave_antiga_mascarada, documento_antigo_mascarado)
			VALUES ($1, $2, $3, $4)`,
			accountID, tipo, MascaraChavePix(chave, TipoChavePix(tipo)),
			mascaraOuNulo(doc)); err != nil {
			return fmt.Errorf("store: gravando o historico do apagar a=%d: %w", accountID, err)
		}

		if _, err := tx.Exec(ctx,
			`DELETE FROM rmt_recebedor WHERE account_id = $1`, accountID); err != nil {
			return fmt.Errorf("store: apagando a chave a=%d: %w", accountID, err)
		}
		return nil
	})
}
