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
