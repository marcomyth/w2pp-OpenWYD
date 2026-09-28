package protocol

import (
	"encoding/binary"
	"fmt"
)

// Painel de Guilda: a janela que reúne o que a guilda é, quem está nela, e o
// que ela pode fazer.
//
// O cliente do WYD nunca teve janela de guilda. Tudo o que existe de guilda no
// legado é comando de chat — /create, /subcreate, /expulsar, /convocar — e o
// servidor deste fork já sabe fazer todos eles (handler/guild.go). O que faltava
// era um lugar para ver. O painel é nosso (client/gamepatch/guilda.cpp), como a
// Loja de Honra, e por isso os pacotes também são.
//
// São três abas, e cada uma tem o seu par de pacotes, porque cada uma custa uma
// coisa diferente:
//
//	Informações  MsgGuildaAbre   (S→C)  sai da memória do servidor, é barata
//	Membros      MsgGuildaMembros(S→C)  vai ao BANCO, então só quando a aba abre
//	Buffs        MsgGuildaBuffs  (S→C)  sai da memória, muda com o tempo
//
// Separar as três é o ponto: a aba Membros lê a guilda inteira do banco,
// inclusive quem está desconectado, e ninguém quer pagar essa consulta toda vez
// que alguém espia a fama.
//
// Os buffs NÃO têm pacote de ativação, de propósito. Eles são ativados usando um
// item comprado por cash, pelo caminho normal de usar item — o painel só mostra
// o que está ligado e quanto falta. Assim não existe um pacote "me dê um buff"
// para um cliente remendado mandar.
const (
	// GuildaMembrosPorPagina é quantos membros cabem numa resposta. Uma guilda
	// vai a 250 (member_cap, 0079), e 250 linhas não cabem num pacote: a aba
	// pede página por página.
	//
	// TEM de bater com o cliente (kMembrosPorPagina em guildarede.cpp).
	GuildaMembrosPorPagina = 40

	// GuildaCidades é quantas cidades a lista de convocação tem: Armia, Azran,
	// Erion, Nippleheim e Noatum, a mesma ordem e o mesmo número das zonas de
	// guild_zone (0012) e de world.cities.
	GuildaCidades = 5

	// GuildaBuffs é quantos buffs o painel mostra.
	GuildaBuffs = 4

	// GuildaRecadoMax é o limite do recado, em bytes. O banco cobra 240
	// caracteres (guild_notice_len_check); aqui são bytes, e são mais, porque um
	// recado acentuado ocupa mais bytes do que caracteres. Quem tem a palavra
	// final é o banco.
	GuildaRecadoMax = 480

	// GuildaStatusMax é o limite da linha de status do membro, em bytes, contra
	// os 32 caracteres da coluna.
	GuildaStatusMax = 64

	// GuildaNomeMax é o campo de nome na linha: nome de personagem do WYD cabe
	// em 16, e é esse o tamanho em toda a struct de MOB.
	GuildaNomeMax = 16

	// GuildaListaMax é quantas guildas a tela "Guilds do Server" mostra. Um
	// servidor pode ter centenas; a tela mostra as de maior fama, que é a ordem
	// que responde "quem manda aqui" — a pergunta que se faz ao abrir a lista.
	//
	// TEM de bater com o cliente (kListaMax em guildarede.h).
	GuildaListaMax = 60
)

// Cargos, como o painel os nomeia. O número é o guild_level do banco, 0 a 9.
const (
	GuildaCargoMembro  uint8 = 0
	GuildaCargoSub     uint8 = 6
	GuildaCargoLider   uint8 = 9
	GuildaCargoMaximo  uint8 = 9
	GuildaCargoInvalid uint8 = 255
)

// GuildaCidade é uma linha da lista de cidades na aba Informações.
//
// Convocados é quantos membros da guilda estão AGORA dentro daquela cidade —
// não um histórico de quem foi convocado. É a resposta para "quanta gente eu
// tenho de prontidão ali", que é a pergunta que se faz antes de convocar.
type GuildaCidade struct {
	Zona       uint8
	Dona       bool  // a guilda de quem abriu o painel domina esta cidade
	Imposto    uint8 // 0..30, a taxa que a dona cobra
	Convocados int16
}

// GuildaAbreBody é o corpo de MsgGuildaAbre: a aba Informações inteira.
//
// Os nomes de aliada e de guerra vêm resolvidos, não como id: o cliente não tem
// registro de guildas e não teria como transformar 11 em REDDRAGONS. Vazio
// significa "não há".
type GuildaAbreBody struct {
	GuildaID   uint16
	Nome       string
	Lider      string
	Membros    int16 // a guilda inteira, do banco
	Online     int16 // quantos estão jogando agora
	Capacidade int16
	Fama       int32
	Aliada     string
	Guerra     string
	Recado     string
	RecadoPor  string
	RecadoEm   int64 // Unix; 0 = nunca houve recado
	MeuCargo   uint8 // o cargo de quem abriu, para a tela saber o que desenhar
	Cidades    [GuildaCidades]GuildaCidade
}

// guildaAbreFixo é o tanto de bytes de tamanho fixo do corpo, antes dos textos
// de tamanho variável.
//
//	2  GuildaID
//	16 Nome
//	16 Lider
//	2  Membros
//	2  Online
//	2  Capacidade
//	4  Fama
//	16 Aliada
//	16 Guerra
//	8  RecadoEm
//	1  MeuCargo
//	1  enchimento
//	5*6 Cidades (zona, dona, imposto, enchimento, convocados)
const guildaAbreFixo = 2 + GuildaNomeMax + GuildaNomeMax + 2 + 2 + 2 + 4 +
	GuildaNomeMax + GuildaNomeMax + 8 + 1 + 1 + GuildaCidades*6

// Encode serializa o corpo da aba Informações.
func (b *GuildaAbreBody) Encode() []byte {
	recado := cortaBytes(b.Recado, GuildaRecadoMax)
	por := cortaBytes(b.RecadoPor, GuildaNomeMax)

	// Os dois textos variáveis viajam com um tamanho na frente, porque o recado
	// é livre e não cabe num campo fixo sem desperdiçar 480 bytes em toda
	// abertura de painel.
	out := make([]byte, guildaAbreFixo+2+len(recado)+2+len(por))
	binary.LittleEndian.PutUint16(out[0:], b.GuildaID)
	escreveNome(out[2:], b.Nome)
	escreveNome(out[18:], b.Lider)
	binary.LittleEndian.PutUint16(out[34:], uint16(b.Membros))
	binary.LittleEndian.PutUint16(out[36:], uint16(b.Online))
	binary.LittleEndian.PutUint16(out[38:], uint16(b.Capacidade))
	binary.LittleEndian.PutUint32(out[40:], uint32(b.Fama))
	escreveNome(out[44:], b.Aliada)
	escreveNome(out[60:], b.Guerra)
	binary.LittleEndian.PutUint64(out[76:], uint64(b.RecadoEm))
	out[84] = b.MeuCargo
	// out[85] é enchimento
	p := 86
	for _, c := range b.Cidades {
		out[p] = c.Zona
		if c.Dona {
			out[p+1] = 1
		}
		out[p+2] = c.Imposto
		// out[p+3] é enchimento
		binary.LittleEndian.PutUint16(out[p+4:], uint16(c.Convocados))
		p += 6
	}
	binary.LittleEndian.PutUint16(out[p:], uint16(len(recado)))
	p += 2
	p += copy(out[p:], recado)
	binary.LittleEndian.PutUint16(out[p:], uint16(len(por)))
	p += 2
	copy(out[p:], por)
	return out
}

// DecodeGuildaAbre lê o corpo da aba Informações. Existe para os testes e para
// quem for reimplementar o cliente: o servidor só escreve.
func DecodeGuildaAbre(b []byte) (GuildaAbreBody, error) {
	var out GuildaAbreBody
	if len(b) < guildaAbreFixo+4 {
		return out, fmt.Errorf("protocol: guilda abre curto: %d", len(b))
	}
	out.GuildaID = binary.LittleEndian.Uint16(b[0:])
	out.Nome = cstr16(b[2:18])
	out.Lider = cstr16(b[18:34])
	out.Membros = int16(binary.LittleEndian.Uint16(b[34:]))
	out.Online = int16(binary.LittleEndian.Uint16(b[36:]))
	out.Capacidade = int16(binary.LittleEndian.Uint16(b[38:]))
	out.Fama = int32(binary.LittleEndian.Uint32(b[40:]))
	out.Aliada = cstr16(b[44:60])
	out.Guerra = cstr16(b[60:76])
	out.RecadoEm = int64(binary.LittleEndian.Uint64(b[76:]))
	out.MeuCargo = b[84]
	p := 86
	for i := range out.Cidades {
		out.Cidades[i] = GuildaCidade{
			Zona:       b[p],
			Dona:       b[p+1] != 0,
			Imposto:    b[p+2],
			Convocados: int16(binary.LittleEndian.Uint16(b[p+4:])),
		}
		p += 6
	}
	texto, p, err := leTexto(b, p, GuildaRecadoMax)
	if err != nil {
		return out, fmt.Errorf("protocol: guilda abre, recado: %w", err)
	}
	out.Recado = texto
	texto, _, err = leTexto(b, p, GuildaNomeMax)
	if err != nil {
		return out, fmt.Errorf("protocol: guilda abre, autor do recado: %w", err)
	}
	out.RecadoPor = texto
	return out, nil
}

// GuildaMembro é uma linha da aba Membros.
//
// Online é decidido pelo servidor no momento do envio: o cliente não tem como
// saber quem está conectado além de quem ele enxerga na tela.
type GuildaMembro struct {
	Nome    string
	Cargo   uint8
	Online  bool
	VistoEm int64 // Unix da última vez online; 0 = desconhecido
	Status  string
}

// GuildaMembrosBody é o corpo de MsgGuildaMembros: uma página do quadro.
//
// Pagina e Total viajam juntos para o cliente saber se ainda falta pedir. Total
// é o número de MEMBROS, não de páginas — é ele que a aba Informações mostra, e
// tê-lo aqui também evita que as duas abas discordem quando alguém sai da
// guilda entre um pedido e outro.
type GuildaMembrosBody struct {
	Pagina  uint8
	Total   int16
	Membros []GuildaMembro
}

// Encode serializa uma página do quadro de membros.
func (b *GuildaMembrosBody) Encode() []byte {
	membros := b.Membros
	if len(membros) > GuildaMembrosPorPagina {
		membros = membros[:GuildaMembrosPorPagina]
	}
	out := make([]byte, 0, 4+len(membros)*(GuildaNomeMax+12))
	cab := make([]byte, 4)
	cab[0] = b.Pagina
	cab[1] = uint8(len(membros))
	binary.LittleEndian.PutUint16(cab[2:], uint16(b.Total))
	out = append(out, cab...)

	for _, m := range membros {
		linha := make([]byte, GuildaNomeMax+12)
		escreveNome(linha, m.Nome)
		linha[GuildaNomeMax] = m.Cargo
		if m.Online {
			linha[GuildaNomeMax+1] = 1
		}
		// +2 e +3 são enchimento, para o Unix cair alinhado em 8
		binary.LittleEndian.PutUint64(linha[GuildaNomeMax+4:], uint64(m.VistoEm))
		out = append(out, linha...)

		status := cortaBytes(m.Status, GuildaStatusMax)
		tam := make([]byte, 2)
		binary.LittleEndian.PutUint16(tam, uint16(len(status)))
		out = append(out, tam...)
		out = append(out, status...)
	}
	return out
}

// DecodeGuildaMembros lê uma página do quadro de membros.
func DecodeGuildaMembros(b []byte) (GuildaMembrosBody, error) {
	var out GuildaMembrosBody
	if len(b) < 4 {
		return out, fmt.Errorf("protocol: guilda membros curto: %d", len(b))
	}
	out.Pagina = b[0]
	n := int(b[1])
	out.Total = int16(binary.LittleEndian.Uint16(b[2:]))
	if n > GuildaMembrosPorPagina {
		return out, fmt.Errorf("protocol: guilda membros, %d linhas numa página de %d", n, GuildaMembrosPorPagina)
	}
	p := 4
	for i := 0; i < n; i++ {
		if p+GuildaNomeMax+12 > len(b) {
			return out, fmt.Errorf("protocol: guilda membros, linha %d truncada", i)
		}
		m := GuildaMembro{
			Nome:    cstr16(b[p : p+GuildaNomeMax]),
			Cargo:   b[p+GuildaNomeMax],
			Online:  b[p+GuildaNomeMax+1] != 0,
			VistoEm: int64(binary.LittleEndian.Uint64(b[p+GuildaNomeMax+4:])),
		}
		p += GuildaNomeMax + 12
		status, np, err := leTexto(b, p, GuildaStatusMax)
		if err != nil {
			return out, fmt.Errorf("protocol: guilda membros, status da linha %d: %w", i, err)
		}
		m.Status = status
		p = np
		out.Membros = append(out.Membros, m)
	}
	return out, nil
}

// GuildaBuff é um dos buffs do painel.
//
// Restam é em SEGUNDOS, e é o servidor que conta: o painel só desenha o número
// que chegou, descontando o relógio local enquanto a janela fica aberta. Ativo
// com Restam zero não existe — é assim que a tela sabe apagar a linha sem
// precisar de um segundo pacote.
type GuildaBuff struct {
	Tipo   uint8
	Ativo  bool
	Restam int32
}

// GuildaItemDeBuff é um Guild Buff que o jogador tem na mochila.
//
// A lista vem do SERVIDOR junto com o estado dos buffs, e não é o cliente que a
// monta: ele não sabe ler o inventário — nem precisa aprender. O servidor já tem
// a mochila na mão, e mandá-la aqui evita um desvio novo no cliente só para ler
// uma coisa que o outro lado já sabe.
//
// Slot é a casa na mochila, e é por ela que a ativação se refere ao item; Dias é
// só para a caixa de escolha poder dizer "15 dias" sem ter tabela.
type GuildaItemDeBuff struct {
	Slot   int16
	Indice int16
	Dias   int16
}

// GuildaItensDeBuffMax é quantos itens a caixa de escolha mostra.
const GuildaItensDeBuffMax = 12

// GuildaBuffsBody é o corpo de MsgGuildaBuffs: o estado dos quatro e o que o
// jogador tem na mochila para acendê-los.
type GuildaBuffsBody struct {
	Buffs [GuildaBuffs]GuildaBuff
	Itens []GuildaItemDeBuff
}

const (
	guildaBuffsSize = GuildaBuffs * 8
	guildaItemSize  = 6
)

// Encode serializa o estado dos buffs e a lista de itens.
func (b *GuildaBuffsBody) Encode() []byte {
	itens := b.Itens
	if len(itens) > GuildaItensDeBuffMax {
		itens = itens[:GuildaItensDeBuffMax]
	}
	out := make([]byte, guildaBuffsSize+2+len(itens)*guildaItemSize)
	for i, bf := range b.Buffs {
		p := i * 8
		out[p] = bf.Tipo
		if bf.Ativo && bf.Restam > 0 {
			out[p+1] = 1
		}
		// p+2 e p+3 são enchimento
		binary.LittleEndian.PutUint32(out[p+4:], uint32(bf.Restam))
	}
	p := guildaBuffsSize
	binary.LittleEndian.PutUint16(out[p:], uint16(len(itens)))
	p += 2
	for _, it := range itens {
		binary.LittleEndian.PutUint16(out[p:], uint16(it.Slot))
		binary.LittleEndian.PutUint16(out[p+2:], uint16(it.Indice))
		binary.LittleEndian.PutUint16(out[p+4:], uint16(it.Dias))
		p += guildaItemSize
	}
	return out
}

// DecodeGuildaBuffs lê o estado dos buffs e a lista de itens.
func DecodeGuildaBuffs(b []byte) (GuildaBuffsBody, error) {
	var out GuildaBuffsBody
	if len(b) < guildaBuffsSize {
		return out, fmt.Errorf("protocol: guilda buffs curto: %d", len(b))
	}
	for i := range out.Buffs {
		p := i * 8
		out.Buffs[i] = GuildaBuff{
			Tipo:   b[p],
			Ativo:  b[p+1] != 0,
			Restam: int32(binary.LittleEndian.Uint32(b[p+4:])),
		}
	}
	p := guildaBuffsSize
	if p+2 > len(b) {
		return out, nil // corpo antigo, sem a lista: os buffs já valem
	}
	n := int(binary.LittleEndian.Uint16(b[p:]))
	p += 2
	if n > GuildaItensDeBuffMax {
		return out, fmt.Errorf("protocol: guilda buffs com %d itens, máximo %d", n, GuildaItensDeBuffMax)
	}
	for i := 0; i < n; i++ {
		if p+guildaItemSize > len(b) {
			return out, fmt.Errorf("protocol: guilda buffs, item %d truncado", i)
		}
		out.Itens = append(out.Itens, GuildaItemDeBuff{
			Slot:   int16(binary.LittleEndian.Uint16(b[p:])),
			Indice: int16(binary.LittleEndian.Uint16(b[p+2:])),
			Dias:   int16(binary.LittleEndian.Uint16(b[p+4:])),
		})
		p += guildaItemSize
	}
	return out, nil
}

// GuildaAtivaBody é o corpo de MsgGuildaAtiva: QUAL buff acender, e com QUAL
// item da mochila.
//
// Os dois viajam juntos porque o servidor confere os dois: que o tipo existe, e
// que a casa da mochila tem mesmo um Guild Buff. Um cliente remendado que mande
// uma casa vazia não acende nada.
type GuildaAtivaBody struct {
	Tipo uint8
	Slot int16
}

const guildaAtivaSize = 4

// Encode serializa o pedido de ativação.
func (b *GuildaAtivaBody) Encode() []byte {
	out := make([]byte, guildaAtivaSize)
	out[0] = b.Tipo
	binary.LittleEndian.PutUint16(out[2:], uint16(b.Slot))
	return out
}

// DecodeGuildaAtiva lê o pedido de ativação.
func DecodeGuildaAtiva(b []byte) (GuildaAtivaBody, error) {
	if len(b) < guildaAtivaSize {
		return GuildaAtivaBody{}, fmt.Errorf("protocol: guilda ativa curto: %d", len(b))
	}
	return GuildaAtivaBody{
		Tipo: b[0],
		Slot: int16(binary.LittleEndian.Uint16(b[2:])),
	}, nil
}

// GuildaPedeBody é o corpo dos pedidos do cliente: MsgGuildaPede e
// MsgGuildaConvoca.
//
// Um corpo só para os dois porque os dois pedem a mesma coisa — "faça isto nesta
// aba/cidade" — e um pedido do cliente nunca carrega valor: quem decide quanto
// custa, quem pode e o que sai é o servidor.
type GuildaPedeBody struct {
	// Aba: 0 Informações, 1 Membros, 2 Buffs. Na convocação, a zona da cidade.
	Alvo   uint8
	Pagina uint8
}

const guildaPedeSize = 2

// Encode serializa um pedido do cliente.
func (b *GuildaPedeBody) Encode() []byte {
	return []byte{b.Alvo, b.Pagina}
}

// DecodeGuildaPede lê um pedido do cliente.
func DecodeGuildaPede(b []byte) (GuildaPedeBody, error) {
	if len(b) < guildaPedeSize {
		return GuildaPedeBody{}, fmt.Errorf("protocol: guilda pede curto: %d", len(b))
	}
	return GuildaPedeBody{Alvo: b[0], Pagina: b[1]}, nil
}

// GuildaTextoBody é o corpo de MsgGuildaRecado e MsgGuildaStatus: um texto e
// nada mais.
type GuildaTextoBody struct {
	Texto string
}

// Encode serializa o texto com o tamanho na frente.
func (b *GuildaTextoBody) Encode(limite int) []byte {
	t := cortaBytes(b.Texto, limite)
	out := make([]byte, 2+len(t))
	binary.LittleEndian.PutUint16(out, uint16(len(t)))
	copy(out[2:], t)
	return out
}

// DecodeGuildaTexto lê o texto, recusando o que passar do limite em vez de
// cortar: cortar o recado de alguém pela metade e gravar assim é pior do que
// dizer não.
func DecodeGuildaTexto(b []byte, limite int) (GuildaTextoBody, error) {
	t, _, err := leTexto(b, 0, limite)
	if err != nil {
		return GuildaTextoBody{}, fmt.Errorf("protocol: guilda texto: %w", err)
	}
	return GuildaTextoBody{Texto: t}, nil
}

// leTexto lê um texto de tamanho variável em b a partir de p: dois bytes de
// tamanho e os bytes do texto. Devolve o texto e onde o próximo campo começa.
func leTexto(b []byte, p, limite int) (string, int, error) {
	if p+2 > len(b) {
		return "", p, fmt.Errorf("sem os 2 bytes de tamanho em %d", p)
	}
	n := int(binary.LittleEndian.Uint16(b[p:]))
	p += 2
	if n > limite {
		return "", p, fmt.Errorf("tamanho %d passa do limite %d", n, limite)
	}
	if p+n > len(b) {
		return "", p, fmt.Errorf("texto de %d bytes não cabe no que sobrou (%d)", n, len(b)-p)
	}
	return string(b[p : p+n]), p + n, nil
}

// cortaBytes corta um texto em no máximo `limite` BYTES sem partir um caractere no
// meio. Cortar por bytes cegamente deixaria meio "ç" no fim da linha, que o
// cliente desenha como lixo.
func cortaBytes(s string, limite int) []byte {
	if len(s) <= limite {
		return []byte(s)
	}
	corte := limite
	for corte > 0 && s[corte]&0xC0 == 0x80 {
		corte--
	}
	return []byte(s[:corte])
}

// escreveNome copia um nome num campo fixo de 16 bytes, com NUL no fim.
func escreveNome(dst []byte, nome string) {
	n := cortaBytes(nome, GuildaNomeMax-1)
	copy(dst[:GuildaNomeMax], make([]byte, GuildaNomeMax))
	copy(dst, n)
}

// GuildaDaLista é uma linha da tela "Guilds do Server".
type GuildaDaLista struct {
	ID      uint16
	Nome    string
	Lider   string
	Membros int16
	Fama    int32
}

// GuildaListaBody é o corpo de MsgGuildaLista.
//
// MinhaGuilda viaja junto para a tela poder destacar a linha de quem está
// olhando: numa lista de sessenta nomes, achar o próprio é a primeira coisa que
// se tenta fazer.
type GuildaListaBody struct {
	MinhaGuilda uint16
	Guildas     []GuildaDaLista
}

// guildaLinhaLista é o tamanho de uma linha: 2 id + 16 nome + 16 líder +
// 2 membros + 4 fama, mais 2 de enchimento para a fama cair alinhada.
const guildaLinhaLista = 2 + GuildaNomeMax + GuildaNomeMax + 2 + 2 + 4

// Encode serializa a lista de guildas.
func (b *GuildaListaBody) Encode() []byte {
	guildas := b.Guildas
	if len(guildas) > GuildaListaMax {
		guildas = guildas[:GuildaListaMax]
	}
	out := make([]byte, 4+len(guildas)*guildaLinhaLista)
	binary.LittleEndian.PutUint16(out[0:], b.MinhaGuilda)
	binary.LittleEndian.PutUint16(out[2:], uint16(len(guildas)))
	p := 4
	for _, g := range guildas {
		binary.LittleEndian.PutUint16(out[p:], g.ID)
		escreveNome(out[p+2:], g.Nome)
		escreveNome(out[p+2+GuildaNomeMax:], g.Lider)
		binary.LittleEndian.PutUint16(out[p+2+2*GuildaNomeMax:], uint16(g.Membros))
		// +2 de enchimento
		binary.LittleEndian.PutUint32(out[p+6+2*GuildaNomeMax:], uint32(g.Fama))
		p += guildaLinhaLista
	}
	return out
}

// DecodeGuildaLista lê a lista de guildas.
func DecodeGuildaLista(b []byte) (GuildaListaBody, error) {
	var out GuildaListaBody
	if len(b) < 4 {
		return out, fmt.Errorf("protocol: guilda lista curta: %d", len(b))
	}
	out.MinhaGuilda = binary.LittleEndian.Uint16(b[0:])
	n := int(binary.LittleEndian.Uint16(b[2:]))
	if n > GuildaListaMax {
		return out, fmt.Errorf("protocol: guilda lista com %d linhas, máximo %d", n, GuildaListaMax)
	}
	p := 4
	for i := 0; i < n; i++ {
		if p+guildaLinhaLista > len(b) {
			return out, fmt.Errorf("protocol: guilda lista, linha %d truncada", i)
		}
		out.Guildas = append(out.Guildas, GuildaDaLista{
			ID:      binary.LittleEndian.Uint16(b[p:]),
			Nome:    cstr16(b[p+2 : p+2+GuildaNomeMax]),
			Lider:   cstr16(b[p+2+GuildaNomeMax : p+2+2*GuildaNomeMax]),
			Membros: int16(binary.LittleEndian.Uint16(b[p+2+2*GuildaNomeMax:])),
			Fama:    int32(binary.LittleEndian.Uint32(b[p+6+2*GuildaNomeMax:])),
		})
		p += guildaLinhaLista
	}
	return out, nil
}

// GuildaNomeMaxCriar é o limite do nome de uma guilda nova, o mesmo
// guildNameMaxLen que o /create cobra.
const GuildaNomeMaxCriar = 16

// GuildaEsquadraMax é o teto de nomes numa escalação de cidade. Sessenta: mais
// do que qualquer cidade precisa numa guerra, e pouco o bastante para o pacote
// caber sem paginação.
//
// TEM de bater com o cliente (kEsquadraMax em guildarede.h) e com o dbServer.
const GuildaEsquadraMax = 60

// GuildaEsquadraBody é o corpo de MsgGuildaEsquadra e de MsgGuildaDesigna: uma
// cidade e os nomes escalados para ela.
//
// O MESMO corpo nos dois sentidos porque a conversa é a mesma nos dois: "esta
// cidade tem esta gente". O servidor manda para desenhar; o cliente manda para
// trocar. Um corpo por sentido seria dois lugares para errar o mesmo campo.
type GuildaEsquadraBody struct {
	Zona  uint8
	Nomes []string
}

// Encode serializa a escalação.
func (b *GuildaEsquadraBody) Encode() []byte {
	nomes := b.Nomes
	if len(nomes) > GuildaEsquadraMax {
		nomes = nomes[:GuildaEsquadraMax]
	}
	out := make([]byte, 2+len(nomes)*GuildaNomeMax)
	out[0] = b.Zona
	out[1] = uint8(len(nomes))
	for i, n := range nomes {
		escreveNome(out[2+i*GuildaNomeMax:], n)
	}
	return out
}

// DecodeGuildaEsquadra lê a escalação.
func DecodeGuildaEsquadra(b []byte) (GuildaEsquadraBody, error) {
	var out GuildaEsquadraBody
	if len(b) < 2 {
		return out, fmt.Errorf("protocol: guilda esquadra curta: %d", len(b))
	}
	out.Zona = b[0]
	n := int(b[1])
	if n > GuildaEsquadraMax {
		return out, fmt.Errorf("protocol: guilda esquadra com %d nomes, máximo %d", n, GuildaEsquadraMax)
	}
	if 2+n*GuildaNomeMax > len(b) {
		return out, fmt.Errorf("protocol: guilda esquadra truncada: %d nomes não cabem em %d bytes", n, len(b))
	}
	for i := 0; i < n; i++ {
		p := 2 + i*GuildaNomeMax
		if nome := cstr16(b[p : p+GuildaNomeMax]); nome != "" {
			out.Nomes = append(out.Nomes, nome)
		}
	}
	return out, nil
}

// As quatro ações do quadro de membros (0x0F4C). Os números são os do contrato
// fechado com quem cuida do cliente, e não podem mudar sem falar com eles.
const (
	GuildaAcaoPromove   uint8 = 1 // a sub-líder
	GuildaAcaoLideranca uint8 = 2 // passar a liderança
	GuildaAcaoExpulsa   uint8 = 3
	GuildaAcaoSai       uint8 = 4 // a única sobre si mesmo, e a única sem nome
)

// GuildaImpostoMax é o teto da taxa da cidade, em porcento.
const GuildaImpostoMax = 30

// GuildaAcaoBody é o 0x0F4C: uma ação do quadro sobre um membro.
type GuildaAcaoBody struct {
	Acao uint8
	Nome string
}

// DecodeGuildaAcao lê a ação.
//
// RECUSA AÇÃO DESCONHECIDA e nome vazio no que não é "sair". Uma ação fora da
// lista não pode virar a de número parecido, e um nome vazio nas outras três faria
// o comando nativo procurar alguém chamado "" — que em algum caminho é o próprio
// jogador.
func DecodeGuildaAcao(b []byte) (GuildaAcaoBody, error) {
	var out GuildaAcaoBody
	if len(b) < 1+GuildaNomeMax {
		return out, fmt.Errorf("protocol: guilda acao curta: %d", len(b))
	}
	out.Acao = b[0]
	if out.Acao < GuildaAcaoPromove || out.Acao > GuildaAcaoSai {
		return out, fmt.Errorf("protocol: guilda acao desconhecida: %d", out.Acao)
	}
	out.Nome = cstr16(b[1 : 1+GuildaNomeMax])
	if out.Acao != GuildaAcaoSai && out.Nome == "" {
		return out, fmt.Errorf("protocol: guilda acao %d sem nome", out.Acao)
	}
	return out, nil
}

// GuildaImpostoBody é o 0x0F4D: a taxa nova da cidade dominada.
type GuildaImpostoBody struct {
	// Zona viaja e é IGNORADA de propósito — ver o handler. Ela está aqui só para
	// o log poder dizer sobre o que era o pedido.
	Zona  uint8
	Ticks uint8
}

// DecodeGuildaImposto lê a taxa.
//
// O TETO É CONFERIDO AQUI E TAMBÉM NO COMANDO NATIVO. A repetição é barata e a
// falta dela não seria: sem esta, um valor de um byte (até 255) desceria até o
// comando como texto, e o que recusa lá é uma regra que pode mudar de dono amanhã.
func DecodeGuildaImposto(b []byte) (GuildaImpostoBody, error) {
	var out GuildaImpostoBody
	if len(b) < 2 {
		return out, fmt.Errorf("protocol: guilda imposto curto: %d", len(b))
	}
	out.Zona, out.Ticks = b[0], b[1]
	if out.Ticks > GuildaImpostoMax {
		return out, fmt.Errorf("protocol: guilda imposto %d acima do teto %d", out.Ticks, GuildaImpostoMax)
	}
	return out, nil
}
