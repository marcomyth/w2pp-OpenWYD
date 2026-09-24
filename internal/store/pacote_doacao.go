package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jeanluca/w2pp-openwyd/internal/pilha"
)

// Recusas previstas do pacote. Viajam como erro e são traduzidas em resultado pelo
// serviço, do mesmo jeito que as da chave Pix.
var (
	// ErrPacoteDesconhecido é um package_id que não está na tabela.
	//
	// RECUSA, e não "doação sem brinde". Um id desconhecido significa que o site e o
	// servidor discordam sobre o que está à venda, e creditar assim mesmo entregaria
	// créditos por um preço que ninguém conferiu. É melhor a compra falhar na hora,
	// com a pessoa ainda olhando a tela, do que ela pagar e receber o que ninguém
	// combinou.
	ErrPacoteDesconhecido = errors.New("store: pacote desconhecido")
	// ErrPacoteDesligado é um pacote que existe e saiu de venda.
	ErrPacoteDesligado = errors.New("store: pacote desligado")
	// ErrPacoteSoStaff é o pacote de teste pedido por quem não é staff.
	//
	// O site o esconde, e o servidor recusa de novo. Esconder na tela não é trava: a
	// tela é uma das portas, e quem chamar a RPC direto passa por ela.
	ErrPacoteSoStaff = errors.New("store: pacote reservado para a staff")
	// ErrPacoteDivergente é o pedido trazer preço ou créditos diferentes dos da
	// tabela.
	//
	// O que VALE são os da tabela; os do pedido são conferidos contra ela. A
	// requisição vem do BFF do site, que é nosso, e ainda assim se confere — porque
	// conferir é barato e destrocar item entregue não é. Um bug de arredondamento no
	// site, ou um pedido montado à mão, pararia aqui.
	ErrPacoteDivergente = errors.New("store: preco ou creditos divergem do pacote")
)

// PacoteDoacao é o que um pacote vende.
type PacoteDoacao struct {
	ID          string
	Credits     int32
	AmountCents int64
	SoStaff     bool
	Ativo       bool
	Itens       []ItemDoPacote
}

// ItemDoPacote é um brinde. A forma é a do payload da delivery_queue de propósito: é
// exatamente isso que vai ser enfileirado, e uma tradução no meio é onde um efeito se
// perde sem ninguém ver.
type ItemDoPacote struct {
	ItemIndex int32
	Eff1      uint8
	EffV1     uint8
	Eff2      uint8
	EffV2     uint8
	Eff3      uint8
	EffV3     uint8
}

// LerPacote devolve o pacote e os brindes dele.
//
// Não confere nada: quem confere é o ConferirPacote, separado porque a conferência
// precisa saber QUEM está comprando e a leitura não.
func (s *Store) LerPacote(ctx context.Context, id string) (PacoteDoacao, error) {
	var p PacoteDoacao
	err := s.pool.QueryRow(ctx, `
		SELECT id, credits, amount_cents, so_staff, ativo
		  FROM donate_pacote WHERE id = $1`, id).
		Scan(&p.ID, &p.Credits, &p.AmountCents, &p.SoStaff, &p.Ativo)
	if errors.Is(err, pgx.ErrNoRows) {
		return PacoteDoacao{}, fmt.Errorf("%q: %w", id, ErrPacoteDesconhecido)
	}
	if err != nil {
		return PacoteDoacao{}, fmt.Errorf("store: lendo o pacote %q: %w", id, err)
	}

	rows, err := s.pool.Query(ctx, `
		SELECT item_index, eff1, effv1, eff2, effv2, eff3, effv3
		  FROM donate_pacote_item WHERE pacote_id = $1 ORDER BY ordem, id`, id)
	if err != nil {
		return PacoteDoacao{}, fmt.Errorf("store: lendo os brindes de %q: %w", id, err)
	}
	defer rows.Close()
	for rows.Next() {
		var it ItemDoPacote
		if err := rows.Scan(&it.ItemIndex, &it.Eff1, &it.EffV1,
			&it.Eff2, &it.EffV2, &it.Eff3, &it.EffV3); err != nil {
			return PacoteDoacao{}, fmt.Errorf("store: lendo os brindes de %q: %w", id, err)
		}
		p.Itens = append(p.Itens, it)
	}
	if err := rows.Err(); err != nil {
		return PacoteDoacao{}, fmt.Errorf("store: lendo os brindes de %q: %w", id, err)
	}
	return p, nil
}

// ConferirPacote diz se esta conta pode comprar este pacote por este preço.
//
// A ORDEM DAS RECUSAS É A DA CAUSA MAIS PROVÁVEL, e não acidental: id errado é bug de
// integração, desligado é o pacote que saiu de venda entre a tela e o clique, só-staff
// é tentativa de comprar o que não é para vender, e divergência é a última porque ela
// só faz sentido depois de o pacote existir.
//
// ehStaff vem de quem chama porque a role mora na conta, e esta camada recebe o que já
// foi resolvido — do mesmo jeito que o RmtWebService recebe o account_id já resolvido
// pelo BFF.
func (s *Store) ConferirPacote(ctx context.Context, id string, ehStaff bool,
	creditsPedidos int32, centavosPedidos int64,
) (PacoteDoacao, error) {
	p, err := s.LerPacote(ctx, id)
	if err != nil {
		return PacoteDoacao{}, err
	}
	if !p.Ativo {
		return PacoteDoacao{}, fmt.Errorf("%q: %w", id, ErrPacoteDesligado)
	}
	if p.SoStaff && !ehStaff {
		return PacoteDoacao{}, fmt.Errorf("%q: %w", id, ErrPacoteSoStaff)
	}
	if creditsPedidos != p.Credits || centavosPedidos != p.AmountCents {
		// O log de quem chama diz o que divergiu; aqui não, porque este erro sobe até
		// uma resposta que o site mostra, e ela não precisa saber os números nossos.
		return PacoteDoacao{}, fmt.Errorf("%q: %w", id, ErrPacoteDivergente)
	}
	return p, nil
}

// EspacosNoBau conta quantos espaços do baú da conta os brindes deste pacote ocupam.
//
// EXISTE PARA A TELA DO SITE, que promete "ocupa N espaços" — e o N tem de vir DAQUI,
// porque a regra de empilhar é do servidor. O site não tem como saber que 64 Baús do
// Apoiador ocupam um espaço e 64 montarias ocupariam 64.
//
// Usa o pilha.Divide, que é a mesma função da entrega: assim o número prometido na
// página e o número que a entrega produz não podem divergir.
func (p PacoteDoacao) EspacosNoBau() int {
	total := 0
	for _, it := range p.Itens {
		total += len(pilha.Divide(int16(it.ItemIndex), quantidadeDoBrinde(it)))
	}
	return total
}

// quantidadeDoBrinde lê o EF_AMOUNT do brinde, ou 1 quando não há.
//
// Um brinde sem EF_AMOUNT é uma unidade: montaria, fada e poção não empilham e não
// carregam quantidade. Ler zero como zero faria o pacote entregar nada.
func quantidadeDoBrinde(it ItemDoPacote) int {
	for _, par := range [3][2]uint8{{it.Eff1, it.EffV1}, {it.Eff2, it.EffV2}, {it.Eff3, it.EffV3}} {
		if par[0] == pilha.EfAmount && par[1] > 0 {
			return int(par[1])
		}
	}
	return 1
}

// comAmount devolve o brinde com o EF_AMOUNT trocado pelo tamanho desta pilha.
//
// Existe porque uma quantidade grande vira MAIS DE UMA pilha: o teto é 120 por pilha, e
// 200 baús seriam 120 + 80. Hoje o maior pacote pede 64 e cabe numa, e escrever certo
// custa pouco — o modo de falhar do errado é entregar 120 no lugar de 200 e ninguém
// notar, porque a primeira pilha parece certa.
func comAmount(it ItemDoPacote, quantidade int) ItemDoPacote {
	q := uint8(quantidade)
	switch {
	case it.Eff1 == pilha.EfAmount:
		it.EffV1 = q
	case it.Eff2 == pilha.EfAmount:
		it.EffV2 = q
	case it.Eff3 == pilha.EfAmount:
		it.EffV3 = q
	}
	return it
}

// payloadDoBrinde monta o JSON que a delivery_queue guarda.
//
// SEM expires_at, e a ausência é a decisão: a duração dos brindes vai na forma NÃO
// INICIADA, como efeito, e começa a contar no primeiro uso. Um expires_at aqui faria
// uma montaria de 15 dias comprada numa sexta começar a gastar prazo na hora, e quem
// viajasse duas semanas receberia um item vencido.
func payloadDoBrinde(it ItemDoPacote) ([]byte, error) {
	return json.Marshal(itemPayload{
		ItemIndex: it.ItemIndex,
		Eff1:      it.Eff1, EffV1: it.EffV1,
		Eff2: it.Eff2, EffV2: it.EffV2,
		Eff3: it.Eff3, EffV3: it.EffV3,
	})
}

// ContaEhStaff diz se a conta tem cargo de staff, para os pacotes reservados.
//
// FALHA FECHADO, e isso é o desenho e não um detalhe: qualquer role que esta função não
// reconheça vira NÃO-STAFF. Errar para o lado de recusar deixa a dona do servidor sem
// comprar o pacote de teste dela, o que ela resolve falando comigo; errar para o outro
// lado deixa qualquer pessoa comprar R$ 500 de Rcoins por R$ 1,00.
//
// A lista é a mesma do ParseAccess do jogo (world/session.go), repetida aqui porque o
// Go não deixa o webserver importar o internal do tmserver — e é bom que não deixe. Se
// um cargo novo aparecer lá, ele cai em não-staff aqui até alguém acrescentar, que é o
// lado seguro de ficar desatualizado.
func (s *Store) ContaEhStaff(ctx context.Context, accountID int64) (bool, error) {
	var role string
	err := s.pool.QueryRow(ctx,
		`SELECT coalesce(role, '') FROM account WHERE id = $1`, accountID).Scan(&role)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, ErrNotFound
	}
	if err != nil {
		return false, fmt.Errorf("store: lendo o cargo da conta %d: %w", accountID, err)
	}
	switch role {
	case "admin", "moderator":
		return true, nil
	default:
		return false, nil
	}
}
