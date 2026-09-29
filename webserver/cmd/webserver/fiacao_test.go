package main

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// O TESTE QUE PEGA A FIAÇÃO ESQUECIDA.
//
// NASCEU DE UM DEFEITO DE VERDADE. O serviço de montaria passou a conferir cargo, com
// um conferidor que precisa ser LIGADO por quem monta o serviço, e que RECUSA tudo se
// ninguém ligar (falha fechada, de propósito). Eu escrevi o conferidor, escrevi a falha
// fechada — e esqueci de ligá-lo aqui. O resultado seria montaria que não salva para
// NINGUÉM, nem para quem salvava antes, e nenhum teste de unidade pegaria: cada um
// monta o serviço por conta própria e liga o que precisa.
//
// POR QUE ELE LÊ O FONTE em vez de subir o servidor: subir o webServer exige banco,
// conteúdo e chaves, e um teste que precisa de tudo isso não roda no lugar onde o erro
// aparece. O que falta aqui é uma LINHA DE FIAÇÃO, e a pergunta "essa linha existe?" o
// fonte responde direto. É um teste tosco de propósito, e tosco que pega é melhor que
// elegante que não roda.
func TestOMainLigaOConferidorDeCargoDaMontaria(t *testing.T) {
	t.Parallel()
	fonte := leMain(t)

	if !strings.Contains(fonte, "mountgrowth.New(st)") {
		t.Fatal("o main nao monta mais o servico de montaria; este teste precisa ser revisto")
	}
	if !strings.Contains(fonte, "mountGrowthAdmin.ComCargos(") {
		t.Error("o main NAO liga o conferidor de cargo da montaria (ComCargos): " +
			"a montaria vai recusar toda escrita, para todo mundo")
	}
}

// TestTodoServicoQueMontaTemOQuePrecisa guarda a ordem das duas linhas.
//
// LIGAR DEPOIS DE USAR NÃO SERVE. Se um dia alguém puser o ComCargos abaixo do ponto
// onde o serviço é entregue ao servidor gRPC, ele já teria sido usado sem conferidor
// durante a montagem. Aqui a conferência é simples: o ComCargos vem antes do
// NewMountGrowthAdmin.
func TestOConferidorEhLigadoAntesDeOServicoSerEntregue(t *testing.T) {
	t.Parallel()
	fonte := leMain(t)
	liga := strings.Index(fonte, "mountGrowthAdmin.ComCargos(")
	entrega := strings.Index(fonte, "NewMountGrowthAdmin(")
	if liga < 0 {
		t.Skip("a outra prova ja reclama da ausencia")
	}
	if entrega < 0 {
		t.Skip("o main nao entrega mais o servico por este nome")
	}
	if liga > entrega {
		t.Errorf("o ComCargos (posicao %d) vem DEPOIS da entrega do servico (posicao %d)", liga, entrega)
	}
}

func leMain(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatalf("lendo o main.go: %v", err)
	}
	// Tira os comentários, para o teste não passar por causa de uma MENÇÃO ao ComCargos
	// dentro de um comentário — que é exactamente o estado em que o defeito estava: a
	// função existia, documentada, e ninguém a chamava.
	return semComentarios(string(b))
}

var comentarioDeLinha = regexp.MustCompile(`(?m)^\s*//.*$`)

func semComentarios(s string) string { return comentarioDeLinha.ReplaceAllString(s, "") }
