package store

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/jackc/pgx/v5"
)

// A chave Pix de RECEBIMENTO do vendedor — para onde vai o dinheiro quando ele
// vende um item por dinheiro real.
//
// Mora em `rmt_recebedor` (0105), tabela própria e não coluna no account, pelo
// mesmo desenho do `donate_payer_profile`: um dado de pagamento por conta,
// preenchido pelo site. E são as duas coisas separadas de propósito — lá é quem
// PAGA, aqui é quem RECEBE, e na maioria das vezes não é a mesma pessoa.

// TipoChavePix é a forma da chave. Os números são os do contrato gRPC.
type TipoChavePix int16

const (
	ChavePixCPF       TipoChavePix = 1
	ChavePixEmail     TipoChavePix = 2
	ChavePixTelefone  TipoChavePix = 3
	ChavePixAleatoria TipoChavePix = 4
)

// Respostas previstas, e não falhas: quem chama traduz em recusa para a tela.
var (
	// ErrChavePixInvalida é a chave que não tem a forma do tipo declarado.
	ErrChavePixInvalida = errors.New("store: chave pix invalida para o tipo")
	// ErrVendaEmCurso é a recusa que impede o desvio de um pagamento já em
	// andamento. Ver a nota em SalvarChavePix.
	ErrVendaEmCurso = errors.New("store: ha cobranca aberta; a chave nao pode mudar agora")
)

var (
	soDigitos    = regexp.MustCompile(`^[0-9]+$`)
	pareceEmail  = regexp.MustCompile(`^[^@\s]+@[^@\s]+\.[^@\s]+$`)
	pareceAleat  = regexp.MustCompile(`^[0-9a-fA-F-]{32,36}$`)
	espacoDemais = regexp.MustCompile(`\s`)
)

// validaChavePix confere a FORMA da chave contra o tipo declarado.
//
// É conferência de forma e não de existência: só o banco do jogador sabe se a
// chave existe, e descobrir isso custa uma chamada à processadora. O que esta
// função evita é o erro de digitação chegar até lá — e, no dia do repasse, virar
// um pagamento recusado que ninguém entende.
func validaChavePix(chave string, tipo TipoChavePix) error {
	chave = strings.TrimSpace(chave)
	if chave == "" || espacoDemais.MatchString(chave) {
		return ErrChavePixInvalida
	}
	switch tipo {
	case ChavePixCPF:
		// Onze dígitos. NÃO conferimos o dígito verificador aqui: a regra do CPF
		// é do domínio fiscal e não do nosso, e uma implementação errada dela
		// recusaria CPF válido — que é pior do que deixar passar um inválido, que
		// a processadora recusa de qualquer jeito.
		if len(chave) != 11 || !soDigitos.MatchString(chave) {
			return ErrChavePixInvalida
		}
	case ChavePixEmail:
		if len(chave) > 77 || !pareceEmail.MatchString(chave) {
			return ErrChavePixInvalida
		}
	case ChavePixTelefone:
		// +55 e mais 10 ou 11 dígitos, que é a forma que o Pix usa.
		if !strings.HasPrefix(chave, "+") {
			return ErrChavePixInvalida
		}
		n := strings.TrimPrefix(chave, "+")
		if len(n) < 12 || len(n) > 14 || !soDigitos.MatchString(n) {
			return ErrChavePixInvalida
		}
	case ChavePixAleatoria:
		if !pareceAleat.MatchString(chave) {
			return ErrChavePixInvalida
		}
	default:
		return ErrChavePixInvalida
	}
	return nil
}

// MascaraChavePix devolve o bastante para a pessoa reconhecer a própria chave, e
// nada além disso.
//
// A LEITURA NUNCA DEVOLVE A CHAVE INTEIRA. O formulário só precisa dizer "você já
// cadastrou ...1234"; chave Pix é dado pessoal e o site é público. Devolver
// inteira seria pôr o dado no navegador de graça, e daí ele vai para o histórico,
// para o cache e para qualquer extensão instalada.
//
// O e-mail guarda a primeira letra e o domínio porque é assim que a pessoa
// reconhece o dela quando tem dois; os outros guardam os quatro últimos.
func MascaraChavePix(chave string, tipo TipoChavePix) string {
	chave = strings.TrimSpace(chave)
	if chave == "" {
		return ""
	}
	if tipo == ChavePixEmail {
		if i := strings.IndexByte(chave, '@'); i > 0 {
			return chave[:1] + "***" + chave[i:]
		}
	}
	if len(chave) <= 4 {
		return "***"
	}
	return "***" + chave[len(chave)-4:]
}

// RecebedorPix é o que a leitura devolve. Repare que a chave NÃO está aqui: só a
// máscara sai desta camada, para não existir um caminho em que a inteira escape
// por descuido de quem chamar.
type RecebedorPix struct {
	TemChave       bool
	ChaveMascarada string
	Tipo           TipoChavePix
	Verificada     bool
}

// SalvarChavePix grava a chave de recebimento da conta.
//
// ELA RECUSA COM COBRANÇA ABERTA, e essa é a trava que importa. Sem ela o roteiro
// é de dois cliques: anunciar, esperar o comprador abrir o QR, trocar a chave, e
// receber numa conta diferente da que estava valendo quando a venda começou.
//
// A trava mora AQUI e não no formulário porque o site é uma das portas e não a
// única — defesa que mora na tela não é defesa.
//
// A leitura das cobranças e a gravação acontecem na MESMA transação, com a linha
// do recebedor travada: sem isso, uma cobrança aberta entre a conferência e o
// UPDATE passaria pelo meio das duas.
func (s *Store) SalvarChavePix(ctx context.Context, accountID int64, chave string, tipo TipoChavePix) error {
	if err := validaChavePix(chave, tipo); err != nil {
		return err
	}
	chave = strings.TrimSpace(chave)

	return s.inTx(ctx, func(tx pgx.Tx) error {
		var existe bool
		if err := tx.QueryRow(ctx,
			`SELECT EXISTS(SELECT 1 FROM account WHERE id = $1 FOR UPDATE)`, accountID).Scan(&existe); err != nil {
			return fmt.Errorf("store: chave pix: lendo a conta a=%d: %w", accountID, err)
		}
		if !existe {
			return ErrNotFound
		}

		var abertas int
		if err := tx.QueryRow(ctx, `
			SELECT count(*) FROM rmt_cobranca c
			  JOIN rmt_anuncio a ON a.id = c.anuncio_id
			 WHERE a.vendedor_conta = $1 AND c.status = 1`, accountID).Scan(&abertas); err != nil {
			return fmt.Errorf("store: chave pix: contando cobrancas a=%d: %w", accountID, err)
		}
		if abertas > 0 {
			return ErrVendaEmCurso
		}

		// O que havia ANTES, lido antes de sobrescrever: é o "de onde" do rastro.
		// Nulo aqui significa primeiro cadastro, e não "não sei qual era".
		var antigaChave *string
		var antigoTipo *int16
		if err := tx.QueryRow(ctx,
			`SELECT chave, tipo FROM rmt_recebedor WHERE account_id = $1`, accountID).
			Scan(&antigaChave, &antigoTipo); err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("store: chave pix: lendo a anterior a=%d: %w", accountID, err)
		}

		// Trocar a chave zera a verificação: a chave nova não é a que foi
		// verificada. Deixar a marca de pé seria dizer que conferimos o que não
		// conferimos.
		if _, err := tx.Exec(ctx, `
			INSERT INTO rmt_recebedor (account_id, chave, tipo, verificada_em, updated_at)
			VALUES ($1, $2, $3, NULL, now())
			ON CONFLICT (account_id) DO UPDATE
			   SET chave = EXCLUDED.chave, tipo = EXCLUDED.tipo,
			       verificada_em = NULL, updated_at = now()`,
			accountID, chave, int16(tipo)); err != nil {
			return fmt.Errorf("store: chave pix: gravando a=%d: %w", accountID, err)
		}

		// O RASTRO, na MESMA transação. Fora dela, uma queda entre as duas
		// escritas deixaria a chave trocada e o registro perdido — que é
		// exatamente o caso em que alguém vai querer o registro.
		//
		// Mascaradas as duas, aqui também: a máscara já responde "mudou de ...1234
		// para ...9876", que é o que a disputa precisa, e um log é o último lugar
		// onde se deve guardar dado pessoal de pagamento inteiro.
		var antigaMasc *string
		if antigaChave != nil && antigoTipo != nil {
			m := MascaraChavePix(*antigaChave, TipoChavePix(*antigoTipo))
			antigaMasc = &m
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO rmt_recebedor_historico
				(account_id, tipo_antigo, chave_antiga_mascarada, tipo_novo, chave_nova_mascarada)
			VALUES ($1, $2, $3, $4, $5)`,
			accountID, antigoTipo, antigaMasc, int16(tipo), MascaraChavePix(chave, tipo)); err != nil {
			return fmt.Errorf("store: chave pix: gravando o historico a=%d: %w", accountID, err)
		}
		return nil
	})
}

// LerChavePix devolve o estado da chave, sempre MASCARADA.
func (s *Store) LerChavePix(ctx context.Context, accountID int64) (RecebedorPix, error) {
	var chave string
	var tipo int16
	var verificada bool
	err := s.pool.QueryRow(ctx, `
		SELECT chave, tipo, verificada_em IS NOT NULL
		  FROM rmt_recebedor WHERE account_id = $1`, accountID).Scan(&chave, &tipo, &verificada)
	if errors.Is(err, pgx.ErrNoRows) {
		return RecebedorPix{}, nil
	}
	if err != nil {
		return RecebedorPix{}, fmt.Errorf("store: chave pix: lendo a=%d: %w", accountID, err)
	}
	return RecebedorPix{
		TemChave:       true,
		ChaveMascarada: MascaraChavePix(chave, TipoChavePix(tipo)),
		Tipo:           TipoChavePix(tipo),
		Verificada:     verificada,
	}, nil
}

// TemChavePixValida é o que o `lojaAbrir` pergunta antes de aceitar um anúncio em
// dinheiro real: sem chave gravada, a recusa é na hora de ANUNCIAR e não na hora
// de pagar.
//
// Recusar no fim seria descobrir o problema com o comprador já com o QR aberto —
// e o vendedor é quem teria de consertar, sem estar por perto.
func (s *Store) TemChavePixValida(ctx context.Context, accountID int64) (bool, error) {
	var tem bool
	if err := s.pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM rmt_recebedor WHERE account_id = $1)`, accountID).Scan(&tem); err != nil {
		return false, fmt.Errorf("store: chave pix: conferindo a=%d: %w", accountID, err)
	}
	return tem, nil
}
