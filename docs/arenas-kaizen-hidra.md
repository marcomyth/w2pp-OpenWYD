# Arenas do Kaizen e das Hidras (Quest 256, passos 3 e 4)

As duas arenas usam templates do legado que só nascem nelas (NPCGener 3476–3508;
o bloco 3488, de Hidras, fica logo fora da caixa). Mexer num desses arquivos não
afeta nenhum outro lugar do mundo.

| Arena | Nível | Quem | Por arena cheia |
|---|---|---|---|
| Coração do Kaizen | 190–265 | Cav. Kaizen (líder), Cav. Servo | 6 + 24 |
| Hidras | 265–320 | Hidra Dourada (líder), Hidra Imortal | 11 + 40 |

## Ajuste de 14/09/2026

Pedido da equipe: a arena do Kaizen castigava quem chegava com os itens da quest.
Na mesma leva, a Hidra Dourada também caiu pela metade.

**Vida** (templates em `Release/TMsrv/run/npc`, BaseScore e CurrentScore):

| Monstro | Nv | Vida antes | Vida agora | Def | Dano |
|---|---|---|---|---|---|
| Cav. Kaizen | 241 | 8.100 | **4.050** | 1.496 | 608 |
| Cav. Servo | 230 | 17.100 | **8.550** | 1.656 | 728 |
| Hidra Dourada | 310 | 23.000 | **11.500** | 961 | 991 |
| Hidra Imortal | 311 | 7.200 | 7.200 | 811 | 421 |

A XP do template não mudou: com metade da vida, esses monstros passam a render
perto do dobro de XP por minuto.

**Saque** (Mesa de Drops, migração `0065_arenas_kaizen_hidra`, editável em /drops):

| Item | Cav. Kaizen | Cav. Servo | H. Dourada | H. Imortal |
|---|---|---|---|---|
| Resto de Oriharucon 419 | 50%, pacote de 3 | 15% | 50%, pacote de 3 | 10% |
| Resto de Lactolerium 420 | 30%, pacote de 2 | 8% | 30%, pacote de 2 | 5% |
| Âmago de Lobo 2392, Dragão Menor 2393, Urso 2394, Dente de Sabre 2395 | 6% cada | 1,5% cada | 6% cada | 1,5% cada |

**Elfos** (migração `0075_arena_elfos_e_chave_orc`, 17/09/2026): os mesmos números das
Hidras, papel por papel — Mestre Elfo como a Hidra Dourada (pacotes de 3 e de 2), Servo
Elfo como a Hidra Imortal. Antes o Mestre dava os Restos só pelos slots 8 e 9 (25%, uma
unidade) e o Servo nenhum Resto nem Âmago. A Chave do Rei Orc (465) também cai nas duas
arenas: 0,5% na Hidra Dourada e no Mestre Elfo, 0,2% na Hidra Imortal e no Servo Elfo.

Por arena limpa: Kaizen ~12,6 Oriharucon, ~5,5 Lactolerium e ~2,9 Âmagos; Hidras
~20,5 Oriharucon, ~8,6 Lactolerium e ~5 Âmagos. O pacote é do tmServer
(`handler/arenas_quest256.go`), porque a Mesa não guarda quantidade. A regra toma
o lugar do slot do template com o mesmo item: antes o Cav. Kaizen dava 25% de cada
Resto (slots 8 e 9) e a Hidra Dourada 0,05% de Âmago de Urso. Os troféus 4119 e
4120 continuam como estavam.

⏳ **Chave no Kaizen:** a equipe quer uma chave de quest também nesta arena, mas
não a Chave do Rei Orc (465); qual chave ainda está em aberto.

## Kaizen e Hidras na mesma conta

Golpe de skill em monstro: `dano − Def/2`, depois a variação de ±10%
(`combat.SkillDamage`). A defesa pesa em todo golpe, e o Kaizen tem **mais
defesa que as Hidras**, apesar de ser o passo anterior:

| Monstro | Def/2 | Golpe de 2.000 entra com | Golpes para matar (antes → agora) |
|---|---|---|---|
| Cav. Kaizen | 748 | 1.252 | 6,5 → 3,2 |
| Cav. Servo | 828 | 1.172 | 14,6 → 7,3 |
| Hidra Dourada | 480 | 1.520 | 15,1 → 7,6 |
| Hidra Imortal | 405 | 1.595 | 4,5 |

Para limpar a arena com golpes de 2.000: Kaizen ~389 golpes antes e ~195 agora;
Hidras ~346 antes e ~263 agora. Quem entra nas Hidras é 75 níveis acima e bate
mais forte: com golpes de 2.500, as Hidras caem em ~200. A Hidra Dourada segue o
monstro mais pesado das duas arenas, em vida e em dano (991). O Cav. Servo era o
gargalo do Kaizen: tinha o dobro da vida do próprio líder.

## Ajuste de 22/09/2026 — metade dos Restos e 30% do ouro

Pedido do Marco, depois de ver a bolsa voltar da arena cheia de Restos: "os restos
podemos diminuir 50% do drop e o gold em 70%, é muito gold", e "na quest dos kaizen tá
caindo mais restos que coração e isso tá errado".

Ele estava certo, e a conta de papel não bastava: a medição (`handler/arenas_quest256_saque_test.go`,
2.000 mortes por template pelo caminho real de morte — slots do template, Mesa e pacote
do líder) dá mais do que a aritmética sobre as tabelas, por causa do viés do sorteio
descrito no fim desta seção.

| Arena | Troféus | Restos antes | por troféu | Restos agora | por troféu |
|---|---|---|---|---|---|
| Cemitério (Coveiro) | 60,4 | 14,7 | 0,24 | 14,7 | 0,24 |
| Jardim dos Deuses | 59,2 | 13,9 | 0,23 | 13,9 | 0,23 |
| Coração do Kaizen | 26,5 | 36,9 | **1,36** | 19,5 | **0,74** |
| Hidras | 33,0 | 45,5 | **1,36** | 24,0 | **0,73** |
| Elfos | 31,0 | 44,4 | **1,45** | 23,6 | **0,76** |

O salto veio da 0065/0075, não do legado: Cemitério e Jardim nunca passaram pela Mesa e
o líder deles solta meio Resto (slots 8 e 9 do template, 25% cada, uma unidade). O Cav.
Kaizen soltava 50% ×3 mais 30% ×2 — duas Restos e pouco contra um Coração.

**Chances novas** (migração `0098_quest_mortal_restos_e_ouro`, a metade exata das da
0065/0075; os Âmagos e a Chave do Rei Orc não foram tocados):

| Item | Líder (Kaizen, Dourada, Mestre Elfo) | Cav. Servo | H. Imortal, Servo Elfo |
|---|---|---|---|
| Resto de Oriharucon 419 | 25%, pacote de 3 | 7,5% | 5% |
| Resto de Lactolerium 420 | 15%, pacote de 2 | 4% | 2,5% |

**Ouro dos troféus** a 30% do que `Common/Settings/QuestsRate.txt` pagava: 3.000 / 6.000 /
30.000 / 75.000 / 150.000 por tier. A XP e as faixas de nível ficaram como estavam. A
mesma migração é a primeira a gravar a tabela `quest_reward` (vazia desde a 0036), então
a partir dela a recompensa vem do banco e o arquivo de conteúdo deixa de valer.

**O que o Mortal tira das quests do nível 39 ao 320**, contando a XP dos monstros da arena
até o teto da rodada:

| | Antes | Depois |
|---|---|---|
| Troféus usados | ~3.000 | ~3.000 |
| Ouro do troféu | 454 KK | **136 KK** |
| Restos | 3.244 | **1.824** |
| Ouro de vender os Restos no NPC | 236 KK | **133 KK** |
| Ouro do resto do saque | 8 KK | 8 KK |
| **Ouro total** | **698 KK** | **277 KK** |

Vender Resto continua sendo quase metade do ouro do up: o Oriharucon vale 60.000 e o
Lactolerium 100.000 na conta do `sell` (Price/4, depois /2 acima de 10.000). Duas coisas
ficaram de fora deste ajuste, à espera de decisão: mexer nesse preço, e o fato de a venda
limpar o slot inteiro pelo preço de UMA unidade — quem vende a pilha de 120 recebe 60.000,
quem divide antes recebe 7,2 milhões.

### O sorteio da Mesa paga mais do que o painel escreve

Medido e **não** corrigido aqui (`internal/droprule/vies_test.go`): o sorteio é
`rand()%10000` sobre um `rand()` do MSVC que só vai até 32.767. Isso reparte 32.768
sorteios em 10.000 baldes — os 2.768 primeiros recebem quatro e os demais três — e toda
chance abaixo de 27,68% sai **22,1% maior** do que o painel diz. Acima disso o excesso cai
até zerar em 100%.

| Painel | Jogo |
|---|---|
| 50% | 54,2% |
| 30% | 35,9% |
| 15% | 18,3% |
| 6% | 7,3% |
| 0,5% | 0,61% |

Vale para as 619 regras da Mesa — Castelo Orc, Acampamento Troll, Reinos, campo de treino —
e não só para estas arenas, por isso consertar é decisão à parte. É também por isso que
cortar a chance pela metade cortou o drop real em ~47% e não em 50%: o viés é maior
embaixo.
