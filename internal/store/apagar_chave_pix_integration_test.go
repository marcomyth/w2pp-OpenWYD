//go:build integration

// Apagar a chave Pix: as duas recusas, a ordem entre elas, e o que o histórico guarda.
//
//	W2PP_TEST_DSN=postgres://postgres:dev@localhost:5432/postgres go test -tags=integration ./internal/store/
package store

import (
	"context"
	"errors"
	"strings"
	"testing"
)

const (
	chaveDoTeste = "vendedor@exemplo.com"
	cpfDoTeste   = "11144477735"
)

// comChavePix cadastra a chave e o documento de um vendedor novo.
func comChavePix(ctx context.Context, t *testing.T, s *Store, nome string) int64 {
	t.Helper()
	conta := contaPix(ctx, t, s, nome)
	if err := s.SalvarChavePix(ctx, conta, chaveDoTeste, ChavePixEmail, cpfDoTeste); err != nil {
		t.Fatalf("cadastrando a chave: %v", err)
	}
	return conta
}

func temChave(ctx context.Context, t *testing.T, s *Store, conta int64) bool {
	t.Helper()
	var existe bool
	if err := s.pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM rmt_recebedor WHERE account_id = $1)`, conta).Scan(&existe); err != nil {
		t.Fatal(err)
	}
	return existe
}

// SEM NADA PENDENTE, A CHAVE SAI — e o documento sai junto.
//
// Os dois juntos porque são um cadastro só: chave sem documento não paga, porque a rota de
// repasse exige o CPF. Deixar o documento para trás guardaria dado pessoal que já não serve
// para nada, e dado que não serve é só o que vaza num incidente.
func TestApagarTiraChaveEDocumentoJuntos(t *testing.T) {
	s, ctx := freshStore(t)
	conta := comChavePix(ctx, t, s, "apagar_ok")

	if err := s.ApagarChavePix(ctx, conta); err != nil {
		t.Fatal(err)
	}
	if temChave(ctx, t, s, conta) {
		t.Error("a linha do recebedor continua lá")
	}
}

// SEM CHAVE, A RESPOSTA É "NÃO EXISTE" e não um sucesso silencioso.
//
// Apagar o que já não está lá parece inofensivo, e não é: a tela precisa saber a diferença
// entre "apaguei agora" e "não havia nada", senão ela confirma uma ação que não aconteceu.
func TestApagarSemChaveDizQueNaoExiste(t *testing.T) {
	s, ctx := freshStore(t)
	conta := contaPix(ctx, t, s, "apagar_sem_chave")

	if err := s.ApagarChavePix(ctx, conta); !errors.Is(err, ErrNotFound) {
		t.Fatalf("erro = %v, queria ErrNotFound", err)
	}
}

// COM VENDA EM CURSO, NÃO APAGA.
func TestApagarComCobrancaAbertaRecusa(t *testing.T) {
	s, ctx := freshStore(t)
	vendedor := comChavePix(ctx, t, s, "apagar_venda")
	comprador := contaPix(ctx, t, s, "comprador_venda")
	anuncioComCobrancaAberta(ctx, t, s, vendedor, comprador, "ref-apagar-venda")

	if err := s.ApagarChavePix(ctx, vendedor); !errors.Is(err, ErrVendaEmCurso) {
		t.Fatalf("erro = %v, queria ErrVendaEmCurso", err)
	}
	if !temChave(ctx, t, s, vendedor) {
		t.Error("a chave sumiu mesmo com a recusa")
	}
}

// COM REPASSE A CAMINHO, NÃO APAGA — e o motivo é PRÓPRIO, não o da venda.
//
// A distinção é o conserto que este trabalho traz: antes as duas travas devolviam o mesmo
// erro, e a pessoa não sabia se esperava a compra fechar (minutos) ou o dinheiro sair.
func TestApagarComRepasseEmCursoRecusaComMotivoProprio(t *testing.T) {
	s, ctx := freshStore(t)
	v := montaVenda(ctx, t, s, "apagar_repasse")
	if _, _, err := s.ConfirmarCobrancaRMT(ctx, v.ref, dentroDoPrazo(), HoraDaProcessadora,
		precoEmCentavos, taxaZero()); err != nil {
		t.Fatal(err)
	}

	err := s.ApagarChavePix(ctx, v.vendedor)
	if !errors.Is(err, ErrRepasseEmCurso) {
		t.Fatalf("erro = %v, queria ErrRepasseEmCurso", err)
	}
	if errors.Is(err, ErrVendaEmCurso) {
		t.Error("o repasse veio com o motivo da VENDA; a distincao e o ponto desta mudanca")
	}
}

// A ORDEM DAS RECUSAS: quando as duas valem, a pessoa lê a da VENDA.
//
// Não é gosto: a cobrança aberta passa sozinha quando a compra fechar ou vencer, e o
// repasse só passa quando o dinheiro sair. Dizer primeiro "há repasse a caminho" mandaria
// alguém esperar o dinheiro quando ainda há uma compra aberta que pode nem virar venda.
func TestQuandoAsDuasValemAPessoaLeADaVenda(t *testing.T) {
	s, ctx := freshStore(t)
	// Primeiro uma venda concluída, que cria o repasse.
	v := montaVenda(ctx, t, s, "apagar_ordem")
	if _, _, err := s.ConfirmarCobrancaRMT(ctx, v.ref, dentroDoPrazo(), HoraDaProcessadora,
		precoEmCentavos, taxaZero()); err != nil {
		t.Fatal(err)
	}
	// E agora uma cobrança aberta por cima, do mesmo vendedor.
	comprador := contaPix(ctx, t, s, "comprador_ordem")
	anuncioComCobrancaAberta(ctx, t, s, v.vendedor, comprador, "ref-apagar-ordem")

	if err := s.ApagarChavePix(ctx, v.vendedor); !errors.Is(err, ErrVendaEmCurso) {
		t.Fatalf("erro = %v, queria a recusa da VENDA por ser a primeira da ordem", err)
	}
}

// O HISTÓRICO NÃO GUARDA CHAVE NEM CPF INTEIROS.
//
// A linha sobrevive ao cadastro que ela descreve, então é ela que fica no banco depois de
// a pessoa ter pedido para tirar os dados. Numa disputa, o que se precisa é "a chave
// terminada em tal foi apagada em tal dia" — e não a chave.
func TestOHistoricoDoApagarNaoGuardaNadaInteiro(t *testing.T) {
	s, ctx := freshStore(t)
	conta := comChavePix(ctx, t, s, "apagar_hist")
	if err := s.ApagarChavePix(ctx, conta); err != nil {
		t.Fatal(err)
	}

	rows, err := s.pool.Query(ctx, `
		SELECT coalesce(chave_antiga_mascarada, ''), coalesce(documento_antigo_mascarado, ''),
		       coalesce(chave_nova_mascarada, ''), coalesce(documento_novo_mascarado, '')
		  FROM rmt_recebedor_historico WHERE account_id = $1`, conta)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()

	linhas := 0
	for rows.Next() {
		var campos [4]string
		if err := rows.Scan(&campos[0], &campos[1], &campos[2], &campos[3]); err != nil {
			t.Fatal(err)
		}
		linhas++
		for _, c := range campos {
			if c == "" {
				continue
			}
			if strings.Contains(c, chaveDoTeste) {
				t.Errorf("o historico guardou a chave INTEIRA: %q", c)
			}
			if strings.Contains(c, cpfDoTeste) {
				t.Errorf("o historico guardou o CPF INTEIRO: %q", c)
			}
		}
	}
	if linhas == 0 {
		t.Error("o apagar nao deixou linha no historico")
	}
}

// A ASSIMETRIA É DE PROPÓSITO, e este teste existe para ela ficar escrita.
//
// Com a MESMA chave e um repasse pendente, GRAVAR passa e APAGAR recusa.
//
// Gravar a mesma chave não desvia nada, e é o que deixa um vendedor antigo — cadastrado
// antes de o CPF ser obrigatório — preencher o documento que falta; sem essa saída ele
// cai num nó fechado, porque o repasse fica pendente por falta de documento e o documento
// não pode ser gravado porque o repasse está pendente.
//
// Apagar não é gravar a mesma chave: é tirá-la, com dinheiro já a caminho dela.
func TestMesmaChaveGravaMasNaoApaga(t *testing.T) {
	s, ctx := freshStore(t)
	v := montaVenda(ctx, t, s, "apagar_assimetria")
	if _, _, err := s.ConfirmarCobrancaRMT(ctx, v.ref, dentroDoPrazo(), HoraDaProcessadora,
		precoEmCentavos, taxaZero()); err != nil {
		t.Fatal(err)
	}

	// Regravar a MESMA chave passa, mesmo com o repasse pendente.
	if err := s.SalvarChavePix(ctx, v.vendedor, "vendedor@exemplo.com", ChavePixEmail, cpfDoTeste); err != nil {
		t.Fatalf("regravar a mesma chave devia passar: %v", err)
	}
	// Apagar, na mesma situação, recusa.
	if err := s.ApagarChavePix(ctx, v.vendedor); !errors.Is(err, ErrRepasseEmCurso) {
		t.Fatalf("apagar = %v, queria ErrRepasseEmCurso", err)
	}
}

var _ = context.Background
