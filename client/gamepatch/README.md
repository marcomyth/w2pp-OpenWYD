# GamePatch.dll

O visual do tooltip de montaria no cliente WYD 7662. Os outros itens não mudam.

- **Paleta das linhas:** título `#E0F7FF`, level necessário `#6E7C8C`, dano
  `#FF9E64`, imunidade e evasão `#FDE68A`, ataque mágico `#A78BFA`, absorção
  PvP/PvE `#5EEAD4`, vitalidade/HP/level/ração `#CBD5E1`, preço `#7DD3FC`.
  Aviso vermelho do cliente (level insuficiente etc.) continua vermelho.
- **Fundo** rgb(12,18,25) com **cantos redondos** e **borda de 5 px** na cor da
  raridade, com um reflexo que dá a volta a cada 2,4 s.
- **Linha "Montaria nível X"** no fim do tooltip, na cor da raridade.

## Raridade: `GamePatch.txt`

Na pasta do cliente, uma montaria por linha (texto em Windows-1252, `#` comenta):

```
Andaluz B = Divina, dourado
```

Nome como aparece no título do tooltip; depois do `=`, o nome do nível e a
família de cor da borda: `cinza`, `verde`, `azul`, `roxo`, `dourado`, `laranja`
ou `vermelho`. Montaria fora do arquivo fica com borda cinza e sem linha de nível.

## Raridade dos equipamentos: `GamePatchItens.bin`

Armas e armaduras ganham o mesmo fundo e borda, com a linha "Item nível X". O
nível de cada item vem de `GamePatchItens.bin`, que o gerador escreve a partir
do ItemList (regra em `webserver/internal/clientrarity`):

| Nível | Borda | Itens |
|---|---|---|
| Divino | dourado | todo Ancient (grade 5–8) |
| Mítico | vermelho | Celestial (`EF_MOBTYPE 3`) |
| Lendário | laranja | (Le) e Arch (`EF_MOBTYPE 1`) |
| Épico | roxo | armadura (A); arma sem letra lv 200+ |
| Raro | azul | armadura (M); arma lv 150–199 |
| Incomum | verde | arma lv 100–149 |
| Comum | cinza | armadura (N); arma até lv 99 |

Acessórios (anel, amuleto, orbe, pedra, familiar, capa) e consumíveis não têm
letra no catálogo: vão por grupo, na lista de `rules.go` do mesmo pacote. Item
que nenhum grupo pega fica com o tooltip do cliente.

A refinação ergue o nível — de equipamento e acessório, nunca de consumível —,
e nunca o baixa: +11 e +12 no mínimo Épico, +13
Lendário, +14 Mítico, +15 Divino. Os pisos vão no cabeçalho do arquivo, e o DLL
lê a refinação do item sob o mouse como `BASE_GetItemSanc`.

As cores das linhas continuam as do cliente nos equipamentos; a paleta é só
das montarias.

## Moldura do slot

Todo slot de item com raridade (bolsa, equipamento, loja, baú) ganha um fundo
na cor de slot do nível, atrás do ícone, e uma moldura de 2 px com relevo por
cima. O DLL troca o `Render` do controle de slot (vtable `0x5F4FF4`, entrada
`+0x58`, `0x40DD40`) e entrega os próprios nós antes e depois do ícone; o
brilho de gema que o cliente já põe no item +10 é da malha 3D e continua lá.
Montaria na bolsa usa o `GamePatch.txt`, pelo nome do catálogo em memória
(`0xFB9608`).

## Como o desenho funciona

O tooltip é o painel `0x102` da janela; o cliente o desenha como um nó de cor
sólida. O DLL troca o `Render` da classe do painel e, no lugar do retângulo do
cliente, encadeia cópias desse nó: primeiro um arredondado cheio com as cores da
borda, depois o arredondado de dentro com o fundo, por cima. Cada pedaço passa
1 px do vizinho, sempre para dentro, então não há junta que abra.

O nó tem **0x16C bytes** (construtor em `0x40BF80`). A cópia precisa ser inteira:
o desenho lê o índice de textura em `+0x160`, e uma cópia curta faz o último
pedaço ser tratado como textura 0 e não aparecer.

## Descrição na janela de skills

O tooltip da janela de skills só tem as linhas fixas do cliente (alcance, mana,
dano, propriedades, pontos, classe, "Skill Passiva"). O DLL escreve nas linhas
livres abaixo delas a descrição do livro da skill (item 5000–5447), lida do
próprio `itemhelp.dat` do cliente: texto novo de skill é só o `itemhelp.dat`.
A skill da janela é um slot com o item do livro em `+0x670`, o mesmo campo que
o desvio do slot já lê. No livro da bolsa o cliente já mostra essas linhas, e
o DLL não repete.

## Como o cliente carrega

O `WYD.exe` já tem um carregador no ponto de entrada (`0x5F3C66`) que chama
`LoadLibraryA` para `GamePatch.dll`, `Shield.dll` e `ClientPatch.dll`, nessa
ordem, antes de entrar no jogo. Nenhum dos dois primeiros existia, então basta
este arquivo estar na pasta do cliente: o exe e o `ClientPatch.dll` ficam como
estão. (O código do `ClientPatch` em `Source/Code/ClientPatch_v7662` não é
exatamente o que está compilado no cliente, por isso ele não é recompilado.)

## Como compilar

Precisa do Visual Studio 2022 Build Tools com C++:

```
client\gamepatch\build.bat
```

Sai em `client\gamepatch\out\GamePatch.dll`: 32 bits, runtime estático, sem
dependência além do `KERNEL32.dll`.

## Como publicar

Junto com os arquivos das montarias, pelo gerador:

```
go run ./webserver/cmd/montariacliente -tabela montarias-cliente.txt ^
    -cliente "<pasta do cliente original>" -catalogo Release\Common\ItemList.csv ^
    -gamepatch client\gamepatch\out\GamePatch.dll
```

O gerador põe na pasta de saída o `WYD.exe`, o `ItemList.bin`, o
`UI\strdef.bin`, o `GamePatchItens.bin` e este DLL, prontos para o launcher.
Com `-catalogo`, o `ItemList.bin` é reescrito a partir do catálogo do servidor
(`webserver/internal/clientitemlist`) — o cliente passa a mostrar o que o
servidor faz. O `GamePatch.txt` ainda é escrito à mão; a raridade ainda não
está no painel.

## Segurança

Antes de gravar qualquer coisa na memória, o DLL confere os bytes da função do
tooltip. Se não forem os da build 7662, ele não faz nada: tooltip sem cor, em
vez de jogo caindo.

## Contador de quest em campos novos (`timerfields.cpp`)

O `MsgStartTime` (0x3A1) liga o contador de tempo da Água e do Pesadelo, mas o
`WYD.exe` só o desenha numa lista fixa de 15 campos de 128x128 do mapa (laço em
`0x47DAA4`). Fora deles, ele esconde o contador e zera a flag. O DLL desvia o
primeiro par da lista (`0x47DACA`) e acrescenta:

| Campo | Onde |
|---|---|
| (19,16) | Castelo Orc de Erion (x 2432–2559, y 2048–2175) |
| (20,15) | Acampamento Troll (x 2560–2687, y 1920–2047) |
| (18,16) | Arena da Quest 256: Cemitério, do Coveiro (x 2304–2431, y 2048–2175) |
| (17,13) | Arena da Quest 256: Jardim dos Deuses, do Jardineiro (x 2176–2303, y 1664–1791) |
| (3,30) | Arena da Quest 256: Coração do Kaizen (x 384–511, y 3840–3967) |
| (5,29) | Arena da Quest 256: Hidras (x 640–767, y 3712–3839) |
| (10,31) | Arena da Quest 256: Elfos (x 1280–1407, y 3968–4095) |

Campo novo: mais um par de `cmp`/`jne` em `FieldHook`, e o servidor tem de
mandar o 0x3A1 a quem chega nele (as arenas mandam na entrada, em
`teleportQuest256Step`). O desvio se instala sozinho, por um objeto global, e
confere os 16 bytes antes de gravar, como os outros.

## Porcentagem dos acessórios no dano de skill (`acessoriopct.cpp`)

Desde 16/09 o Hércules dá % de dano físico (efeito 89) e o Hecate % de dano
mágico (efeito 90). O servidor aplica as duas dentro da conta de dano de skill,
mas o `WYD.exe` tem a própria cópia dessa conta (`0x542AA7`), que é o "Atq
Mágico" da janela C e o dano dos tooltips de skill. Sem o DLL, trocar um Brinco
de Hecate +9 por um +15 não muda a janela, embora o golpe suba.

O DLL desvia dois pontos, na mesma ordem e com a mesma conta inteira do servidor
(`tmserver/internal/combat/skill.go`):

| Endereço | Ramo | Conta |
|---|---|---|
| `0x542FCC` | sem Magia (2ª árvore do TK, Huntress) | dano × (100 + físico) / 100, antes do 5/4 |
| `0x542FF4` | com Magia | depois de (4×Magia+100), dano × (100 + mágico) / 100, antes do 5/4 |

A % é a soma do efeito no equipamento (montaria fora), lida com a mesma função
do tooltip (`0x53821E`), que já aplica o refino: o +15 conta 32%, como o
tooltip mostra. O "Ataque" da janela não passa por aqui: ele vem do servidor, que
já soma o Hércules. As poções continuam fora da janela.

## Olhos de Águia (`olhosdeaguia.cpp`)

O alcance do golpe físico é do cliente: `BASE_GetMobAbility` (`0x539C2C`) com
`EF_RANGE`. O DLL troca a regra da Huntress com Olhos de Águia (bit 19): com
Garra na mão direita, +1 de alcance, ou +2 com a Invisibilidade (bit 23); com
outra arma, nada. A Força Espectral continua somando +1 por fora, em quem chama.

## Dividir pilha (`divisao.cpp`)

Shift+clique só abre a caixa de quantidade para os itens de uma lista fixa do
WYD.exe (`0x42052A`), a do legado: Poeiras, Restos, Âmagos e pouco mais. O
DLL troca essa decisão pela lista do servidor (`internal/pilha`), mantendo a
original: Jóias 2441-2444, Pedra do Sábio, Classes A-E e (P), Barras, Pergaminhos
da Água e troféus da Quest 256 passam a dividir. O envio (`0x46B41C`) não tem
lista própria. **Item novo em `pilha.Empilha` precisa entrar também em
`Divide`**, ou o servidor divide e o cliente nunca pede.

## Desenho dentro do quadro (`camadas.cpp`, `pincel.cpp`, `d3dpainel.cpp`)

Base para desenhar por cima do jogo sem janela nenhuma. Cada módulo registra uma
**camada** (`camadas.h`); o `d3dpainel` percorre todas no `EndScene` e desenha
cada uma como uma textura, dentro do quadro do cliente — o print do próprio jogo
sai com elas, e valem em tela cheia.

O desvio do D3D não é na tabela virtual do dispositivo: a deste cliente mora no
heap e o próprio D3D a reescreve (o desvio durava dois quadros). O caminho é
desviar a **importação** de `Direct3DCreate9` no WYD.exe, depois `CreateDevice`
(slot 16), e então o **código** de `d3d9.dll!EndScene`, cujo prólogo é
`push 0x14; mov eax, imm32` — 7 bytes realocáveis. Breakpoint de hardware não
serve: o protetor limpa os registradores de depuração.

Cada camada pinta em GDI num DIB top-down de 32 bits (que já é A8R8G8B8) e o
alfa entra no fim, porque o GDI nunca escreve esse canal. Uma camada cujo
`pixels` devolve nulo não é desenhada, mas continua no teste do mouse: é assim
que se toma um clique sem desenhar nada.

Ler pixel do quadro só é permitido **fora** da cena, então acontece depois do
`EndScene` original (1x1 render target + superfície em memória, no mesmo formato
do backbuffer), a cada 120 ms e logo após cada clique.

O mouse é lido por DirectInput e o corte fica em `0x4B50EF`, logo após a leitura:
é o único lugar que decide de quem é o clique, zerando `rgbButtons` quando o
cursor está sobre uma camada. Sem isso o personagem caminha para o ponto clicado.

## Painel de alvos (`alvos.cpp`, `overlay.cpp`, `macromago.cpp`, `zoomcam.cpp`)

`'` varre os alvos em volta, os números travam e CapsLock solta; o alvo travado
é atacado todo quadro (`0x5162B3`) ou com magia (`0x4567CE`), à escolha do
painel. Aliado é quem tem a capa do mesmo reino (item em `entidade+0x0A58`,
tabelas de `internal/reinos`), o que cobre os guardas. Ajustes em `alvo.txt`.

`macromago.cpp` conserta o macro mágico, que não perseguia: os dois `je` de
`0x4974C7`/`0x4974D7`, a coleira em volta do ponto e a comporta do retorno em
`0x496C4A`. `zoomcam.cpp` levanta o limite de zoom (`cam+0xC0`, 15 no original)
pelo `zoom.txt`.

## Loja do Servidor (`loja.cpp`)

A vitrine global, desenhada como camada. Hoje com ofertas **de mentira**: falta
o servidor espelhar as barracas abertas (`tmserver/internal/handler/autotrade.go`)
e o protocolo entre os dois.

Quem abre é o botão de Loja Pessoal do próprio jogo: uma camada sem desenho fica
sobre aquela célula da barra e toma o clique, então o ícone continua sendo a arte
do cliente. Medidas em 1024x768: divisórias dos slots em x = 583, 621, 660, 698,
736, 774, 812; faixa em y 669..706; interior da célula x 664..693, y 670..704.
Como a barra é ancorada embaixo, o que fica guardado em `loja.txt` é o
deslocamento a partir do centro e a altura a partir da base.

A célula só toma clique com a barra aberta — senão um clique no chão seria
engolido —, e isso é descoberto lendo quatro pontos da moldura no quadro
anterior. Como rede, `0x60F4FC` (id da janela ativa, `0x1388` = janela de itens
da lojinha) é lido todo quadro: aparecendo, volta a `0xFFFF` e a nossa loja abre.
