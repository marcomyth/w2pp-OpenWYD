# Painel de NPCs por cidade (ferramenta local de GM)

> Status: **IMPLEMENTADO** (mapeamento completo; edição implementada mas ainda **não exercitada contra
> um Postgres real**). Origem: necessidade de ver os **NPCs de loja agrupados por cidade**,
> separando os **base** (do conteúdo) dos **criados por nós**, com o **set que usam, aparência,
> posição, nome e itens** — e de poder **ativar/desativar**, **mover** e **adicionar/remover item**.
>
> Complementa [npc-editing-plan.md](./npc-editing-plan.md) (a arquitetura e o backend, já prontos) e
> [npc-generator-inventory.md](./npc-generator-inventory.md) (o inventário bruto de generators).
> Aquele plano registrava "front-end Next.js fora de escopo"; este doc descreve o front-end que
> **existe hoje**, e que não é Next.js.

## 1. Por que uma ferramenta local, e não o BFF Next.js

O contrato gRPC do `NpcAdminService` está escrito e documentado
([npc-admin-nextjs.md](../integrations/npc-admin-nextjs.md)), mas **não existe app Next.js neste
repositório** — nem `package.json` em lugar nenhum. Todo o backend de edição de NPC estava, na
prática, inacessível a um moderador.

O painel segue o precedente que já funciona aqui: o **itembrowser**
(`webserver/cmd/itembrowser`) — uma página HTML única embutida por `go:embed`, servida por
`net/http`, sem Node, sem build step e sem dependência externa. Sobe com um `go run` e não adiciona
nenhuma stack nova a um repositório 100% Go.

Nada disso invalida o BFF: o contrato gRPC continua intacto e o painel usa o **mesmo
`npcadmin.Service`** in-process, com as mesmas validações e a mesma trilha de auditoria.

```
make npc-panel                                 # 96 lojas, só leitura, a partir de Release/
ALL=-all make npc-panel                        # os 572 NPCs (quest, banco, montaria, evento)
DSN="$W2PP_DB_DSN" MODERATOR=1 make npc-panel   # com edição
```

Sem `-dsn` o painel é um **mapa somente-leitura** da árvore de conteúdo; toda escrita responde
`503`. Ele fica em loopback e **não tem login próprio** — a autorização é real mesmo assim, porque
`npcadmin` revalida em cada chamada que `-moderator` é uma conta com `role` `moderator` ou `admin`.

## 2. De onde vêm os dados

Da **árvore de conteúdo**, não do banco. `npc_definition` é semeada a partir dela e guarda um
subconjunto curado (80 linhas) até alguém rodar `dbserver import-npcs` — o conteúdo é a única fonte
completa, e ler dele permite responder "quais NPCs temos e onde" **sem banco nenhum**.

| Dado pedido | De onde sai |
|---|---|
| Nome | `STRUCT_MOB.Name` do template (Latin-1 → UTF-8) |
| Posição | `StartX`/`StartY` do bloco em `NPCGener.txt` |
| **Set / aparência** | `STRUCT_MOB.Equip[16]`, resolvido contra `ItemList.csv` (nome, `IndexMesh`, grade, ícone) |
| Itens (loja) | `STRUCT_MOB.Carry[64]`, só para `Merchant != 0` — num mob esse mesmo array é a tabela de drop |
| Base vs nosso | `npc_definition.origin` — `content` vs `custom` (migration 0018) |

Havendo banco, as linhas são sobrepostas por `slug` (a mesma chave que `import-npcs` deriva), de
modo que o moderador vê a posição e a visibilidade **em vigor**, não as originais de fábrica. O
fluxo continua de mão única: Postgres é dono da definição, o tmServer materializa a entidade viva
(`npc-editing-plan.md` §2).

## 3. A cidade: um dado que não existia

`npc_definition.map_id` é `0` em **todas** as 80 linhas semeadas, e o próprio proto o descreve como
*"label only"*. Ou seja: nenhum NPC tinha cidade atribuída, e ler o valor cru faria **todo** NPC
aparecer como "Armia" (o rótulo do id 0). O painel portanto **deriva a cidade da posição** e só
grava `map_id` quando um moderador confirma ou sobrepõe.

### 3.1 Por que não usar `world.Village()`

`tmserver/internal/world/city.go` já classifica posições — mas por **retângulo `CityLimit`**
(`Basedef.cpp:54`), que é a zona de **taxa e respawn**, não "de que cidade este NPC é". O retângulo
de Armia termina em `y=2052`, o que deixa de fora NPCs que obviamente estão na cidade:

| NPC | Posição | Fora do retângulo por |
|---|---|---|
| `Mestre_Archi` | 2102, 2038 | 14 tiles ao norte |
| `Foema_Ancian` | 2097, 2038 | 14 tiles ao norte |
| `ForeLearner` | 2090, 2047 | 5 tiles ao norte |
| `Cap.Cavaleiros` | 2095, 2047 | 5 tiles ao norte |
| `Guarda_Carga` | 2190, 2148 | 19 tiles a leste |

Rodando o retângulo canônico contra o roster curado, **26 de 80 NPCs (33%) ficavam sem cidade**.

### 3.2 Duas perguntas diferentes, dois critérios

O painel separa o que o legado mistura:

- **"De que cidade é este NPC?"** → `mapzones.Classify`: distância ao centro do assentamento. Os
  raios são **calibrados contra o conteúdo**, não chutados — as distâncias dos blocos vizinhos são
  densas até um ponto e então dão um salto (Armia vai de 2 a 144 e só volta a ter algo em 289;
  Azran 2..142 e depois 272; Erion 17..23 e depois 130). Cada raio fica dentro do próprio vão.
- **"Isto é um NPC de cidade ou um monstro?"** → `mapzones.InTown`: o **retângulo legado**, espelhando
  `world.nonCombatNPC`. Monstros nascem cem unidades fora do portão, mas não nas ruas. Usar o raio
  para essa pergunta jogou **148 Ghouls, Trolls e Orcs dentro de Azran**.

### 3.3 Assentamentos além das 5 cidades

O `Guarda_Carga` (banco, `Merchant == 2`) só existe em cidade de verdade — é o marcador canônico.
Os 10 blocos dele revelam **6 centros**, um a mais que a tabela de 5 cidades do legado. Somando dois
clusters de lojas sem banco, a tabela de zonas ganhou três entradas **`UNVERIFIED`**: os ids 0..4
continuam congelados (batem com `world/city.go` e com o contrato `ListMapZones` já publicado), e as
derivadas entram a partir do 5.

| Zona | Centro | Evidência | Nome |
|---|---|---|---|
| 5 | 3241, 1683 | tem banco: Creta, Guarda_Carga, Alquimista_Odin, Urnammu, Uxmal | não identificado |
| 6 | 1090, 315 | 7 lojas `Merchant==1`, sem banco | **Pesadelo Místico** (ver abaixo) |
| 7 | 1310, 330 | 6 lojas `Merchant==1`, sem banco | **Pesadelo Normal** (ver abaixo) |

### 3.4 Correção: as zonas 6 e 7 não eram vilas

As zonas 6 e 7 entraram aqui como "vilas não identificadas" — clusters de loja sem banco e sem nome
em lugar nenhum. **Estavam erradas.** Ao portar a masmorra do Pesadelo
([pesadelo-plan.md](./pesadelo-plan.md)) os segmentos bateram exatamente:

| Cluster | Segmento /128 | É |
|---|---|---|
| 1090, 315 | (8, 2) | `Pesadelo_M` — Regions.txt linha 5 |
| 1310, 330 | (10, 2) | `Pesadelo_N` — Regions.txt linha 4 |

Não são assentamentos: são os **interiores das masmorras**, e as lojas dentro delas são NPCs de
dungeon. Isso também explica o achado do §4.3: seis das oito lojas com `Carry[]` vazio (`Irena_`,
`Lainy`, `RoPerion`, `Balmers`, `Naomi`, `Rubyen`) são justamente as que ficam nesses dois
segmentos — não são lojas de cidade quebradas.

`mapzones` foi corrigido: 6 e 7 passam a ser *Pesadelo Místico* e *Pesadelo Normal*, marcadas como
verificadas, e o Arcano entrou como zona 9. O teste amarra cada zona ao **segmento**, não ao rótulo
— é o segmento que prova a identificação, e foi a falta dessa checagem que deixou o erro passar.

A zona 5 (3241, 1683) continua **não identificada**: cai no segmento (25,13), que não é Pesadelo, e
tem um `Guarda_Carga`, então é cidade de verdade — só sem nome conhecido.

## 4. O que o painel mostra, e o que deixa de fora

Duas peneiras, em sequência.

### 4.1 NPC vs. monstro (`isNPC`)

`NPCGener.txt` tem **6099 blocos**, a maioria monstro. O byte `Merchant` é sobrecarregado pelo legado
(`Merchant != 0` também marca raids, montarias e guardas de evento), então nem "merchant" nem "em
cidade" isolados respondem "isto é um NPC". O critério é o mesmo de `world.nonCombatNPC`:
**`Merchant != 0`**, ou então **`Merchant == 0` dentro do retângulo de uma cidade e com clã não
hostil** — porque conteúdo real de cidade traz estátuas de combine e atores decorativos sem byte de
papel. Sobram **572 NPCs**; os 5527 monstros de campo ficam de fora (são da drop tool e do editor de
template de mob).

### 4.2 Loja é o escopo padrão (`OnlyShops`)

Dos 572, a maioria ainda não é loja: são quest, banco, montaria e ator de evento. Quem decide não é
a tabela de rótulos, e sim o handler — `reqShopList`
([tmserver/internal/handler/shop.go](../../tmserver/internal/handler/shop.go)) abre `ShopType 1` para
**`Merchant == 1`** e `ShopType 3` para **`Merchant == 19`**, e mais nada.

`Merchant == 2` (Guarda_Carga) fica **de fora de propósito**: chega pela mesma mensagem
`_MSG_REQShopList`, o que o faz parecer loja, mas o handler desvia antes e abre o **cofre da conta**,
não uma lista de preços — é banco, e não tem estoque para editar.

Resultado: **96 lojas** (88 com `Merchant 1` + 8 com `Merchant 19`), payload de ~196 KB. Para ver o
mapa inteiro dos 572, `-all` (ou `ALL=-all make npc-panel`).

| Zona | Lojas |
|---|---:|
| Armia | 39 |
| Azran | 15 |
| Erion | 12 |
| Noatum | 9 |
| Campo (fora de cidade) | 7 |
| Pesadelo Normal (masmorra) | 6 |
| Pesadelo Místico (masmorra) | 6 |
| Nippleheim | 1 |
| Cidade do Leste (não identificada) | 1 |

As 7 lojas de campo são legítimas (as mercenárias `Merc_*`, `Bardes`, `Redmiron`, e `Prona`
estacionada no canto do mapa), então o escopo de loja **não** esconde o balde "Campo" — com menos de
cem linhas não há o que ocultar. E um NPC de campo nunca aparece só como "sem cidade": carrega o
assentamento mais próximo e a distância ("Campo · perto de Erion (130)").

### 4.3 Oito lojas vêm sem estoque

`Prona` (os dois blocos) e as seis lojas das vilas do noroeste — `Irena_`, `Lainy`, `RoPerion`,
`Balmers`, `Naomi`, `Rubyen` — têm o `Carry[]` **inteiramente zerado no conteúdo**. Confirmado nos
bytes crus dos templates de 816 bytes, então é fato do conteúdo, não falha de decodificação: elas
estão no mundo sem vender nada.

O painel marca essas linhas com **"sem estoque"** em vez de escondê-las — uma vitrine vazia é
normalmente o motivo de alguém abrir esta ferramenta. O conjunto exato está fixado em
`TestOnlyShopsReal`, de modo que uma regressão real de decodificação (que esvaziaria muitas outras)
ainda quebra o teste.

## 5. O parser compartilhado

O painel precisava ler `NPCGener.txt`, mas o parser vivia em `tmserver/internal/content` — invisível
para o webServer por causa da regra de `internal` do Go. Foi essa mesma barreira que levou o
dbServer a manter uma **cópia de 97 linhas** dele em `dbserver/cmd/dbserver/main.go`.

O parser foi promovido para **`internal/npcgener`** (o padrão do repo para código compartilhado
entre serviços, `CLAUDE.md` §"Shared packages"). `content.NPCGenerator` virou um alias de tipo e
`LoadNPCGenerators` delega, então nada dentro do tmserver mudou. A cópia do dbServer continua lá e
**pode ser removida em seguida** — não foi tocada nesta passagem para manter o raio pequeno.

## 6. O que ainda não foi verificado

- **O caminho de escrita nunca rodou contra um Postgres real.** Não há Docker nem Postgres na
  máquina onde isto foi construído. A camada HTTP é testada com um `Admin` falso (visibilidade,
  mover preservando a definição, editar loja, mapeamento de todos os `AdminResult`, e o fato de que
  detalhe de erro do banco não vaza para o browser), e por baixo dela roda o `npcadmin.Service`, que
  já tinha seus próprios testes. Ainda assim, **o fluxo ponta a ponta com banco está por validar**.
- **A UI não foi aberta num browser.** Sem Playwright disponível. A lógica de renderização foi
  exercitada headless em Node contra o payload real (208 linhas, painel de detalhe com set e loja,
  `esc()` e `fold()`), mas layout e CSS não foram vistos.
- **`-race` não foi rodado** (sem gcc na máquina). `go build`, `go vet` e `go test` passam.
