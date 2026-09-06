//go:build integration

// Integration tests for the item census, against a real PostgreSQL.
//
//	W2PP_TEST_DSN=postgres://postgres@localhost:5432/postgres go test -tags=integration ./internal/store/
//
// What earns a database here is the counting itself. The refine level is read
// out of whichever of three effect pairs happens to hold it, the three storage
// places are counted separately, and the comparison between two days is a FULL
// JOIN so an item that vanished still has a row. Every one of those is SQL, and
// every one of them is silently wrong rather than loudly broken when it is.
package store

import (
	"context"
	"testing"

	"github.com/jeanluca/w2pp-openwyd/internal/domain"
)

func storeCenso(t *testing.T) *Store {
	t.Helper()
	pool := testPool(t)
	if err := Migrate(context.Background(), pool); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	s := New(pool)
	limpar := func() {
		ctx := context.Background()
		_, _ = s.pool.Exec(ctx, `DELETE FROM item_census_meta`)
		_, _ = s.pool.Exec(ctx, `DELETE FROM item_census`)
		_, _ = s.pool.Exec(ctx, `DELETE FROM item`)
	}
	limpar()
	t.Cleanup(limpar)
	return s
}

// personagem seeds a character and returns its id.
func personagem(t *testing.T, s *Store, contaID int64, nome string) int64 {
	t.Helper()
	var id int64
	err := s.pool.QueryRow(context.Background(),
		`INSERT INTO character (account_id, slot, name) VALUES ($1, 0, $2) RETURNING id`,
		contaID, nome).Scan(&id)
	if err != nil {
		t.Fatalf("seed personagem %q: %v", nome, err)
	}
	return id
}

// item seeds one item row. par says which of the three effect pairs carries the
// effect, so the tests can prove the refine is found in any of them.
func item(t *testing.T, s *Store, kind string, contaID, charID *int64, slot, index int16, par int, ef, val int16) {
	t.Helper()
	campos := [][2]string{{"eff1", "effv1"}, {"eff2", "effv2"}, {"eff3", "effv3"}}
	c := campos[par]
	_, err := s.pool.Exec(context.Background(),
		`INSERT INTO item (owner_kind, account_id, character_id, slot, item_index, `+c[0]+`, `+c[1]+`)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		kind, contaID, charID, slot, index, ef, val)
	if err != nil {
		t.Fatalf("seed item %d: %v", index, err)
	}
}

func TestCensoContaPorIndiceERefino(t *testing.T) {
	ctx := context.Background()
	s := storeCenso(t)
	cID := conta(t, s, "censo_conta")
	pID := personagem(t, s, cID, "CensoHero")

	// A mesma espada refinada em três lugares diferentes, e o refino guardado
	// num par diferente em cada uma: se a busca olhasse só o primeiro par, duas
	// destas cairiam no refino 0 e o número ficaria errado sem nada reclamar.
	item(t, s, "char_equip", nil, &pID, 0, 1100, 0, domain.EffSanc, 11)
	item(t, s, "char_carry", nil, &pID, 1, 1100, 1, domain.EffSanc, 11)
	item(t, s, "account_cargo", &cID, nil, 0, 1100, 2, domain.EffSanc, 11)
	// E uma sem refino nenhum, que tem de virar linha separada.
	item(t, s, "char_carry", nil, &pID, 2, 1100, 0, 0, 0)

	run, contou, err := s.RecordCensus(ctx)
	if err != nil {
		t.Fatalf("RecordCensus: %v", err)
	}
	if !contou {
		t.Fatal("RecordCensus não contou na primeira chamada")
	}
	if run.Units != 4 || run.Kinds != 2 {
		t.Errorf("run = %d unidades / %d tipos, want 4 / 2", run.Units, run.Kinds)
	}
	if run.CountedAt.IsZero() {
		t.Error("contado_em vazio; a tela precisa da hora da foto")
	}

	linhas := lerCensoDeHoje(t, s)
	refinada, ok := linhas[11]
	if !ok {
		t.Fatalf("nenhuma linha no refino 11: %+v", linhas)
	}
	if refinada.Units != 3 {
		t.Errorf("refino 11 = %d unidades, want 3", refinada.Units)
	}
	if refinada.Equipped != 1 || refinada.Carried != 1 || refinada.Stored != 1 {
		t.Errorf("refino 11 por lugar = %d/%d/%d, want 1/1/1 (equipado/mochila/baú)",
			refinada.Equipped, refinada.Carried, refinada.Stored)
	}
	if simples := linhas[0]; simples.Units != 1 || simples.Carried != 1 {
		t.Errorf("refino 0 = %+v, want 1 unidade na mochila", simples)
	}
}

// A segunda chamada do dia tem de ser de graça: o serviço acorda de seis em
// seis horas justamente por não confiar num processo vivo por 24.
func TestCensoNaoContaDuasVezesNoMesmoDia(t *testing.T) {
	ctx := context.Background()
	s := storeCenso(t)
	cID := conta(t, s, "censo_repete")
	pID := personagem(t, s, cID, "CensoRepete")
	item(t, s, "char_carry", nil, &pID, 0, 1100, 0, domain.EffSanc, 9)

	if _, contou, err := s.RecordCensus(ctx); err != nil || !contou {
		t.Fatalf("primeira: contou=%v err=%v", contou, err)
	}
	// Mais um item entra no mundo depois da foto.
	item(t, s, "char_carry", nil, &pID, 1, 1100, 0, domain.EffSanc, 9)

	run, contou, err := s.RecordCensus(ctx)
	if err != nil {
		t.Fatalf("segunda: %v", err)
	}
	if contou {
		t.Error("contou de novo no mesmo dia")
	}
	// E devolveu a foto que existe, não uma vazia: quem chama registra a hora.
	if run.Units != 1 {
		t.Errorf("run = %d unidades, want 1 (a foto de antes, não uma recontagem)", run.Units)
	}
	if n := lerCensoDeHoje(t, s)[9].Units; n != 1 {
		t.Errorf("linha do dia = %d, want 1; a segunda chamada mexeu na foto", n)
	}
}

func TestCensoComparaDuasFotos(t *testing.T) {
	ctx := context.Background()
	s := storeCenso(t)
	cID := conta(t, s, "censo_cresce")
	pID := personagem(t, s, cID, "CensoCresce")

	// A foto de sete dias atrás, escrita à mão: são dois dias de história que o
	// teste não tem tempo de esperar.
	fotoAntiga(t, s, 7, []linhaFoto{
		{1100, 11, 2}, // vai crescer
		{1100, 0, 50}, // vai ficar igual
		{2200, 9, 4},  // vai sumir por completo
	})

	item(t, s, "char_carry", nil, &pID, 0, 1100, 0, domain.EffSanc, 11)
	item(t, s, "char_carry", nil, &pID, 1, 1100, 0, domain.EffSanc, 11)
	item(t, s, "char_carry", nil, &pID, 2, 1100, 0, domain.EffSanc, 11)
	item(t, s, "char_carry", nil, &pID, 3, 1100, 0, domain.EffSanc, 11)
	for i := int16(0); i < 50; i++ {
		item(t, s, "account_cargo", &cID, nil, i, 1100, 0, 0, 0)
	}
	if _, _, err := s.RecordCensus(ctx); err != nil {
		t.Fatalf("RecordCensus: %v", err)
	}

	cmp, err := s.CensusGrowth(ctx, CensusQuery{Dias: 7, Subiu: true})
	if err != nil {
		t.Fatalf("CensusGrowth: %v", err)
	}
	if cmp.Ate.Zero() || cmp.De.Zero() {
		t.Fatalf("faltou uma das pontas: de=%+v ate=%+v", cmp.De, cmp.Ate)
	}
	if dias := int(cmp.Ate.Day.Sub(cmp.De.Day).Hours() / 24); dias != 7 {
		t.Errorf("janela = %d dias, want 7", dias)
	}
	if len(cmp.Linha) != 2 {
		t.Fatalf("linhas = %d, want 2 (o que subiu e o que sumiu; o que não mudou fica de fora): %+v",
			len(cmp.Linha), cmp.Linha)
	}
	// Ordenado por crescimento: o que subiu vem primeiro.
	subiu := cmp.Linha[0]
	if subiu.Index != 1100 || subiu.Sanc != 11 || subiu.Delta != 2 || subiu.Units != 4 || subiu.Was != 2 {
		t.Errorf("primeira linha = %+v, want índice 1100 refino 11 de 2 para 4", subiu)
	}
	// FULL JOIN: o que sumiu não tem linha na foto de hoje e mesmo assim aparece.
	sumiu := cmp.Linha[1]
	if sumiu.Index != 2200 || sumiu.Units != 0 || sumiu.Was != 4 || sumiu.Delta != -4 {
		t.Errorf("última linha = %+v, want índice 2200 de 4 para 0", sumiu)
	}
}

func TestCensoOrdenaPeloQueSumiu(t *testing.T) {
	ctx := context.Background()
	s := storeCenso(t)
	cID := conta(t, s, "censo_some")
	pID := personagem(t, s, cID, "CensoSome")

	fotoAntiga(t, s, 7, []linhaFoto{{1100, 11, 1}, {2200, 9, 30}})
	item(t, s, "char_carry", nil, &pID, 0, 1100, 0, domain.EffSanc, 11)
	item(t, s, "char_carry", nil, &pID, 1, 1100, 0, domain.EffSanc, 11)
	if _, _, err := s.RecordCensus(ctx); err != nil {
		t.Fatalf("RecordCensus: %v", err)
	}

	cmp, err := s.CensusGrowth(ctx, CensusQuery{Dias: 7, Subiu: false})
	if err != nil {
		t.Fatalf("CensusGrowth: %v", err)
	}
	if len(cmp.Linha) == 0 || cmp.Linha[0].Index != 2200 {
		t.Fatalf("primeira linha = %+v, want a que mais sumiu (2200)", cmp.Linha)
	}
	if cmp.Linha[0].Delta != -30 {
		t.Errorf("delta = %d, want -30", cmp.Linha[0].Delta)
	}
}

func TestCensoSoRefinadoTiraORuido(t *testing.T) {
	ctx := context.Background()
	s := storeCenso(t)
	cID := conta(t, s, "censo_refino")
	pID := personagem(t, s, cID, "CensoRefino")

	fotoAntiga(t, s, 7, []linhaFoto{{1100, 0, 10}, {1100, 11, 1}})
	for i := int16(0); i < 40; i++ {
		item(t, s, "char_carry", nil, &pID, i, 1100, 0, 0, 0)
	}
	item(t, s, "char_carry", nil, &pID, 90, 1100, 0, domain.EffSanc, 11)
	item(t, s, "char_carry", nil, &pID, 91, 1100, 0, domain.EffSanc, 11)
	if _, _, err := s.RecordCensus(ctx); err != nil {
		t.Fatalf("RecordCensus: %v", err)
	}

	cmp, err := s.CensusGrowth(ctx, CensusQuery{Dias: 7, Subiu: true, SoRefinado: true})
	if err != nil {
		t.Fatalf("CensusGrowth: %v", err)
	}
	// Sem o filtro, as trinta unidades comuns a mais afogariam a única linha que
	// interessa — que é a razão de o filtro existir.
	if len(cmp.Linha) != 1 || cmp.Linha[0].Sanc != 11 {
		t.Fatalf("linhas = %+v, want só a do refino 11", cmp.Linha)
	}
}

// Com menos história do que a janela pedida, a comparação cai para a foto mais
// antiga que existe. Quem chama precisa conseguir perceber isso: senão a tela
// diz "cresceu 40 em 30 dias" quando cresceu 40 em dois.
func TestCensoCaiParaAFotoMaisAntiga(t *testing.T) {
	ctx := context.Background()
	s := storeCenso(t)
	cID := conta(t, s, "censo_curto")
	pID := personagem(t, s, cID, "CensoCurto")

	fotoAntiga(t, s, 2, []linhaFoto{{1100, 11, 1}})
	item(t, s, "char_carry", nil, &pID, 0, 1100, 0, domain.EffSanc, 11)
	item(t, s, "char_carry", nil, &pID, 1, 1100, 0, domain.EffSanc, 11)
	if _, _, err := s.RecordCensus(ctx); err != nil {
		t.Fatalf("RecordCensus: %v", err)
	}

	cmp, err := s.CensusGrowth(ctx, CensusQuery{Dias: 30, Subiu: true})
	if err != nil {
		t.Fatalf("CensusGrowth: %v", err)
	}
	if dias := int(cmp.Ate.Day.Sub(cmp.De.Day).Hours() / 24); dias != 2 {
		t.Errorf("janela real = %d dias, want 2 (só existem duas fotos)", dias)
	}
	if len(cmp.Linha) != 1 || cmp.Linha[0].Delta != 1 {
		t.Errorf("linhas = %+v, want uma com delta 1", cmp.Linha)
	}
}

// Banco sem foto nenhuma devolve vazio sem erro: é o estado de um servidor que
// acabou de subir, não uma falha.
func TestCensoSemFotoNaoEhErro(t *testing.T) {
	s := storeCenso(t)
	cmp, err := s.CensusGrowth(context.Background(), CensusQuery{Dias: 7})
	if err != nil {
		t.Fatalf("CensusGrowth: %v", err)
	}
	if !cmp.Ate.Zero() || len(cmp.Linha) != 0 {
		t.Errorf("cmp = %+v, want vazio", cmp)
	}
}

func TestCensoHistoricoDeUmItem(t *testing.T) {
	ctx := context.Background()
	s := storeCenso(t)
	cID := conta(t, s, "censo_hist")
	pID := personagem(t, s, cID, "CensoHist")

	fotoAntiga(t, s, 2, []linhaFoto{{1100, 11, 5}})
	fotoAntiga(t, s, 1, []linhaFoto{{1100, 11, 6}})
	item(t, s, "char_carry", nil, &pID, 0, 1100, 0, domain.EffSanc, 11)
	if _, _, err := s.RecordCensus(ctx); err != nil {
		t.Fatalf("RecordCensus: %v", err)
	}

	pontos, err := s.CensusHistory(ctx, 1100, 11, 30)
	if err != nil {
		t.Fatalf("CensusHistory: %v", err)
	}
	if len(pontos) != 3 {
		t.Fatalf("pontos = %d, want 3", len(pontos))
	}
	// Mais novo primeiro, que é a ordem em que a tendência se lê.
	if pontos[0].Units != 1 || pontos[1].Units != 6 || pontos[2].Units != 5 {
		t.Errorf("série = %d,%d,%d, want 1,6,5 (do mais novo para o mais velho)",
			pontos[0].Units, pontos[1].Units, pontos[2].Units)
	}
}

// --- apoio ---

type linhaFoto struct {
	index    int16
	sanc     int16
	unidades int
}

// fotoAntiga writes a snapshot dated diasAtras days ago. Waiting for real days
// to pass is not an option, and the alternative — moving the clock — would test
// the test harness instead of the query.
func fotoAntiga(t *testing.T, s *Store, diasAtras int, linhas []linhaFoto) {
	t.Helper()
	ctx := context.Background()
	total := 0
	for _, l := range linhas {
		if _, err := s.pool.Exec(ctx, `
			INSERT INTO item_census (dia, item_index, sanc, unidades, mochila)
			VALUES (current_date - $1::int, $2, $3, $4, $4)`,
			diasAtras, l.index, l.sanc, l.unidades); err != nil {
			t.Fatalf("foto antiga %d: %v", l.index, err)
		}
		total += l.unidades
	}
	if _, err := s.pool.Exec(ctx, `
		INSERT INTO item_census_meta (dia, contado_em, unidades, variedades)
		VALUES (current_date - $1::int, now() - make_interval(days => $1), $2, $3)`,
		diasAtras, total, len(linhas)); err != nil {
		t.Fatalf("meta antiga: %v", err)
	}
}

// lerCensoDeHoje returns today's rows keyed by refine level.
func lerCensoDeHoje(t *testing.T, s *Store) map[int16]domain.ItemCensus {
	t.Helper()
	rows, err := s.pool.Query(context.Background(), `
		SELECT item_index, sanc, unidades, equipados, mochila, bau
		  FROM item_census WHERE dia = current_date`)
	if err != nil {
		t.Fatalf("ler censo: %v", err)
	}
	defer rows.Close()
	out := map[int16]domain.ItemCensus{}
	for rows.Next() {
		var c domain.ItemCensus
		if err := rows.Scan(&c.Index, &c.Sanc, &c.Units, &c.Equipped, &c.Carried, &c.Stored); err != nil {
			t.Fatalf("scan censo: %v", err)
		}
		out[c.Sanc] = c
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterar censo: %v", err)
	}
	return out
}
