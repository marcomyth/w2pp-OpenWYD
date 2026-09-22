package handler

import (
	"math"
	"testing"
	"time"

	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/protocol"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

// As duas durações que os itens dão, repetidas aqui para os testes não
// dependerem da tabela: se alguém trocar 15 por 20 dias na tabela, estes testes
// continuam medindo o que dizem medir.
const (
	quinzeDias = 15 * 24 * time.Hour
	trintaDias = 30 * 24 * time.Hour
)

// relogioFixo prende o relógio do despachante num instante, para os testes de
// duração não dependerem de quanto tempo o teste leva para rodar.
func relogioFixo(d *Dispatcher, t time.Time) func(time.Duration) {
	agora := t
	d.now = func() time.Time { return agora }
	return func(passo time.Duration) { agora = agora.Add(passo) }
}

// Um buff ligado vale pela duração da receita e some sozinho quando o relógio
// passa dela — sem depender do tick, porque quem consulta o bônus confere a hora.
func TestBuffDeGuildaValeEExpira(t *testing.T) {
	d, w := painelDeGuilda(t)
	avanca := relogioFixo(d, time.Date(2026, 9, 21, 20, 0, 0, 0, time.UTC))

	fim, ok := d.ligaBuffDeGuilda(w, guildaDeTeste, buffGuildaVida, quinzeDias)
	if !ok {
		t.Fatal("o buff não ligou")
	}
	e := liderDaGuilda(guildaDeTeste)

	if b := d.bonusDeBuffDeGuilda(e); b.vidaPc != 15 {
		t.Errorf("vida = %d%%, want 15%%", b.vidaPc)
	}

	// Um minuto antes de vencer ainda vale. A conta é contra o relógio PRESO do
	// despachante, não contra o do sistema: time.Until aqui mediria a distância
	// até uma data de 2026 a partir de hoje.
	avanca(fim.Sub(d.now()) - time.Minute)
	if b := d.bonusDeBuffDeGuilda(e); b.vidaPc != 15 {
		t.Errorf("um minuto antes do fim o buff já tinha caído: %+v", b)
	}
	// Depois do fim, não vale mais.
	avanca(2 * time.Minute)
	if b := d.bonusDeBuffDeGuilda(e); !b.vazio() {
		t.Errorf("o buff continuou valendo depois de vencer: %+v", b)
	}
}

// Usar um segundo item com o buff ligado SOMA o tempo em vez de reiniciá-lo: o
// item foi pago, e quem clica cedo não pode perder uma hora por isso.
func TestBuffDeGuildaSomaTempoEmVezDeReiniciar(t *testing.T) {
	d, w := painelDeGuilda(t)
	avanca := relogioFixo(d, time.Date(2026, 9, 21, 20, 0, 0, 0, time.UTC))

	primeiro, _ := d.ligaBuffDeGuilda(w, guildaDeTeste, buffGuildaVida, quinzeDias)
	avanca(10 * time.Minute)
	segundo, _ := d.ligaBuffDeGuilda(w, guildaDeTeste, buffGuildaVida, quinzeDias)

	if !segundo.After(primeiro) {
		t.Fatalf("o segundo item não somou: primeiro %v, segundo %v", primeiro, segundo)
	}
	if got := segundo.Sub(primeiro); got != quinzeDias {
		t.Errorf("somou %v, want %v (a duração do item)", got, quinzeDias)
	}
}

// O tempo acumulado tem teto: sem ele, vinte itens comprados viram um dia
// inteiro de buff, que é outro produto.
func TestBuffDeGuildaTemTeto(t *testing.T) {
	d, w := painelDeGuilda(t)
	inicio := time.Date(2026, 9, 21, 20, 0, 0, 0, time.UTC)
	relogioFixo(d, inicio)

	var fim time.Time
	for i := 0; i < 20; i++ {
		fim, _ = d.ligaBuffDeGuilda(w, guildaDeTeste, buffGuildaDano, trintaDias)
	}
	if got := fim.Sub(inicio); got > buffGuildaTetoAcumulado {
		t.Errorf("acumulou %v, acima do teto %v", got, buffGuildaTetoAcumulado)
	}
	if got := fim.Sub(inicio); got != buffGuildaTetoAcumulado {
		t.Errorf("acumulou %v, want exatamente o teto %v", got, buffGuildaTetoAcumulado)
	}
}

// Os quatro buffs somam juntos: nada aqui é exclusivo.
func TestBuffsDeGuildaSomamJuntos(t *testing.T) {
	d, w := painelDeGuilda(t)
	relogioFixo(d, time.Date(2026, 9, 21, 20, 0, 0, 0, time.UTC))

	for _, tipo := range []uint8{buffGuildaVida, buffGuildaDefesa, buffGuildaDano, buffGuildaDrop} {
		if _, ok := d.ligaBuffDeGuilda(w, guildaDeTeste, tipo, quinzeDias); !ok {
			t.Fatalf("buff %d não ligou", tipo)
		}
	}
	b := d.bonusDeBuffDeGuilda(liderDaGuilda(guildaDeTeste))
	if b.vidaPc != 15 || b.acPc != 18 || b.danoPc != 15 || b.drop != 20 {
		t.Errorf("soma dos quatro = %+v", b)
	}
}

// O buff é da GUILDA: quem não é dela não recebe nada, e quem não tem guilda
// tampouco.
func TestBuffDeGuildaSoValeParaAGuilda(t *testing.T) {
	d, w := painelDeGuilda(t)
	relogioFixo(d, time.Date(2026, 9, 21, 20, 0, 0, 0, time.UTC))
	d.ligaBuffDeGuilda(w, guildaDeTeste, buffGuildaVida, quinzeDias)

	deOutra := &world.Entity{ID: 2, Name: "Estranho", Guild: 99, HP: 100, Mode: world.MobUser}
	if b := d.bonusDeBuffDeGuilda(deOutra); !b.vazio() {
		t.Errorf("membro de outra guilda recebeu o buff: %+v", b)
	}
	semGuilda := &world.Entity{ID: 3, Name: "Solto", HP: 100, Mode: world.MobUser}
	if b := d.bonusDeBuffDeGuilda(semGuilda); !b.vazio() {
		t.Errorf("jogador sem guilda recebeu o buff: %+v", b)
	}
	if b := d.bonusDeBuffDeGuilda(nil); !b.vazio() {
		t.Errorf("entidade nula devolveu bônus: %+v", b)
	}
}

// O tick apaga o que venceu e limpa a linha da guilda do mapa, para ele não
// crescer com uma entrada por guilda que um dia usou um buff.
func TestTickApagaBuffVencidoELimpaOMapa(t *testing.T) {
	d, w := painelDeGuilda(t)
	avanca := relogioFixo(d, time.Date(2026, 9, 21, 20, 0, 0, 0, time.UTC))
	d.ligaBuffDeGuilda(w, guildaDeTeste, buffGuildaVida, quinzeDias)

	avanca(14 * 24 * time.Hour)
	d.tickBuffsDeGuilda(w)
	if _, ok := d.guildaBuffs[guildaDeTeste]; !ok {
		t.Fatal("o tick apagou um buff que ainda valia")
	}

	avanca(2 * 24 * time.Hour) // passou dos 15 dias
	d.tickBuffsDeGuilda(w)     // este apaga o vencido
	if b := d.bonusDeBuffDeGuilda(liderDaGuilda(guildaDeTeste)); !b.vazio() {
		t.Errorf("o bônus sobreviveu ao vencimento: %+v", b)
	}
	d.tickBuffsDeGuilda(w) // e este limpa a linha, já sem nada ligado
	if _, ok := d.guildaBuffs[guildaDeTeste]; ok {
		t.Error("a linha da guilda ficou no mapa sem nenhum buff ligado")
	}
}

// Um tipo que não existe não liga nada, e guilda zero também não: as duas coisas
// chegam do caminho de usar item, e nenhuma pode criar uma linha no mapa.
func TestBuffDeGuildaRecusaTipoEGuildaInvalidos(t *testing.T) {
	d, w := painelDeGuilda(t)
	relogioFixo(d, time.Date(2026, 9, 21, 20, 0, 0, 0, time.UTC))

	if _, ok := d.ligaBuffDeGuilda(w, guildaDeTeste, 250, quinzeDias); ok {
		t.Error("um tipo inexistente ligou um buff")
	}
	if _, ok := d.ligaBuffDeGuilda(w, 0, buffGuildaVida, quinzeDias); ok {
		t.Error("guilda zero ligou um buff")
	}
	if len(d.guildaBuffs) != 0 {
		t.Errorf("uma recusa criou %d linha(s) no mapa", len(d.guildaBuffs))
	}
}

// A aba Buffs mostra os quatro sempre, ligados ou não, e os segundos restantes
// arredondados para cima — um buff com meio segundo de vida não pode ser
// desenhado como desligado.
func TestCorpoDeBuffs(t *testing.T) {
	d, w := painelDeGuilda(t)
	avanca := relogioFixo(d, time.Date(2026, 9, 21, 20, 0, 0, 0, time.UTC))
	d.ligaBuffDeGuilda(w, guildaDeTeste, buffGuildaDano, quinzeDias)

	corpo := d.corpoDeBuffs(guildaDeTeste)
	if len(corpo.Buffs) != protocol.GuildaBuffs {
		t.Fatalf("a aba trouxe %d buffs, want %d", len(corpo.Buffs), protocol.GuildaBuffs)
	}
	for i, r := range receitasDeBuff {
		if corpo.Buffs[i].Tipo != r.Tipo {
			t.Errorf("posição %d = tipo %d, want %d", i, corpo.Buffs[i].Tipo, r.Tipo)
		}
	}
	dano := corpo.Buffs[2]
	if !dano.Ativo || dano.Restam != int32(quinzeDias.Seconds()) {
		t.Errorf("Buff de Dano = %+v", dano)
	}
	if corpo.Buffs[0].Ativo {
		t.Error("um buff que ninguém ligou saiu ativo")
	}

	// Meio segundo de vida ainda é ativo, com 1 segundo restante.
	avanca(quinzeDias - 500*time.Millisecond)
	dano = d.corpoDeBuffs(guildaDeTeste).Buffs[2]
	if !dano.Ativo || dano.Restam != 1 {
		t.Errorf("no último meio segundo o buff = %+v, want ativo com 1s", dano)
	}
}

// Uma guilda sem nenhum buff ainda recebe a aba inteira, com os quatro
// desligados: sem isto o painel não teria o que desenhar na primeira abertura.
func TestCorpoDeBuffsDeGuildaSemBuff(t *testing.T) {
	d, _ := painelDeGuilda(t)
	relogioFixo(d, time.Date(2026, 9, 21, 20, 0, 0, 0, time.UTC))
	corpo := d.corpoDeBuffs(guildaDeTeste)
	for i, b := range corpo.Buffs {
		if b.Ativo || b.Restam != 0 {
			t.Errorf("posição %d saiu ligada: %+v", i, b)
		}
		if b.Tipo == 0 {
			t.Errorf("posição %d saiu sem tipo", i)
		}
	}
}

// O bônus entra no score: vida, defesa e dano como percentual, drop como soma
// direta na escala própria dele.
func TestAplicaBuffDeGuilda(t *testing.T) {
	e := &world.Entity{AC: 1000, HpAddPct: 10, DanoFisicoPct: 5, EquipDropBonus: 16}
	aplicaBuffDeGuilda(e, bonusDeGuilda{vidaPc: 15, acPc: 18, danoPc: 15, drop: 20})

	if e.HpAddPct != 25 {
		t.Errorf("HpAddPct = %d, want 25 (soma aos 10 do equipamento)", e.HpAddPct)
	}
	if e.AC != 1180 {
		t.Errorf("AC = %d, want 1180 (1000 +18%%)", e.AC)
	}
	// O buff de dano paga os DOIS, e o físico soma aos 5 que já havia.
	if e.DanoFisicoPct != 20 || e.DanoMagicoPct != 15 {
		t.Errorf("dano = %d%% físico, %d%% mágico, want 20 e 15",
			e.DanoFisicoPct, e.DanoMagicoPct)
	}
	if e.EquipDropBonus != 36 {
		t.Errorf("drop = %d, want 36 (os 16 da fada mais os 20 do buff)", e.EquipDropBonus)
	}
}

// Um bônus vazio não pode mexer em nada — é o caso de 99% das chamadas, já que
// refreshScore roda a cada troca de equipamento.
func TestAplicaBuffDeGuildaVazioNaoMexe(t *testing.T) {
	e := &world.Entity{AC: 1000, HpAddPct: 10, EquipDropBonus: 16}
	aplicaBuffDeGuilda(e, bonusDeGuilda{})
	if e.AC != 1000 || e.HpAddPct != 10 || e.EquipDropBonus != 16 ||
		e.DanoFisicoPct != 0 || e.DanoMagicoPct != 0 {
		t.Errorf("um bônus vazio mexeu no score: %+v", bonusDeGuilda{
			vidaPc: int(e.HpAddPct), acPc: int(e.AC), danoPc: int(e.DanoFisicoPct), drop: int(e.EquipDropBonus),
		})
	}
	aplicaBuffDeGuilda(nil, bonusDeGuilda{vidaPc: 15}) // não pode estourar
}

// Os dois Tickets reaproveitados acendem os buffs, e a diferença entre eles é
// só o tempo. É por isso que o casamento é por ÍNDICE: os dois dividem o
// EF_VOLATILE 197, e pelo volátil não haveria como separar 15 dias de 30.
func TestOsDoisItensTemDuracoesDiferentes(t *testing.T) {
	if got := duracaoDoItemDeBuff(3439); got != quinzeDias {
		t.Errorf("3439 dá %v, want %v", got, quinzeDias)
	}
	if got := duracaoDoItemDeBuff(3440); got != trintaDias {
		t.Errorf("3440 dá %v, want %v", got, trintaDias)
	}
	for _, index := range []int16{0, 1, 2340, 2400, 3438, 3441, 4117, 5137} {
		if got := duracaoDoItemDeBuff(index); got != 0 {
			t.Errorf("o item %d caiu no caminho do buff de guilda (%v)", index, got)
		}
	}
}

// Um item acende UM buff, o que o jogador escolheu — e só ele.
//
// Começou diferente: um item acendia os quatro. A Josiel trocou em 21/09/2026 ao
// ver o resultado, e este teste é o que trava a regra nova.
func TestUmItemAcendeApenasOBuffEscolhido(t *testing.T) {
	d, w := painelDeGuilda(t)
	relogioFixo(d, time.Date(2026, 9, 21, 20, 0, 0, 0, time.UTC))

	s := &world.Session{Conn: 1, Mode: world.UserPlay}
	e := liderDaGuilda(guildaDeTeste)
	e.Carry[0] = world.Item{Index: 3440}

	d.acendeUmBuff(w, s, e, 0, buffGuildaDrop)

	b := d.bonusDeBuffDeGuilda(e)
	if b.drop != 20 {
		t.Errorf("o buff escolhido nao acendeu: %+v", b)
	}
	if b.vidaPc != 0 || b.acPc != 0 || b.danoPc != 0 {
		t.Errorf("acendeu buff que ninguem pediu: %+v", b)
	}
	if e.Carry[0].Index != 0 {
		t.Errorf("o item nao foi gasto: sobrou %d", e.Carry[0].Index)
	}
}

// Uma casa vazia, ou uma casa sem Guild Buff, nao acende nada e nao gasta nada.
// E o pedido que vem do cliente, entao e por aqui que um cliente remendado
// tentaria ganhar buff de graca.
func TestAcenderComCasaErradaNaoDaBuff(t *testing.T) {
	d, w := painelDeGuilda(t)
	relogioFixo(d, time.Date(2026, 9, 21, 20, 0, 0, 0, time.UTC))

	s := &world.Session{Conn: 1, Mode: world.UserPlay}
	e := liderDaGuilda(guildaDeTeste)
	e.Carry[5] = world.Item{Index: 2340} // uma cria de montaria, nao um buff

	d.acendeUmBuff(w, s, e, 5, buffGuildaVida)  // casa com outro item
	d.acendeUmBuff(w, s, e, 9, buffGuildaVida)  // casa vazia
	d.acendeUmBuff(w, s, e, -1, buffGuildaVida) // casa invalida
	d.acendeUmBuff(w, s, e, 9999, buffGuildaVida)

	if b := d.bonusDeBuffDeGuilda(e); !b.vazio() {
		t.Errorf("acendeu buff sem item: %+v", b)
	}
	if e.Carry[5].Index != 2340 {
		t.Error("gastou o item errado")
	}
}

// A lista que a aba Buffs manda e a mochila filtrada, com os dias de cada item.
func TestItensDeBuffNaMochila(t *testing.T) {
	e := liderDaGuilda(guildaDeTeste)
	e.Carry[2] = world.Item{Index: 3439}
	e.Carry[7] = world.Item{Index: 1030} // qualquer outra coisa
	e.Carry[9] = world.Item{Index: 3440}

	itens := itensDeBuffNaMochila(e)
	if len(itens) != 2 {
		t.Fatalf("achou %d itens, want 2", len(itens))
	}
	if itens[0].Slot != 2 || itens[0].Indice != 3439 || itens[0].Dias != 15 {
		t.Errorf("primeiro item = %+v", itens[0])
	}
	if itens[1].Slot != 9 || itens[1].Indice != 3440 || itens[1].Dias != 30 {
		t.Errorf("segundo item = %+v", itens[1])
	}
}

// Sem guilda o item NÃO é gasto. Queimar um item de cash sem entregar nada é o
// pior resultado possível.
func TestItemDeBuffNaoEGastoSemGuilda(t *testing.T) {
	d, w := painelDeGuilda(t)
	relogioFixo(d, time.Date(2026, 9, 21, 20, 0, 0, 0, time.UTC))

	s := &world.Session{Conn: 1, Mode: world.UserPlay}
	e := &world.Entity{ID: 1, Name: "Solto", HP: 100, Mode: world.MobUser}
	e.Carry[0] = world.Item{Index: 3439}

	d.acendeUmBuff(w, s, e, 0, buffGuildaVida)

	if e.Carry[0].Index != 3439 {
		t.Error("o item foi gasto por quem não tem guilda")
	}
	if len(d.guildaBuffs) != 0 {
		t.Error("acendeu buff sem guilda")
	}
}

// As quatro receitas têm de ter tipos distintos, na ordem das posições do
// pacote: o painel casa a posição com o tipo, e um furo aqui trocaria o efeito
// de dois buffs.
func TestReceitasDeBuffSaoConsistentes(t *testing.T) {
	if len(receitasDeBuff) != protocol.GuildaBuffs {
		t.Fatalf("%d receitas para %d posições no pacote", len(receitasDeBuff), protocol.GuildaBuffs)
	}
	for i, r := range receitasDeBuff {
		if int(r.Tipo) != i+1 {
			t.Errorf("posição %d tem tipo %d: o tipo tem de ser a posição mais um", i, r.Tipo)
		}
		if r.Nome == "" {
			t.Errorf("receita %d sem nome: %+v", i, r)
		}
		achada, ok := receitaDoBuff(r.Tipo)
		if !ok || achada.Nome != r.Nome {
			t.Errorf("receitaDoBuff(%d) não achou a própria receita", r.Tipo)
		}
	}
	if _, ok := receitaDoBuff(0); ok {
		t.Error("o tipo 0 achou receita")
	}
}

// Um buff de 30 dias TEM de atravessar um restart: ele foi comprado com cash, e
// uma manutenção no meio do mês não pode apagá-lo.
func TestBuffsDeGuildaVoltamDoBanco(t *testing.T) {
	d, _ := painelDeGuilda(t)
	agora := time.Date(2026, 9, 21, 20, 0, 0, 0, time.UTC)
	relogioFixo(d, agora)

	d.restauraBuffsDeGuilda([]world.GuildBuffRecord{
		{GuildID: guildaDeTeste, Type: buffGuildaVida, ExpiresAt: agora.Add(trintaDias)},
		{GuildID: guildaDeTeste, Type: buffGuildaDrop, ExpiresAt: agora.Add(time.Hour)},
		// Vencido: o banco filtra, mas o relógio dele não é o deste processo.
		{GuildID: guildaDeTeste, Type: buffGuildaDefesa, ExpiresAt: agora.Add(-time.Minute)},
		// Lixo que não pode criar linha nenhuma.
		{GuildID: 0, Type: buffGuildaVida, ExpiresAt: agora.Add(trintaDias)},
		{GuildID: guildaDeTeste, Type: 99, ExpiresAt: agora.Add(trintaDias)},
	})

	b := d.bonusDeBuffDeGuilda(liderDaGuilda(guildaDeTeste))
	if b.vidaPc != 15 || b.drop != 20 {
		t.Errorf("os buffs vivos não voltaram: %+v", b)
	}
	if b.acPc != 0 {
		t.Errorf("um buff vencido voltou à vida: %+v", b)
	}
	if _, ok := d.guildaBuffs[0]; ok {
		t.Error("guilda zero ganhou linha no mapa")
	}
}

func TestDuracaoEmTexto(t *testing.T) {
	cases := []struct {
		d    time.Duration
		quer string
	}{
		{trintaDias, "30 dias"},
		{quinzeDias, "15 dias"},
		{25 * time.Hour, "1 dia"},
		{3 * time.Hour, "3 hora(s)"},
		{90 * time.Second, "1 minuto(s)"},
		{time.Second, "1 minuto(s)"}, // nunca "0 minuto(s)"
	}
	for _, c := range cases {
		if got := duracaoEmTexto(c.d); got != c.quer {
			t.Errorf("duracaoEmTexto(%v) = %q, want %q", c.d, got, c.quer)
		}
	}
}

// A conta do percentual de vida NÃO pode estourar o int32.
//
// Este teste nasce de um personagem morto de verdade, em 21/09/2026: nível 350
// com 50.000.100 de vida máxima, um buff de guilda de +15%, e a multiplicação
// 50.000.100 × 115 passando do teto do int32. O resultado virava negativo, o
// corte de negativos o transformava em zero, e refreshScore então baixava a vida
// do jogador para zero — ele morria de pé e continuava morto depois de relogar,
// porque o zero era gravado.
//
// O caso não é exótico: qualquer equipamento com EF_HPADD faz a mesma conta.
func TestComPercentualNaoEstoura(t *testing.T) {
	// O caso exato que matou o personagem.
	const vidaEnorme = 50_000_100
	if got := comPercentual(vidaEnorme, 15); got <= 0 {
		t.Fatalf("comPercentual(%d, 15) = %d: a conta estourou e virou negativa", vidaEnorme, got)
	}
	if got := comPercentual(vidaEnorme, 15); got != 57_500_115 {
		t.Errorf("comPercentual(%d, 15) = %d, want 57500115", vidaEnorme, got)
	}
	// Bem acima do teto: satura em vez de dar a volta. Errar por um número é
	// ruim; errar por um SINAL mata alguém.
	if got := comPercentual(2_000_000_000, 100); got != math.MaxInt32 {
		t.Errorf("comPercentual saturou em %d, want %d", got, int32(math.MaxInt32))
	}
	// Os casos comuns continuam iguais.
	for _, c := range []struct{ base, pct, quer int32 }{
		{1000, 0, 1000},
		{1000, 15, 1150},
		{1000, -50, 500},
		{0, 15, 0},
	} {
		if got := comPercentual(c.base, c.pct); got != c.quer {
			t.Errorf("comPercentual(%d, %d) = %d, want %d", c.base, c.pct, got, c.quer)
		}
	}
}

// E a vida do jogador nunca pode ser zerada por um buff. Este e o teste que
// fecha o caminho inteiro: refreshScore com o buff ligado, numa vida grande.
func TestBuffDeVidaNaoMataOJogador(t *testing.T) {
	d, w := painelDeGuilda(t)
	relogioFixo(d, time.Date(2026, 9, 21, 20, 0, 0, 0, time.UTC))
	d.ligaBuffDeGuilda(w, guildaDeTeste, buffGuildaVida, quinzeDias)

	e := liderDaGuilda(guildaDeTeste)
	e.BaseMaxHP = 50_000_100
	e.MaxHP = 50_000_100
	e.HP = 50_000_100

	d.refreshScore(e)

	if e.HP <= 0 {
		t.Fatalf("o buff de vida matou o jogador: HP = %d", e.HP)
	}
	if m := effectiveMaxHP(e); m <= 0 {
		t.Errorf("vida maxima efetiva = %d", m)
	}
}

// A marca do buff entra no vetor de afetos quando ele acende e sai quando ele
// apaga — é ela que faz o ícone aparecer na barra do jogo.
func TestMarcaDoBuffEntraESaiDoVetorDeAfetos(t *testing.T) {
	d, w := painelDeGuilda(t)
	avanca := relogioFixo(d, time.Date(2026, 9, 21, 20, 0, 0, 0, time.UTC))
	e := liderDaGuilda(guildaDeTeste)

	// Sem buff, nada entra.
	if d.sincronizaAfetosDeGuilda(e) {
		t.Error("sincronizou sem buff nenhum")
	}

	d.ligaBuffDeGuilda(w, guildaDeTeste, buffGuildaDano, quinzeDias)
	if !d.sincronizaAfetosDeGuilda(e) {
		t.Fatal("a marca nao entrou")
	}
	achou := false
	for _, a := range e.Affect {
		if a.Type == tipoNaBarra[2] {
			achou = true
			if a.Time != buffDeGuildaSentinela {
				t.Errorf("Time = %d, want a sentinela %d", a.Time, buffDeGuildaSentinela)
			}
		}
	}
	if !achou {
		t.Errorf("a marca do buff de dano nao esta no vetor: %v", e.Affect[:6])
	}
	// Chamar de novo nao muda nada: sem isso o servidor mandaria o vetor de 268
	// bytes a cada refresh, dizendo que nada aconteceu.
	if d.sincronizaAfetosDeGuilda(e) {
		t.Error("sincronizou duas vezes a mesma coisa")
	}

	// O tempo do pacote sai do prazo de verdade, nao da sentinela.
	tk, nosso := d.tempoDoBuffNaBarra(e, tipoNaBarra[2])
	if !nosso {
		t.Fatal("o tipo nao foi reconhecido como nosso")
	}
	if querido := uint32(quinzeDias / (8 * time.Second)); tk != querido {
		t.Errorf("tempo = %d ticks, want %d (15 dias em ticks de 8s)", tk, querido)
	}

	// Vencido, a marca sai. Neste teste a entidade nao esta numa sessao, entao
	// quem a tira e a propria sincronizacao - dai ela devolver "mudou".
	avanca(quinzeDias + time.Minute)
	d.tickBuffsDeGuilda(w)
	if !d.sincronizaAfetosDeGuilda(e) {
		t.Error("a sincronizacao nao viu o buff vencido")
	}
	for _, a := range e.Affect {
		if a.Type == tipoNaBarra[2] {
			t.Error("a marca continua no vetor depois de vencer")
		}
	}
}

// Os tipos usados na barra nao podem ser nenhum dos que applyAffectScore trata:
// o efeito entraria duas vezes, uma pelo bonus e outra pelo afeto.
func TestTiposDaBarraNaoColidemComOsDoServidor(t *testing.T) {
	tratados := map[uint8]bool{}
	for _, v := range []uint8{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15,
		19, 25, 27, 28, 29, 34, 35, 36, 37, 42} {
		tratados[v] = true
	}
	vistos := map[uint8]bool{}
	for _, tipo := range tipoNaBarra {
		if tratados[tipo] {
			t.Errorf("o tipo %d e tratado por applyAffectScore: o efeito entraria duas vezes", tipo)
		}
		if tipo == 0 || tipo > 49 {
			t.Errorf("o tipo %d esta fora da faixa 1..49 que o cliente aceita", tipo)
		}
		if vistos[tipo] {
			t.Errorf("o tipo %d aparece duas vezes", tipo)
		}
		vistos[tipo] = true
	}
}

// O caminho do LOGIN tem de pôr a marca. Este teste nasce de um erro de
// processo: a edição que ligava sendLoginAffects à sincronização falhou em
// silêncio, eu segui achando que tinha entrado, e o resultado foi um servidor
// publicado onde nada aparecia na barra — sem nenhum sintoma no código.
func TestSincronizaCobreOsTresCaminhos(t *testing.T) {
	d, w := painelDeGuilda(t)
	relogioFixo(d, time.Date(2026, 9, 21, 20, 0, 0, 0, time.UTC))
	d.ligaBuffDeGuilda(w, guildaDeTeste, buffGuildaVida, quinzeDias)

	// 1. Login: quem entra com a guilda buffada ja ve o icone.
	e := liderDaGuilda(guildaDeTeste)
	if !d.sincronizaAfetosDeGuilda(e) {
		t.Error("login: a marca nao entrou")
	}

	// 2. Quem nao tem guilda nao ganha marca nenhuma.
	solto := &world.Entity{ID: 2, Name: "Solto", HP: 100, Mode: world.MobUser}
	if d.sincronizaAfetosDeGuilda(solto) {
		t.Error("quem nao tem guilda ganhou marca")
	}

	// 3. Trocar de guilda tira a marca da antiga.
	e.Guild = 99
	if !d.sincronizaAfetosDeGuilda(e) {
		t.Error("trocar de guilda nao tirou a marca")
	}
	for _, a := range e.Affect {
		if a.Type == tipoNaBarra[0] {
			t.Error("a marca da guilda antiga ficou")
		}
	}
}
