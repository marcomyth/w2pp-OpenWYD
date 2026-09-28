//go:build integration

// Testes de integração da posse da conta: quem pode entrar, quem tem de esperar, e
// como a posse volta quando o dono some.
//
//	W2PP_TEST_DSN=postgres://postgres:dev@localhost:5432/postgres go test -tags=integration ./internal/store/
package store

import (
	"errors"
	"testing"
	"time"

	"github.com/jeanluca/w2pp-openwyd/internal/domain"
)

// envelheceOBatimento empurra o último sinal de vida do dono para trás, que é como
// se simula um processo que parou de bater SEM dormir o teste.
func envelheceOBatimento(t *testing.T, s *Store, conta int64, atras time.Duration) {
	t.Helper()
	if _, err := s.pool.Exec(t.Context(),
		`UPDATE account SET dono_batimento = now() - $2::interval WHERE id = $1`, conta, atras); err != nil {
		t.Fatalf("envelhecendo o batimento: %v", err)
	}
}

// TestDoisProcessosNaMesmaConta: o segundo é recusado enquanto o primeiro está
// vivo, e entra depois que o save final do primeiro solta a posse.
func TestDoisProcessosNaMesmaConta(t *testing.T) {
	s, ctx := freshStore(t)
	const primeiro, segundo = int64(11), int64(12)
	conta := contaComPersonagem(ctx, t, s, "posse_dois", 100, 0)

	if err := s.TomarPosseDaConta(ctx, conta, primeiro); err != nil {
		t.Fatalf("a primeira tomada: %v", err)
	}
	if err := s.TomarPosseDaConta(ctx, conta, segundo); !errors.Is(err, ErrContaEmUso) {
		t.Fatalf("erro = %v, queria ErrContaEmUso — a conta está viva no primeiro", err)
	}
	// O primeiro tomar de novo passa: é relogin, ou um retry.
	if err := s.TomarPosseDaConta(ctx, conta, primeiro); err != nil {
		t.Fatalf("o dono não conseguiu retomar a própria conta: %v", err)
	}

	// O save de SAÍDA solta a posse na mesma transação.
	if err := s.SalvarPersonagemComCarga(ctx, conta,
		personagemDoPar("posse_dois_p", 100, nil), 0, nil, nil, nil, primeiro, 1, true); err != nil {
		t.Fatalf("o save de saída: %v", err)
	}
	if err := s.TomarPosseDaConta(ctx, conta, segundo); err != nil {
		t.Fatalf("depois do save de saída o segundo tinha de entrar: %v", err)
	}
}

// TestSaveDoMeioNaoSoltaAPosse: só o save de SAÍDA solta. Um save de meio de jogo
// que soltasse abriria a porta para outra execução no meio da partida.
func TestSaveDoMeioNaoSoltaAPosse(t *testing.T) {
	s, ctx := freshStore(t)
	const dono, outro = int64(21), int64(22)
	conta := contaComPersonagem(ctx, t, s, "posse_meio", 100, 0)

	if err := s.TomarPosseDaConta(ctx, conta, dono); err != nil {
		t.Fatal(err)
	}
	if err := s.SalvarPersonagemComCarga(ctx, conta,
		personagemDoPar("posse_meio_p", 100, nil), 0, nil, nil, nil, dono, 1, false); err != nil {
		t.Fatal(err)
	}
	if err := s.TomarPosseDaConta(ctx, conta, outro); !errors.Is(err, ErrContaEmUso) {
		t.Fatalf("erro = %v, queria ErrContaEmUso — o save do meio não solta", err)
	}
}

// TestSaveQueFalhaMantemAPosse: conta cujo último save não caiu é justamente a que
// ninguém deve carregar.
func TestSaveQueFalhaMantemAPosse(t *testing.T) {
	s, ctx := freshStore(t)
	const dono, outro = int64(31), int64(32)
	conta := contaComPersonagem(ctx, t, s, "posse_falha", 100, 0)
	if err := s.TomarPosseDaConta(ctx, conta, dono); err != nil {
		t.Fatal(err)
	}
	// Slot 3 não existe: o save inteiro volta atrás, e a soltura estava dentro dele.
	ch := personagemDoPar("fantasma", 0, nil)
	ch.Slot = 3
	if err := s.SalvarPersonagemComCarga(ctx, conta, ch, 0, nil, nil, nil, dono, 1, true); !errors.Is(err, ErrNotFound) {
		t.Fatalf("erro = %v, queria ErrNotFound", err)
	}
	if err := s.TomarPosseDaConta(ctx, conta, outro); !errors.Is(err, ErrContaEmUso) {
		t.Fatalf("erro = %v: o save falhou, então a posse tinha de FICAR", err)
	}
}

// TestPosseVenceSemBatimento: um processo que caiu sem soltar não pode trancar a
// conta para sempre.
func TestPosseVenceSemBatimento(t *testing.T) {
	s, ctx := freshStore(t)
	const morto, novo = int64(41), int64(42)
	conta := contaComPersonagem(ctx, t, s, "posse_vence", 0, 0)
	if err := s.TomarPosseDaConta(ctx, conta, morto); err != nil {
		t.Fatal(err)
	}
	// Ainda dentro do prazo: não vence.
	envelheceOBatimento(t, s, conta, PrazoDaPosse/2)
	if err := s.TomarPosseDaConta(ctx, conta, novo); !errors.Is(err, ErrContaEmUso) {
		t.Fatalf("erro = %v: dentro do prazo a posse tem de valer", err)
	}
	// Passou do prazo: vence.
	envelheceOBatimento(t, s, conta, PrazoDaPosse+10*time.Second)
	if err := s.TomarPosseDaConta(ctx, conta, novo); err != nil {
		t.Fatalf("depois do prazo a posse tinha de ser tomada: %v", err)
	}
}

// TestBatimentoRenovaEDizQuemAindaEMeu: o retorno é o ponto — quem não volta na
// lista deixou de ser meu.
func TestBatimentoRenovaEDizQuemAindaEMeu(t *testing.T) {
	s, ctx := freshStore(t)
	const meu, outro = int64(51), int64(52)
	minha := contaComPersonagem(ctx, t, s, "posse_bate_minha", 0, 0)
	perdida := contaComPersonagem(ctx, t, s, "posse_bate_perdida", 0, 0)
	if err := s.TomarPosseDaConta(ctx, minha, meu); err != nil {
		t.Fatal(err)
	}
	if err := s.TomarPosseDaConta(ctx, perdida, meu); err != nil {
		t.Fatal(err)
	}
	// A outra execução toma uma delas, como se o meu prazo tivesse vencido.
	envelheceOBatimento(t, s, perdida, PrazoDaPosse+time.Minute)
	if err := s.TomarPosseDaConta(ctx, perdida, outro); err != nil {
		t.Fatal(err)
	}

	aindaMinhas, err := s.BaterPelasContas(ctx, meu, []int64{minha, perdida})
	if err != nil {
		t.Fatalf("BaterPelasContas: %v", err)
	}
	if len(aindaMinhas) != 1 || aindaMinhas[0] != minha {
		t.Fatalf("ainda minhas = %v, queria só [%d]", aindaMinhas, minha)
	}
	// E o batimento renovou a que ficou: ela não vence agora.
	if err := s.TomarPosseDaConta(ctx, minha, outro); !errors.Is(err, ErrContaEmUso) {
		t.Errorf("erro = %v: o batimento tinha de ter renovado o prazo", err)
	}
}

// TestNinguemSoltaAPosseAlheia: a soltura sempre confere de quem é.
func TestNinguemSoltaAPosseAlheia(t *testing.T) {
	s, ctx := freshStore(t)
	const dono, intruso = int64(61), int64(62)
	conta := contaComPersonagem(ctx, t, s, "posse_alheia", 0, 0)
	if err := s.TomarPosseDaConta(ctx, conta, dono); err != nil {
		t.Fatal(err)
	}
	if err := s.SoltarPosseDaConta(ctx, conta, intruso); err != nil {
		t.Fatalf("SoltarPosseDaConta: %v", err)
	}
	if err := s.TomarPosseDaConta(ctx, conta, intruso); !errors.Is(err, ErrContaEmUso) {
		t.Fatalf("erro = %v: o intruso soltou a posse do dono", err)
	}
	// E o dono solta a dele.
	if err := s.SoltarPosseDaConta(ctx, conta, dono); err != nil {
		t.Fatal(err)
	}
	if err := s.TomarPosseDaConta(ctx, conta, intruso); err != nil {
		t.Fatalf("depois de o dono soltar, o outro tinha de entrar: %v", err)
	}
}

// TestSemEpocaNaoHaPosse: servidor sem numeração não disputa nada, e não pode
// travar login por isso.
func TestSemEpocaNaoHaPosse(t *testing.T) {
	s, ctx := freshStore(t)
	conta := contaComPersonagem(ctx, t, s, "posse_sem_epoca", 0, 0)
	for i := 0; i < 2; i++ {
		if err := s.TomarPosseDaConta(ctx, conta, 0); err != nil {
			t.Fatalf("tomada %d sem época: %v", i, err)
		}
	}
	var epoca *int64
	if err := s.pool.QueryRow(ctx, `SELECT dono_epoca FROM account WHERE id = $1`, conta).Scan(&epoca); err != nil {
		t.Fatal(err)
	}
	if epoca != nil {
		t.Errorf("época zero carimbou dono: %v", *epoca)
	}
}

// TestPosseDeContaQueNaoExiste: o login de uma conta apagada no meio do caminho
// tem de dizer NotFound, e não "em uso".
func TestPosseDeContaQueNaoExiste(t *testing.T) {
	s, ctx := freshStore(t)
	if err := s.TomarPosseDaConta(ctx, 999777, 1); !errors.Is(err, ErrNotFound) {
		t.Fatalf("erro = %v, queria ErrNotFound", err)
	}
}

var _ = domain.Item{}
