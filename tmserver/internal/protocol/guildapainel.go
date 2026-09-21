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

// GuildaBuffsBody é o corpo de MsgGuildaBuffs.
type GuildaBuffsBody struct {
	Buffs [GuildaBuffs]GuildaBuff
}

const guildaBuffsSize = GuildaBuffs * 8

// Encode serializa o estado dos buffs.
func (b *GuildaBuffsBody) Encode() []byte {
	out := make([]byte, guildaBuffsSize)
	for i, bf := range b.Buffs {
		p := i * 8
		out[p] = bf.Tipo
		if bf.Ativo && bf.Restam > 0 {
			out[p+1] = 1
		}
		// p+2 e p+3 são enchimento
		binary.LittleEndian.PutUint32(out[p+4:], uint32(bf.Restam))
	}
	return out
}

// DecodeGuildaBuffs lê o estado dos buffs.
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
	return out, nil
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
func (b *GuildaTextoBody) Encode(max int) []byte {
	t := cortaBytes(b.Texto, max)
	out := make([]byte, 2+len(t))
	binary.LittleEndian.PutUint16(out, uint16(len(t)))
	copy(out[2:], t)
	return out
}

// DecodeGuildaTexto lê o texto, recusando o que passar do limite em vez de
// cortar: cortar o recado de alguém pela metade e gravar assim é pior do que
// dizer não.
func DecodeGuildaTexto(b []byte, max int) (GuildaTextoBody, error) {
	t, _, err := leTexto(b, 0, max)
	if err != nil {
		return GuildaTextoBody{}, fmt.Errorf("protocol: guilda texto: %w", err)
	}
	return GuildaTextoBody{Texto: t}, nil
}

// leTexto lê um texto de tamanho variável em b a partir de p: dois bytes de
// tamanho e os bytes do texto. Devolve o texto e onde o próximo campo começa.
func leTexto(b []byte, p, max int) (string, int, error) {
	if p+2 > len(b) {
		return "", p, fmt.Errorf("sem os 2 bytes de tamanho em %d", p)
	}
	n := int(binary.LittleEndian.Uint16(b[p:]))
	p += 2
	if n > max {
		return "", p, fmt.Errorf("tamanho %d passa do limite %d", n, max)
	}
	if p+n > len(b) {
		return "", p, fmt.Errorf("texto de %d bytes não cabe no que sobrou (%d)", n, len(b)-p)
	}
	return string(b[p : p+n]), p + n, nil
}

// cortaBytes corta um texto em no máximo max BYTES sem partir um caractere no
// meio. Cortar por bytes cegamente deixaria meio "ç" no fim da linha, que o
// cliente desenha como lixo.
func cortaBytes(s string, max int) []byte {
	if len(s) <= max {
		return []byte(s)
	}
	corte := max
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
