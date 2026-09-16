# Quest do Acampamento Troll

Uma corrida pelo acampamento murado a leste de Erion (x 2638–2670, y 1967–2003),
onde o Troll Enigma ficava na jaula. É feita para Mortais de 320 a 400, com os
números do Castelo Orc, e **paga só em saque**: Armas D, âmagos e ovos de Cavalo
Fantasma. Nunca dá experiência. É regra nova, não do legado, pedida pela equipe
em 14/09/2026; o design fica no artefato "Atlas de Quests W2PP".

## Estado

✅ Os cinco monstros da quest (templates `ATroll_*`) e o Xamã Troll <br/>
✅ O saque na Mesa de Drops (migração `0064_acampamento_troll`) <br/>
✅ 0 XP para os monstros da quest <br/>
✅ Armas D com o add do design <br/>
✅ A corrida: a Chave do Rei Orc abre o acampamento, um grupo por vez, 15 min <br/>
✅ O Troll Enigma do mundo (bloco 3804) desligado <br/>
✅ Recalibrada em 16/09: Enigma no centro depois de 100 abates, 4 Caos, todos montados com arma +11, escada nova de adds <br/>
⏳ A descrição nova da Chave do Rei Orc no `itemhelp.dat` precisa ir pelo launcher <br/>
⏳ O contador gráfico: o `GamePatch.dll` com o campo (20,15) precisa ser compilado e ir pelo launcher <br/>
⏳ Prêmio de conclusão e trava de nível/evolução <br/>

## A chave

É a **Chave do Rei Orc (465)**, a mesma do Castelo Orc, desde 16/09/2026. Quem
tem a chave escolhe onde gastá-la: com o Xamã Orc (ou no Portão Sul) ou com o
Xamã Troll. De onde ela sai está em `docs/castelo-orc.md`: entradas pagas das
Hidras e dos Elfos e o Deserto.

- Até 16/09 o acampamento tinha chave própria, a Chave dos Trolls (3223), com um
  sorteio à parte na entrada dos Elfos. Ela nunca chegou ao cliente (lá era o
  "Cupom da Sorte"), e o sorteio saiu junto: a entrada dos Elfos dá uma chave só,
  a 465, 1 a cada 3. A linha 3223 do `ItemList.csv` voltou a ser `Cupom_da_Sorte`.
- **A descrição da chave no cliente** (`itemhelp.dat`, bloco 465) passou a citar
  os dois lugares e termina com "Trolls ou Orcs o que vamos caçar hoje?". É gravada
  com o `webserver/cmd/itemnovocliente` (`client/icones/README.md`).

## A corrida (`handler/acampamento_troll_run.go`, motor em `handler/corrida.go`)

- **Quem abre:** o líder do grupo (ou quem está sozinho), entregando a chave ao
  **Xamã Troll** (template `ATroll_Xama`, Merchant 100, grau 41), de pé do lado de
  fora do muro oeste, em (2633,1979). A chave é consumida.
- **O Xamã nasce pelo código**, não pelo NPCGener, pelo mesmo motivo do Xamã Orc:
  com `W2PP_NPC_EDITING` ligado, um mercador do NPCGener só apareceria depois de um
  `dbserver import-npcs`.
- **Um grupo por vez**, no servidor inteiro. Com o acampamento ocupado, o Xamã diz
  quantos minutos faltam.
- **Na abertura:**
  - quem não é do grupo e está dentro vai para fora do muro oeste (2633,1985);
  - nascem os 4 seguidores, os 4 guardiões e 40 de tropa. **O boss não**;
  - o grupo cai no meio do acampamento (2651,1983), cada um numa casa livre, com o
    contador de 15 min.
- **Durante:**
  - quem não é do grupo não entra andando no acampamento;
  - os seguidores voltam a cada 30 s;
  - **o Troll Enigma nasce no centro (2651,1983) no 100º abate** de monstro da
    quest. Um só por chave. O grupo ouve a contagem a cada 25 abates.
- **Fim:** aos 15 min; **5 s depois que o Troll Enigma cai** (o drop já foi direto
  para a bolsa, então não há tempo de saque a guardar); ou 1 min depois que ninguém
  do grupo está mais no acampamento. Os monstros da quest somem e quem estiver
  dentro vai para fora do muro.
- **Os 100 abates cabem nos 15 min, mas sem folga:** a abertura põe 48 monstros no
  acampamento (40 de tropa, 4 Caos e 4 Magos), e só os Magos voltam, até 4 vivos a
  cada 30 s. Os 52 que faltam são Magos, ou seja, no mínimo 13 levas (6,5 min) de
  Magos de 75 mil de HP.
- **Reinício do servidor** encerra a corrida (nada é persistido).
- **A caixa é só o miolo**, dentro do muro. Os Trolls do mundo aberto em volta
  (blocos 700–715) nascem nas linhas y 1960 e 2010, fora dela: quem caça ali não é
  varrido nem barrado durante a corrida.
- **Não há portão.** O acampamento aparece fechado no HeightMap do servidor, e o
  grupo entra por teleporte. Os Trolls do mundo aberto continuam lá fora durante a
  corrida.
- **Contador: precisa do GamePatch.** O servidor manda o mesmo `MsgStartTime` da
  Água e do Orc, mas o WYD.exe 7662 só desenha o contador em 15 campos fixos. O
  acampamento fica no campo (20,15), acrescentado em
  `client/gamepatch/timerfields.cpp`. O tempo também vai em texto a cada minuto
  ("Acampamento Troll: N min"), para todo cliente.

O motor (`corrida.go`) é o do Castelo Orc com as coordenadas, a chave e os textos
virando dados. O Orc ainda roda a própria cópia (`castelo_orc_run.go`); passá-lo
para o motor é um passo à parte.

## Os monstros

Réplicas dos Trolls do acampamento, com o visual do original. Partiram dos números
do tier correspondente do Castelo Orc e foram recalibradas em 16/09/2026, a pedido
do Marco: o Mago com metade do HP e do dano, o Caos com o HP que era do Mago, a
tropa com metade do dano. O Enigma fica para depois.

| Template | Nome no jogo | Veio de | Papel | Nv | HP | Defesa | Dano | Resist. | Montaria | Bloco |
|---|---|---|---|---|---|---|---|---|---|---|
| `ATroll_Enigma` | Troll Enigma | `Troll_Enigma` | boss | 350 | 3.000.000 | 3.000 | 2.020 | 25 | Cavalo Fantasma B (2372) | 6116 |
| `ATroll_Mago` | Troll Mago | `Troll_Mago` | seguidor | 320 | 75.000 | 2.200 | 760 | 15 | Dente de Sabre (2365) | 6117 (grupo de 4) |
| `ATroll_Caos` | Troll Caos | `Troll_Caos` | guardião | 330 | 150.000 | 2.400 | 1.620 | 20 | Cavalo s/Sela N (2366) | 6118, 6119 (2 cada) |
| `ATroll_Insano` | Troll Insano | `Troll_Insano` | tropa | 300 | 18.000 | 1.800 | 610 | 10 | Dragão Menor (2363) | 6120–6123 |
| `ATroll_Cacador` | Caçador Troll | `Cacador_Troll` | tropa | 300 | 18.000 | 1.800 | 610 | 10 | Dente de Sabre (2365) | 6124–6127 |

- **Visual:** todos montados e com a arma do Troll original em +11 (EF_SANC 234).
  Só aparência, como nos guardiões do Castelo Orc: o Equip de monstro é visual e
  não entra nos números dele.
- **Exceção de status no painel** (`mob_template_stat`, com `W2PP_MOB_STAT_EDITING`)
  passa por cima destes arquivos, inclusive do equipamento. Se um template já foi
  editado no painel, o ajuste daqui não aparece.

**Onde nascem:**

| Quem | Pontos |
|---|---|
| Troll Enigma | (2651,1983), o centro, depois de 100 abates |
| Troll Mago | (2660,1985), raio 3 |
| Troll Caos | 2 em (2658,1973) e 2 em (2662,1994) |
| Troll Insano | (2651,1983) · (2650,1972) · (2655,1990) · (2648,1978) |
| Caçador Troll | (2646,1997) · (2656,1998) · (2660,1971) · (2663,1978) |

- O Troll Mago mantém as magias do original (barra de skill 32/35).
- O 0 XP está no código (`handler/acampamento_troll.go`) e não em Clan 4, como no
  Orc. O Exp do template também é 0: o boot avisa ("unbalanced Exp") para estes
  cinco, e o aviso é esperado. **Não rode o `cmd/exptool` neles.**
- **O Troll Enigma do mundo** (bloco 3804) não tinha drop nenhum: o Carry do
  template estava vazio e não havia regra na Mesa. A migração 0064 o desliga pela
  tabela `npc_generator_off`; ele volta com `/gm npc on 3804`. O bloco não sai do
  NPCGener, porque o índice de um bloco é a posição dele no arquivo.

## O saque

Os templates não têm drop próprio. Tudo é da Mesa de Drops e se ajusta em `/drops`
no painel. A conta supõe 40 de tropa, 4 guardiões, o boss e ~56 seguidores (os 4
do começo e as levas que faltam para os 100 abates). As chances não mudaram em
16/09; a meta subiu porque há mais guardiões e mais Magos por entrada.

| Meta por entrada | Tropa | Seguidor | Guardião | Boss |
|---|---|---|---|---|
| ~8 Armas D (8 tipos, chance por tipo) | 0,5% | 0,5% | 10% | 12,5% |
| ~4,4 Repletion D (Classe D 4019) | 10% | — | 10% | — |
| ~3,6 Poeira de Oriharucon 412 | 7,5% | — | 7,5% | 30% |
| ~1,5 Poeira de Lactolerium 413 | 2,8% | — | 2,8% | 25% |
| ~6,2 Âmago de Cav. s/ Sela N 2396 | 3% | 9% | — | — |
| ~4,7 Âmago de Cav. s/ Sela B 2401 | 2% | 7% | — | — |
| ~3,8 Âmago de Cav. Fantasma N 2397 | — | 5% | 25% | — |
| ~2,3 Âmago de Cav. Fantasma B 2402 | — | 3% | 15% | — |
| ~0,4 Ovo de Cav. Fantasma: N 2307 + B 2312 | — | — | — | 25% + 12,5% |

- **Os ovos de Cavalo Fantasma só caem do boss**: 1 a cada ~2,7 entradas.
- **Cada Troll Caos rola as 8 armas separadas, a 10% cada**: sai em média 0,8 arma
  por Caos, e 3 armas de um Caos só acontecem (~4% das vezes). Com 4 Caos, são
  ~3,2 armas por entrada só deles.
- Uma entrada rende uns 35 itens, bem menos que os ~106 do Orc: a bolsa cheia
  preocupa menos aqui. Drop de mob ocupa um espaço novo, e com a bolsa cheia o item
  se perde.

### As Armas D

As D de nível mais baixo, a primeira de cada par D do catálogo:

| Física (add de dano) | Mágica (add de magia) |
|---|---|
| Gram 869 · Martelo Dragão 809 · Luna 910 · Arco Élfico 824 · Martelo Psíquico 935 | Olho do Carbunkle 899 · Gungnir 854 · Cajado de Âmbar 902 |

**O que o servidor acrescenta** (`acampamentoTrollFinish`, só em monstro da quest):
a arma sai com o refino que o bônus de drop sorteou (+0 a +2; se ele pôs outra
coisa no slot, a arma sai +0) e com **um** add sorteado da tabela. Desde 16/09/2026
todo monstro usa a mesma escada, e quanto maior o add, mais raro:

| | Tropa, seguidor e guardião | Só o Troll Enigma |
|---|---|---|
| Física | dano 27 (35%) · 36 (28%) · 45 (20%) · 54 (12%) · **63** (5%) | dano 36 (10%) · 45 (20%) · 54 (35%) · 63 (35%) |
| Mágica | magia 12 (30%) · 16 (25%) · 20 (18%) · 24 (13%) · 28 (9%) · **32** (5%) | magia 20 (10%) · 24 (20%) · 28 (35%) · 32 (35%) |
| Skill | nunca | **20% das armas** levam também skill |

- **O teto é 63 de dano e 32 de magia**, para todo monstro. O Enigma não passa
  dele: só sorteia pendendo para o alto.
- **Skill** é o `EF_SPECIALALL` ("Aprendizagem de Skill"), 15, 18 ou 21 por igual,
  no terceiro slot da arma, **por cima** do add de dano ou magia. Só o Troll Enigma
  solta.
- Cada número é um degrau do bônus de drop do legado (`refine/dropbonus.go`): dano
  de arma anda de 9 em 9, magia de 4 em 4 e skill de 3 em 3.

## Como testar

A corrida inteira, com conta de GM:

```
/gm item 465                    a Chave do Rei Orc
vá ao muro oeste do acampamento (2633,1979) e clique no Xamã Troll como líder
```

Um monstro solto, sem corrida:

```
/gm criar ATroll_Enigma         o boss na sua frente (não renasce)
/gm gerar 6117 aqui             o grupo de 4 seguidores
/gm gerar 6120 aqui             um grupo de tropa
/gm criar ATroll_Caos           um guardião
```

GM (moderador para cima) não é barrado nem varrido do acampamento durante a
corrida.
