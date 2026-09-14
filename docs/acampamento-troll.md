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
✅ A corrida: a Chave dos Trolls abre o acampamento, um grupo por vez, 15 min <br/>
✅ O Troll Enigma do mundo (bloco 3804) desligado <br/>
⏳ A Chave dos Trolls no cliente: nome, ícone e descrição (ver abaixo) <br/>
⏳ O contador gráfico: o `GamePatch.dll` com o campo (20,15) precisa ser compilado e ir pelo launcher <br/>
⏳ Prêmio de conclusão e trava de nível/evolução <br/>

## A chave

É a **Chave dos Trolls (3223)**, um dos "Cupom da Sorte" sem uso do catálogo — o
mesmo caminho da Chave do Inferno (3222).

| Onde | Como | Meta |
|---|---|---|
| Quest 256 dos Elfos (nível 320–350) | na entrada paga, sorteada (`acampamentoTrollKeyOnEntry`) | 1 a cada 3 entradas |

- É a mesma entrada que sorteia a Chave do Rei Orc, com um sorteio próprio: uma
  entrada pode dar as duas chaves.
- Só a entrada paga sorteia: gastar o Emblema do Guarda no NPC ou pela bolsa. O
  Mestre Grifo leva de graça para a mesma arena e não dá chave.
- Os monstros da arena não dão a chave, pelo mesmo motivo do Orc: a arena não tem
  relógio e renasce sozinha.

**No cliente, a chave ainda aparece como "Cupom da Sorte".** O nome sai do
`ItemList.bin`, que é gerado do `ItemList.csv`, e o ícone e a descrição são os
passos do `webserver/cmd/itemnovocliente` (`client/icones/README.md`). Até isso
chegar pelo launcher, o servidor já trata o item como chave, e o Xamã já fala
"Chave dos Trolls".

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
  - nascem o boss, os 4 seguidores, os 2 guardiões e 40 de tropa;
  - o grupo cai no meio do acampamento (2651,1983), cada um numa casa livre, com o
    contador de 15 min.
- **Durante:**
  - quem não é do grupo não entra andando no acampamento;
  - os seguidores voltam a cada 30 s.
- **Fim:** aos 15 min; 2 min depois que o Troll Enigma cai (o tempo de saque); ou 1
  min depois que ninguém do grupo está mais no acampamento. Os monstros da quest
  somem e quem estiver dentro vai para fora do muro.
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

Réplicas dos Trolls do acampamento, com o visual do original e os números do tier
correspondente do Castelo Orc.

| Template | Nome no jogo | Veio de | Tier (Orc) | Nv | HP | Defesa | Dano | Resist. | Bloco |
|---|---|---|---|---|---|---|---|---|---|
| `ATroll_Enigma` | Troll Enigma | `Troll_Enigma` | boss (Grão-Lorde) | 350 | 3.000.000 | 3.000 | 2.020 | 25 | 6116 |
| `ATroll_Mago` | Troll Mago | `Troll_Mago` | seguidor (Guarda do Lorde) | 320 | 150.000 | 2.200 | 1.520 | 15 | 6117 (grupo de 4) |
| `ATroll_Caos` | Troll Caos | `Troll_Caos` | guardião (Sentinela) | 330 | 450.000 | 2.400 | 1.620 | 20 | 6118, 6119 |
| `ATroll_Insano` | Troll Insano | `Troll_Insano` | tropa (Cavaleiro) | 300 | 18.000 | 1.800 | 1.220 | 10 | 6120–6123 |
| `ATroll_Cacador` | Caçador Troll | `Cacador_Troll` | tropa (Cavaleiro) | 300 | 18.000 | 1.800 | 1.220 | 10 | 6124–6127 |

**Onde nascem:**

| Quem | Pontos |
|---|---|
| Troll Enigma | (2668,1985), a jaula do Enigma do mundo |
| Troll Mago | (2660,1985), raio 3 |
| Troll Caos | (2658,1973) e (2662,1994) |
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
no painel. A conta supõe 40 de tropa, 2 guardiões, o boss e ~44 seguidores (4 no
começo e mais 4 a cada 30 s, em uns 5 min perto do boss).

| Meta por entrada | Tropa | Seguidor | Guardião | Boss |
|---|---|---|---|---|
| ~6 Armas D (8 tipos, chance por tipo) | 0,5% | 0,5% | 10% | 12,5% |
| ~4,2 Repletion D (Classe D 4019) | 10% | — | 10% | — |
| ~3,5 Poeira de Oriharucon 412 | 7,5% | — | 7,5% | 30% |
| ~1,4 Poeira de Lactolerium 413 | 2,8% | — | 2,8% | 25% |
| ~5,2 Âmago de Cav. s/ Sela N 2396 | 3% | 9% | — | — |
| ~3,9 Âmago de Cav. s/ Sela B 2401 | 2% | 7% | — | — |
| ~2,7 Âmago de Cav. Fantasma N 2397 | — | 5% | 25% | — |
| ~1,6 Âmago de Cav. Fantasma B 2402 | — | 3% | 15% | — |
| ~0,4 Ovo de Cav. Fantasma: N 2307 + B 2312 | — | — | — | 25% + 12,5% |

- **Os ovos de Cavalo Fantasma só caem do boss**: 1 a cada ~2,7 entradas.
- Uma entrada rende uns 28 itens, bem menos que os ~106 do Orc: a bolsa cheia
  preocupa menos aqui. Drop de mob ocupa um espaço novo, e com a bolsa cheia o item
  se perde.

### As Armas D

As D de nível mais baixo, a primeira de cada par D do catálogo:

| Física (add de dano) | Mágica (add de magia) |
|---|---|
| Gram 869 · Martelo Dragão 809 · Luna 910 · Arco Élfico 824 · Martelo Psíquico 935 | Olho do Carbunkle 899 · Gungnir 854 · Cajado de Âmbar 902 |

**O que o servidor acrescenta** (`acampamentoTrollFinish`, só em monstro da quest):
a arma sai com o refino que o bônus de drop sorteou (+0 a +2; se ele pôs outra
coisa no slot, a arma sai +0) e com **um** add sorteado da tabela:

| | Tropa, seguidor e guardião | Só o boss |
|---|---|---|
| Física | dano 36 (50%) · 45 (35%) · 63 raro (15%) | dano 45 (25%) · 63 (30%) · **72** (25%) · **27 + skill** (20%) |
| Mágica | magia 20 (50%) · 24 (35%) · 28 raro (15%) | magia 24 (25%) · 28 (30%) · **32** (25%) · **20 + skill** (20%) |

- **Skill** é o `EF_SPECIALALL` ("Aprendizagem de Skill"), 15, 18 ou 21 por igual.
- Cada número é um degrau do bônus de drop do legado (`refine/dropbonus.go`): dano
  de arma anda de 9 em 9, magia de 4 em 4 e skill de 3 em 3.
- **A mágica com skill fica nos 20 de magia** porque o design fixou 20-24-28-32 para
  as magas; a física com skill cai para 27, um degrau abaixo dos 36. Esta é a
  leitura do pedido e fica para confirmar em jogo.

## Como testar

A corrida inteira, com conta de GM:

```
/gm item 3223                   a Chave dos Trolls (o cliente ainda mostra "Cupom da Sorte")
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
