# Simulação de combate: Huntress contra Cav. Lugefer e TK (17/09/2026)

## A regra que este teste serve

Equalizar as classes contra os mesmos monstros e jogadores de referência, para um
PvP saudável: **uma luta entre personagens equivalentes deve durar de 1 a 2
minutos, com poção**. Antes de mexer em números, desenhar as árvores das outras
classes; depois, criar cenários de personagem para todas as classes e evoluções e
rodar esta simulação em cada um.

Estado em 17/09: só a Huntress foi redesenhada. O TK ainda não foi mexido.

## Referências

- **Porradeiro (TK físico, Mortal 400, antes das mudanças):** FOR 2802, DES 712,
  CON 189, HP 14.277, Ataque 5.515, Defesa 2.414, crítico 31,6%, maestrias 0.
- **Xorimpas (HT, Mortal 400, depois das mudanças):** FOR 2172, DES 700, CON 512,
  HP 7.526, Ataque 8.111, Defesa 2.348, crítico 54,0%, maestrias 174/278/224/223.
- **Cav. Lugefer** (gerador no3622, arquivo `Cav._Lugefer`): 32.000 de vida, dano
  5.000, defesa 4.900. Pedido do Marco: pelo menos 1 milhão de vida, mais defesa e
  o dobro do dano.

## Como rodar

```
go test -tags simulacao -run TestSimulacaoHuntress -v ./tmserver/internal/handler/
SIM_OUT=relatorio.md go test -tags simulacao -run TestSimulacaoHuntress ./tmserver/internal/handler/
```

A tag `simulacao` deixa o teste fora do CI. Ele chama as funções de combate do
servidor na ordem do handler de ataque e do golpe de monstro, com os personagens
montados pelos números da janela C (`simulacao_ht_test.go`).
`TestSimulacaoContaAberta` abre a conta de um golpe de cada skill.

## Escolhas e limites

- Uma ação a cada 0,8 s (a trava de cadência do servidor), para os dois lados.
- HT: Tempestade, Golpe Felino e Lâmina das Sombras quando liberam; senão físico.
  Evasão Aprimorada, Encantar Gelo e Toxina ligados; todas as skills aprendidas.
- TK: só golpe físico (maestria 0 na janela).
- Arma da HT suposta: Khyrius +11 (778 de dano de arma); o dano de skill depende
  3× dela.
- Regras de combate padrão do código; o painel de produção pode ter outras.
- Sem poção, cura, grupo nem absorção de montaria — por isso ainda não mede a meta
  de 1-2 minutos.
- Proposta do Lugefer testada: 1 milhão de vida, dano ×2, defesa ×1,5.

## Resultado (main b217690d)


## Personagens montados

| | Ataque | Defesa | HP | Crítico (byte) | Esquiva contra o outro |
|---|---|---|---|---|---|
| Xorimpas (HT) | 8111 | 2348 | 7526 | 135 | 578‰ vs TK, 650‰ vs Lugefer |
| Porradeiro (TK) | 5515 | 2414 | 14277 | 79 | 216‰ vs HT |
| Cav. Lugefer (arquivo) | 5000 | 4900 | 32000 | — | 1‰ vs HT |

HT: esquiva da Captura +125%, perfuração da Lança de Ferro 11%.

## PvE — Cav. Lugefer como está

| Luta | Vencedor | Tempo (s) | Vida final da HT | Golpes | Dano médio físico | Tempestade média | Veneno total |
|---|---|---|---|---|---|---|---|
| 1 | HT | 0.8 | 7526 | 2 | 0 | 24488 | 0 |
| 2 | HT | 0.0 | 7526 | 1 | 0 | 39597 | 0 |
| 3 | HT | 0.0 | 7526 | 1 | 0 | 35853 | 0 |
| 4 | HT | 0.8 | 3471 | 2 | 0 | 27829 | 0 |
| 5 | HT | 0.8 | 7526 | 2 | 0 | 29720 | 0 |
| 6 | HT | 0.0 | 7526 | 1 | 0 | 40059 | 0 |
| 7 | HT | 0.0 | 7526 | 1 | 0 | 36188 | 0 |
| 8 | HT | 0.8 | 3739 | 2 | 0 | 29720 | 0 |
| 9 | HT | 0.8 | 7526 | 2 | 0 | 18841 | 0 |
| 10 | HT | 0.0 | 7526 | 1 | 0 | 34848 | 0 |

HT venceu 10 de 10; tempo 0.4 s (±0.4); vida final 6742 (±1570).

**HT no Lugefer, somando as 10 lutas**

| Golpe | Usos | Acertos | Críticos | Dano médio | Maior | Lâmina Aérea (vezes / média) |
|---|---|---|---|---|---|---|
| Tempestade | 10 | 10 | 0 | 31714 | 40059 | 0 / 0 |
| Golpe Felino | 5 | 5 | 3 | 17423 | 23680 | 0 / 0 |

## PvE — Cav. Lugefer proposto: 1 milhão de vida, dano ×2, defesa ×1,5

| Luta | Vencedor | Tempo (s) | Vida final da HT | Golpes | Dano médio físico | Tempestade média | Veneno total |
|---|---|---|---|---|---|---|---|
| 1 | Lugefer | 2.4 | 0 | 3 | 0 | 42019 | 0 |
| 2 | Lugefer | 0.8 | 0 | 1 | 0 | 27484 | 0 |
| 3 | Lugefer | 5.6 | 0 | 7 | 8496 | 26966 | 0 |
| 4 | Lugefer | 6.4 | 0 | 8 | 7371 | 34038 | 0 |
| 5 | Lugefer | 2.4 | 0 | 3 | 0 | 36018 | 0 |
| 6 | Lugefer | 5.6 | 0 | 7 | 8416 | 43837 | 0 |
| 7 | Lugefer | 0.8 | 0 | 1 | 0 | 37801 | 0 |
| 8 | Lugefer | 7.2 | 0 | 9 | 6119 | 49126 | 0 |
| 9 | Lugefer | 0.8 | 0 | 1 | 0 | 35335 | 0 |
| 10 | Lugefer | 8.8 | 0 | 11 | 8989 | 38907 | 4372 |

HT venceu 0 de 10; tempo 4.1 s (±2.8); vida final 0 (±0).

**HT no Lugefer, somando as 10 lutas**

| Golpe | Usos | Acertos | Críticos | Dano médio | Maior | Lâmina Aérea (vezes / média) |
|---|---|---|---|---|---|---|
| físico | 20 | 20 | 10 | 7931 | 17968 | 13 / 60 |
| Tempestade | 10 | 10 | 0 | 37153 | 49126 | 0 / 0 |
| Golpe Felino | 7 | 7 | 3 | 12157 | 20325 | 0 / 0 |
| Lâmina das Sombras | 14 | 14 | 3 | 18002 | 41439 | 0 / 0 |

## PvP — Xorimpas (HT) contra Porradeiro (TK)

| Luta | Quem abre | Vencedor | Tempo (s) | Vida final HT | Vida final TK | Físico HT (médio) | Lâmina Aérea (vezes/média) | Tempestade HT | Físico TK (médio) | Críticos TK | Esquivas HT |
|---|---|---|---|---|---|---|---|---|---|---|---|
| 1 | HT | HT | 2.4 | 6470 | 0 | 0 | 0 / 0 | 7962 | 528 | 0 | 0 de 2 |
| 2 | TK | HT | 1.6 | 6403 | 0 | 0 | 0 / 0 | 8603 | 1123 | 1 | 1 de 2 |
| 3 | HT | HT | 1.6 | 7023 | 0 | 0 | 0 / 0 | 7816 | 503 | 0 | 0 de 1 |
| 4 | TK | HT | 3.2 | 6500 | 0 | 2868 | 1 / 713 | 5261 | 513 | 0 | 2 de 4 |
| 5 | HT | HT | 1.6 | 7526 | 0 | 0 | 0 / 0 | 8684 | 0 | 0 | 1 de 1 |
| 6 | TK | HT | 2.4 | 6998 | 0 | 0 | 0 / 0 | 8116 | 528 | 0 | 2 de 3 |
| 7 | HT | HT | 4.0 | 6450 | 0 | 2241 | 1 / 673 | 6559 | 538 | 0 | 2 de 4 |
| 8 | TK | HT | 2.4 | 7028 | 0 | 0 | 0 / 0 | 5213 | 498 | 0 | 2 de 3 |
| 9 | HT | HT | 3.2 | 7526 | 0 | 2944 | 1 / 680 | 7524 | 0 | 0 | 3 de 3 |
| 10 | TK | HT | 0.8 | 7526 | 0 | 0 | 0 / 0 | 14281 | 0 | 0 | 1 de 1 |

HT venceu 10 de 10; tempo 2.3 s (±0.9).

**HT no TK, somando as 10 lutas**

| Golpe | Usos | Acertos | Críticos | Dano médio | Maior | Lâmina Aérea (vezes / média) |
|---|---|---|---|---|---|---|
| físico | 4 | 4 | 4 | 2573 | 2944 | 3 / 688 |
| Tempestade | 10 | 10 | 0 | 8001 | 14281 | 0 / 0 |
| Golpe Felino | 9 | 7 | 4 | 6876 | 9876 | 0 / 0 |
| Lâmina das Sombras | 6 | 3 | 1 | 10980 | 18852 | 0 / 0 |

**TK na HT, somando as 10 lutas**

| Golpe | Usos | Acertos | Críticos | Dano médio | Maior | Lâmina Aérea (vezes / média) |
|---|---|---|---|---|---|---|
| físico | 24 | 10 | 1 | 581 | 1123 | 0 / 0 |

