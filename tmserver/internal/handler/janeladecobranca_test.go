package handler

import (
	"strings"
	"testing"
	"time"

	"github.com/jeanluca/w2pp-openwyd/internal/store"
)

// OS DOIS PADRÕES DO MESMO PRAZO TÊM DE CASAR.
//
// handler.JanelaDeCobranca é o que o jogo PROMETE ao comprador na mensagem; e
// store.JanelaPadraoCobranca é o que GRAVA o expira_em, noutro processo. São dois
// padrões da mesma coisa, e se discordarem a mensagem mente — "pague em 15 minutos"
// com a cobrança vencendo em 5 é a mentira mais cara que esta tela pode contar.
//
// Este teste existe porque o defeito é fácil e silencioso: alguém muda um número e o
// outro fica. Nada quebra, nada avisa, e a diferença só aparece num jogador que pagou
// dentro do prazo prometido e não recebeu.
func TestOsDoisPadroesDaJanelaCasam(t *testing.T) {
	if JanelaDeCobranca != store.JanelaPadraoCobranca {
		t.Errorf("o jogo promete %v e o banco grava %v; os dois padroes sao do MESMO prazo",
			JanelaDeCobranca, store.JanelaPadraoCobranca)
	}
}

// E A MENSAGEM DIZ O PRAZO DE VERDADE, seja ele qual for.
//
// O número não está escrito na frase de propósito: com ele fixo, mudar a janela
// deixaria a mensagem mentindo. Este teste prova que a frase acompanha.
func TestAMensagemDizOPrazoDeVerdade(t *testing.T) {
	anterior := JanelaDeCobranca
	t.Cleanup(func() { JanelaDeCobranca = anterior })

	for _, d := range []time.Duration{5 * time.Minute, 15 * time.Minute, 30 * time.Minute} {
		JanelaDeCobranca = d
		msg := msgPagueNoSite()
		quer := map[time.Duration]string{
			5 * time.Minute:  "5 minutos",
			15 * time.Minute: "15 minutos",
			30 * time.Minute: "30 minutos",
		}[d]
		if !strings.Contains(msg, quer) {
			t.Errorf("com janela de %v a mensagem diz %q; queria conter %q", d, msg, quer)
		}
	}
}

// E O PADRÃO DE HOJE É QUINZE, para a mudança de 25/09/2026 não voltar sozinha num
// merge distraído.
func TestOPadraoEhQuinzeMinutos(t *testing.T) {
	if JanelaDeCobranca != 15*time.Minute {
		t.Errorf("JanelaDeCobranca = %v, queria 15m", JanelaDeCobranca)
	}
}
