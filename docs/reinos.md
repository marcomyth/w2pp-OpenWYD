# Reinos e Reis — a invasão

Desenho fechado em 17/09/2026 (Atlas de Quests, área "Reinos e Reis"). A cidade
dos Reinos fica em x 1676–1776, y 1556–1892: Hekalotia (azul, clã 7) embaixo, com
o Rei Harabard em (1750,1574), e Akelonia (vermelho, clã 8) em cima, com o Rei
Glantuar em (1750,1880).

## Quem ataca quem

| Jogador | Guardas azuis | Guardas vermelhos |
|---|---|---|
| Capa azul | não fere (golpe zerado, com aviso) | atacado à vista |
| Capa vermelha | atacado à vista | não fere |
| Sem capa de reino | ignorado até bater num azul | ignorado até bater num vermelho |

- **A capa decide o lado** (`internal/reinos.ClanDaCapa`, Basedef.cpp:3220-3244),
  só dentro da cidade dos Reinos. O clã gravado no personagem não muda: a guilda
  depende dele.
- **Inimigo do Reino:** quem não tem capa de reino e bate num monstro de um reino
  vira inimigo DAQUELE reino por 10 minutos, renovados a cada golpe. Os guardas
  dele caçam o jogador; a morte apaga a marca. O golpe de um pet marca o dono.
- Os Reis e o exército levam golpe. Continuam NPC o Guarda Real (byte 17 = 100),
  os oráculos e o Kingdom Broker.
- Nenhum monstro dos Reinos dá XP.

## O exército

Equipamento de monstro é só visual; a força está no template.

| Papel | Templates | HP | Defesa | Dano azul | Dano vermelho | Montaria |
|---|---|---|---|---|---|---|
| Tropa | Guarda do Rei, Combatente, Lanceiro, Virago, Bruxa | 60.000 | 2.600 | 2.600 | 1.900 | Andaluz B / N |
| Elite | Cavaleiro Real, Averest, Feiticeira | 180.000 | 3.000 | 3.200 | 2.400 | Andaluz B / N |
| Escolta do Trono | Escolta Real (nova, 6 por Rei) | 500.000 | 3.400 | 4.800 | 4.000 | Fenrir / Fenrir das Sombras, nível 35 |
| Rei | Harabard / Glantuar | 6.000.000 | 4.000 | 3.500 | 12.000 | Unicórnio 120 |

Tropa, elite e escolta com arma e set +11. Os Reis com arma +12 (Demolidor
Celestial no azul, Éden no vermelho), set +12, Coroa Celestial e a capa celestial
do reino (Mestre de Hekalotia 3197 / Mestre de Akelonia 3198). O de Akelonia é
sempre o mesmo template com "_" no fim.

- **Rei Azul absorve 60%** de todo golpe (`absorcaoDoRei`, pelo `absorbBlow`).
- **Os Reis e a Escolta voltam 4 horas depois de morrer** (blocos 8, 9 e
  6128-6139 sem período de minuto; `esperaDoRenascimento`). A tropa e a elite
  seguem nos blocos de sempre.
- **Avisos:** abaixo de 90% de vida, quem veste a capa do reino recebe "O Rei está
  sob ataque" (uma vez por luta). A queda do Rei vai para o servidor inteiro, com o
  nome de quem deu o golpe final.

## Saque (migração 0074)

A chance fica na Mesa de Drops, o pacote no código (`reinosPacotes`).

| Item | Tropa | Elite | Escolta | Rei |
|---|---|---|---|---|
| Alma do Unicórnio 1740 | — | — | — | 5% (Harabard) |
| Alma da Fênix 1741 | — | — | — | 5% (Glantuar) |
| Fragmento de Alma 3224 | — | — | 2% | — |
| Âmago de Andaluz B 2405 / N 2400 | 2% ×5 | 8% ×10 | — | — |
| Âmago de Fenrir 2406 / Sombras 2408 | — | — | 12% ×5 | 50% ×10 |
| Moeda de Prata 1Mi 4026 | 3% | — | — | — |
| Moeda de Prata 5Mi 4027 | — | 5% | 15% | 100% ×10 |
| Poeira de Oriharucon 412 | 8% | 15% | 25% | 100% ×30 |
| Poeira de Lactolerium 413 | 3% | 8% | 25% | 100% ×15 |
| Classe C / B / A | C 6% ×5 | B 8% ×10 | A 10% ×10 | A 100% ×20 |

O Rei continua soltando o Símbolo de Coragem (1732). **As Almas e a Pedra da
Imortalidade saem de todo o resto do jogo** (regra "*" a 0%, e as regras nomeadas
de painel para esses itens são apagadas). Todo o saque vai para a bolsa de quem dá
o golpe final.

## Praça do Dragão de Armia

- **Dragão Dourado** (Merchant 36, bloco 3420, em 2068,2068): cada 10 Fragmentos de
  Alma na bolsa viram uma Alma, sorteada meio a meio. Um clique troca tudo que os
  Fragmentos e o espaço permitem.
- **Lendas** (blocos 6140-6143, templates `Lenda_TK/FM/BM/HT`, todas com o nome
  `Lenda_Passada_`, Merchant 100 grau 42): set e arma +15 da classe, capa Mestre dos
  Aventureiros, Unicórnio 120. Clique: "Eu já fiz minha parte, você poderá ser a
  próxima lenda."

| Lenda | Set | Arma |
|---|---|---|
| TK | Melpômene 1230-1233 | Hrotti 3765 |
| FM | Potâmides 1365-1368 | Gae Bulg 3665 |
| BM | Dríade 1515-1518 | Dordje 3733 |
| HT | Urânia 1665-1668 | Eithna 3625 |

## Pendente

- **Fragmento de Alma (3224)** no cliente: ItemList.bin, itemicon.bin, itemhelp.dat e
  Itemname.bin. O servidor já o trata como empilhável (`internal/pilha`); o
  `Release/Common/ItemList.csv` ainda o chama de Cupom da Sorte.
- **Dragão maior e vermelho:** o corpo não mudou (200). Testar 293, 335 e 224 com
  `/gm criar` antes de trocar o template.
- **Números:** calibrar na Forja de Builds contra os personagens de topo.
