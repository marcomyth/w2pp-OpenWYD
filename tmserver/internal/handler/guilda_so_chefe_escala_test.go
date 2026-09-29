package handler

import (
	"testing"

	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/protocol"
)

// SÓ O LÍDER E OS SUB-LÍDERES ESCALAM PARA AS CIDADES, decisão da Hanna em 29/09/2026.
//
// SEM ISTO QUALQUER MEMBRO ESCALAVA. O painel esconde o botão de quem não pode, e
// esconder é tudo o que ele faz: o pacote continua chegando de quem o montar à mão.
// Quem decide é o servidor.

// TestQuemEscalaEQuemNaoEscala é a régua, cargo por cargo.
//
// A TABELA VEM DOS CARGOS QUE O JOGO EMITE: 9 é o líder (guildLeaderLevel), 6 a 8 são
// os sub-líderes que o /subcreate cria, e 0 é o membro comum. Os valores do meio (1 a 5)
// não são emitidos por nada hoje, e por isso entram aqui como NÃO — um cargo que
// ninguém sabe de onde veio não deve ganhar poder por omissão.
func TestQuemEscalaEQuemNaoEscala(t *testing.T) {
	t.Parallel()
	casos := map[uint8]bool{
		9: true,  // líder
		8: true,  // sub-líder
		7: true,  // sub-líder
		6: true,  // sub-líder
		5: false, // ninguém emite
		1: false, // ninguém emite
		0: false, // membro comum
	}
	for cargo, quer := range casos {
		if got := guildaPodeEscalar(cargo); got != quer {
			t.Errorf("cargo %d: guildaPodeEscalar = %v, queria %v", cargo, got, quer)
		}
	}
}

// TestAFraseDaRecusaCabeNoPainel mede os BYTES em cp1252, que é o que vai no fio.
//
// A frase tem acento, então contar caracteres em Go daria outro número, e o painel corta
// em 94 bytes sem avisar.
func TestAFraseDaRecusaCabeNoPainel(t *testing.T) {
	t.Parallel()
	const limite = 94
	noFio := protocol.ClientText(msgGuildaSoChefeEscala)
	if len(noFio) > limite {
		t.Errorf("a recusa tem %d bytes no fio, limite %d: %q",
			len(noFio), limite, msgGuildaSoChefeEscala)
	}
	if msgGuildaSoChefeEscala == "" {
		t.Error("recusa vazia nao manda pacote nenhum, e a pessoa fica sem saber por que")
	}
}
