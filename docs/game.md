# Comandos do jogo

> Status no servidor Go: ✅ funciona · ⏳ pendente (depende de sistema ainda não modelado).
> Os comandos são digitados no chat (`/comando`); o cliente os envia como um "sussurro"
> cujo alvo é o nome do comando (`_MSG_MessageWhisper`).

✅ /torre: se teleportará para a guerra de torres <br/>
✅ /armia: se teleportará para a cidade de Armia <br/>
✅ /erion: se teleportará para a cidade de Erion <br/>
✅ /azran: se teleportará para a cidade de Azran <br/>
✅ /gelo: se teleportará para a cidade de Gelo <br/>
✅ /kefra: se teleportará para a cidade de Kefra <br/>
✅ /noatun: se teleportará para Noatun <br/>
✅ /red: se teleportará para o rei de Akelonia <br/>
✅ /blue: se teleportará para o rei de Hekalotia <br/>
✅ /arch: se teleportará para a cidade dos reinos (apenas o teleporte; o destrave em si é feito na NPC Lindy, ver abaixo) <br/>
✅ /reino: teleporta de acordo com a capa — capa de Hekalotia (azul) leva ao rei de Hekalotia, capa de Akelonia (vermelha) ao rei de Akelonia, e qualquer capa neutra (sem capa, Capa Branca do Monstro #550, capa verde/Manto do Aprendiz #4006, …) à cidade dos reinos — comando novo, não existe na fonte legada <br/>
⏳ /crias: se teleportará para o drop de crias (Sleipnir e Svaldfire) — sem coordenada na fonte legada <br/>
❌ /destravar40, /destravar90, /arcana: REMOVIDOS. O destrave é pelo caminho do jogador, e a equipe não adianta: o 40 na combinação do Odin (receita do Destrave Lv40), o 90 com a Pedra da Fúria (3020) no nível 90 e 500 de fama — que ainda tem sorteio, e dá a Cythera Mística (3502) — e a Arcana com a mesma Pedra no nível 200 com 500 de fama e as 4 Pedras Secretas, que vira a Arcana (3507). Quem perder o sorteio junta fama de novo <br/>
⏳ /create: (nome da guild): cria guild — sistema de guild não modelado <br/>
✅ /sair: sai da sua guild (limpa a guild + atualiza a tag; metadados de guild não modelados) <br/>
⏳ /guild: mostra o index (ID) da sua guild — sistema de guild não modelado <br/>
✅ /buffs: Remove todos os buffs do personagem <br/>
✅ /xp (ou /bonus): mostra todos os bônus de XP ativos — o total que vale no chão onde o personagem está, de onde cada ponto vem (Baú de XP com o tempo restante, fada, montaria de loja, peças grade 7, peças com joia), os eventos do servidor e a taxa da zona na Mesa de XP. Avisa os dois casos que somem calados: de 500% para cima o jogo ignora o bônus inteiro, e os +30% da Fada Suprema não valem dentro do Pesadelo. Em grupo diz a regra (vale o maior bônus de quem está na luta) e não um número, porque esse depende de quem está perto na hora do abate. Comando novo, não existe na fonte legada <br/>
✅ /status: a ficha de combate que a janela de personagem não tem. Começa pelo que decide o duelo — **acerto e esquiva** em percentual contra um oponente igual, mais os dois números crus por trás deles (a precisão, que é descontada da esquiva do alvo, e a esquiva própria em milésimos, teto 650) —, depois **perfuração e absorção** e o resto do bloco de PvP do equipamento. Só então vem o contexto: Defesa de Evolução, o quanto a montaria absorve, a Jóia da Absorção e o bônus de drop. A Defesa **não** é repetida: a janela do personagem já a mostra. Cada linha só aparece se o personagem tiver aquilo. Não existe "taxa de acerto" absoluta: a rolagem é sempre a sua precisão MENOS a esquiva do outro, por isso o percentual é medido contra uma cópia do próprio personagem. Comando novo, não existe na fonte legada <br/>
✅ /pontos: mostra os pontos de lojinha da **conta** e, quando há uma barraca de pé, quanto ela rende por janela e quanto falta para o próximo crédito. Comando novo, não existe na fonte legada — ver "Lojinha" abaixo <br/>
✅ /novato: entrega o kit de entrada — 5 Frangos Assados e 3 Baús de Experiência (as variantes 5760/5761, intransferíveis e sem preço, empilhados num espaço cada) e uma Shire de 3 dias (item 3980: +150 de dano, +15 de ataque mágico, 20% de absorção PvE e +3% de XP). **Uma vez por conta**, gravado em `newbie_kit_claim` (0062) — apagar o personagem não devolve o kit — e só para personagens **Mortais**. O que não couber na bolsa vai para o baú da conta, e a mensagem diz isso. O tempo da Shire vai nos efeitos de duração, então o relógio dela só começa quando o jogador montar. Comando novo, não existe na fonte legada. As duas variantes precisam do cliente publicado (`go run ./webserver/cmd/kitnovatocliente`), senão aparecem sem nome e sem ícone <br/>
✅ /cp: mostra os pontos de caos atuais do personagem (`PKPoint-75`; 0 = nick branco). Recuperam de duas formas: +1 por hora online (gate do `RegenMob` legado) e **+1 por nível subido**, ambas com teto no neutro 75 — o ganho por nível é um desvio consciente do legado, pedido na issue #279 <br/>
✅ /nt: mostra quantas entradas de Pesadelo Arcano o personagem tem (`extra.NT`). Persistido em `character.nightmare_tickets`; a Escritura do Pesadelo dá 13 e cada entrada no Arcano gasta 1 ([pesadelo-plan.md](./migration/pesadelo-plan.md)) <br/>
✅ /nig: mostra o horário de cada tier do Pesadelo — qual está aberto e quanto falta para os outros. Desvio consciente: o legado imprime só o relógio (`!!HHMMSS`) e deixa o cliente calcular <br/>
✅ **/\<nomedojogador\>** (sem escrever nada depois): mostra o nick, a **cidadania** e a **fama** daquele jogador, mais a guild entre colchetes quando ele tem uma, e a **mensagem** dele (`/snd`) numa segunda linha se ele tiver posto uma. É o `_NN_Check_User_Info` do legado (`Cidadania: %d / Fama: %d`) — um sussurro **sem texto** não é sussurro, é "inspecionar". Escrever `/fulano oi` continua sendo sussurro normal <br/>
✅ /nick \<jogador\>: o mesmo que acima, em forma de comando — existe porque é descobrível, enquanto "digite o nome e não escreva nada" não é <br/>
✅ /snd \<texto\>: define a sua mensagem de recado, que aparece para quem te inspecionar. `/snd` sozinho limpa. Vale só enquanto você está logado — o legado também apaga a cada login — e é cortada em 96 caracteres <br/>
✅ /gritar \<mensagem\>: grito global — consome 1 Trombeta Mágica (item 3330) e envia `[Nome]> mensagem` a todos os jogadores online, em verde (`_MSG_MagicTrumpet`). Alias legado: `/spk`. Sem trombeta, avisa e não grita; o alcance é o deste canal (o fan-out entre canais do legado passava pelo DBSrv e não foi portado) <br/>

> Bônus já implementados (existem na fonte legada, fora da lista acima): `/selados`,
> `/amagos`, `/agua` (teleportes).

# Comandos de GM / moderação

> Digitados como `/gm <subcomando> <args>` — o cliente envia como sussurro ao alvo
> `gm` com o resto da linha no corpo (o mesmo truque do `_MSG_MessageWhisper`).
> Autoridade vem da coluna `account.role` (`moderator`/`admin`), carregada no login
> — **não** do frágil "Level ≥ 1000" do legado. Comando negado é silencioso. Toda
> execução é auditada (slog: conta, alvo, args). Implementação: `handler/gm.go`.

✅ /gm kick \<jogador\>: desconecta um jogador online (não derruba GM de nível igual/superior) <br/>
✅ /gm notice \<texto\> (ou /gm aviso): anúncio global a todos os jogadores <br/>
✅ /gm goto \<jogador\> (ou /gm ir): teleporta você até o jogador <br/>
✅ /gm summon \<jogador\> (ou /gm puxar): puxa o jogador até você <br/>
✅ /gm spawn \<id\>: cria uma criatura de teste (índice do roster de summons) na sua posição <br/>
✅ /gm item \<id\> \[qtd\]: coloca um item (por índice) no seu inventário; `qtd` (1–120) cria um stack <br/>
✅ /gm setlevel \<n\>: sobe o seu nível para n (apenas sobe — não rebaixa) <br/>
✅ /gm setgold \<n\>: define o seu ouro carregado <br/>
✅ /gm ban \<jogador|conta\>: bloqueia a conta (via `account.is_blocked`) e derruba se online <br/>
✅ /gm unban \<jogador|conta\>: remove o bloqueio da conta <br/>
✅ /gm guildname \<id\> \<nome\>: registra o nome de uma guild (issue #131; só em memória — não há fluxo de criação de guild ainda) <br/>
✅ /gm guildfame \<id\> \<fama\>: registra a fama de uma guild (issue #131; mesma ferramenta admin-only do legado `+guildfame set`) <br/>
✅ /gm dano [nome] (ou /gm ataque): mostra a conta do Ataque da janela parte por parte — itens, montaria, bônus de arma da classe, atributos, buffs e dano da arma <br/>
✅ /gm guerra torre aviso [min]: anuncia a Guerra de Torres agora; abre depois de `min` minutos (padrão 1) <br/>
✅ /gm guerra torre abrir [min]: abre a guerra na hora, por `min` minutos (padrão 24, o mesmo das 20:06 às 20:30) <br/>
✅ /gm guerra torre fim: encerra agora — a guilda com a torre ganha a fama, como no fim normal <br/>
✅ /gm guerra torre estado: fase, dono, horários e a agenda do painel <br/>
⏳ /gm guerra cidade / noatum: responde que ainda não existe; entram quando essas guerras forem portadas <br/>

> `notice` sai na linha de aviso do servidor (MSG_MessagePanel, ID 0 — o `SendNotice` do legado),
> prefixada `[GM]`. A guerra forçada ignora a hora e o interruptor do painel até terminar, e passa
> pelas mesmas transições da agendada: mesmos avisos a todos, mesma limpeza da área, mesmo prêmio.
>
> `ban`/`unban` gravam em `account.is_blocked` — o login já rejeita contas bloqueadas; a migração do ban administrativo para o binServer
> (entitlement) fica para uma issue futura (`web-platform-plan.md §binServer`).


# Lojinha

A barraca de venda (autotrade) **não prende mais o vendedor**. Ao abrir a lojinha o
servidor ergue um **clone** ao lado do dono — um Carbúnculo mercador
(`Release/TMsrv/run/npc/Merc_Carbunkle`) com o nome do dono — e o personagem sai
livre para andar, caçar e mexer na bolsa enquanto a barraca continua vendendo.

Desvio consciente do legado, onde o vendedor **era** a barraca (`_MSG_SendAutoTrade.cpp`)
e qualquer ação derrubava a loja. Só é possível porque o cliente não pergunta se uma
barraca é jogador: o `MSG_CreateMobTrade` copia o título para a entidade sem testar o
id, e o clique responde com o id cru (confirmado desmontando o `WYD.exe` 7662 em
`0x0048541D` e `0x004604D1`).

**A barraca só cai em três situações**, e nenhuma delas é jogar normalmente:

1. o dono fecha a lojinha;
2. a **sessão acaba** — sair do jogo, voltar à seleção de personagem ou cair a conexão;
3. o anti-fraude do próprio autotrade recusa (o item do baú não bate com o anunciado).

Andar, atacar, lootear, arrastar item, alternar o modo PK, entrar numa troca (mesmo
recusada), usar a máquina, o Pergaminho da Água, o Pesadelo, a gema ou o bilhete de
quest **não derrubam a loja**. Antes derrubavam, porque o `RemoveTrade` do legado
fechava a barraca junto com a troca e é chamado de quinze lugares de jogo comum —
ver a nota em `removeTrade` (`handler/trade.go`).

O dono pode ir para **outro mapa** e a barraca continua vendendo. O baú **não viaja
com ele**: o Cargo é da conta, o servidor o guarda por `account_id` e só o descarrega
quando a conta desconecta (`World.ReleaseCargo`). A única distância que importa numa
venda é a do **comprador** até a barraca.

O que **não** mudou:
- A loja continua vendendo do **baú da conta** (Cargo), e a compra continua protegida
  pelo memcmp contra o slot vivo — mexer no baú com a loja aberta não duplica nada,
  apenas faz a próxima compra daquele slot falhar.
- Sem `-content` (ou sem o template) a loja volta ao comportamento do legado: abre,
  prende o vendedor, e aí sim **andar fecha** — porque nessa forma ele *é* a barraca.

## Pontos por tempo de lojinha

Uma barraca aberta **com pelo menos um item à venda** rende **3 pontos a cada 15
minutos**, ou **7** se o dono estiver com uma **Fada Azul** equipada (3901, 3904 ou
3907 — as três durações). Os pontos são da **conta**, não do personagem, e vivem numa
carteira própria (`shop_points`), separada do saldo de doação.

Vender a última peça para o relógio na hora; reabastecer não paga o tempo em que a
prateleira ficou vazia. O saldo aparece com `/pontos` no jogo e na página da conta no
painel. **Gastar** os pontos ainda não existe — só o acúmulo.

> Divergência deliberada: no legado só a Fada Azul de 3 dias (3901) dá bônus de drop;
> as de 5 e 7 dias dão XP (`CMob.cpp:716` vs `731`). Aqui as três valem os 7 pontos,
> porque quem compra "a fada azul" de 7 dias não espera ganhar menos que a de 3. A
> divergência é só desta recompensa — o bônus de drop continua fiel ao legado.
# Evoluções 
NPC Evoluções vende poeira, upe o seu Mortal, Arch, Celestial e Sub Celestial com ela.

Para liberar a Soul permanente do Mortal no nível técnico 369+, leve à Kibita a Pedra Secreta da
classe: TransKnight usa Água (5334), Foema usa Sol (5336), Beastmaster usa Terra (5335) e Huntress
usa Vento (5337). A pedra será consumida, a capa será substituída pela versão do reino e o
personagem retornará à seleção para recarregar a progressão.

Faça as quest dos quatros cristais no seu Arch para liberar mais pontos.
- Dê /red ou /blue para ir direto para o rei desejado.
• Não precisa transformar o Lac, somente separe 10 que já vai funcionar

**Destrave do Arch (níveis 355 e 370) — NPC Lindy.** O Arch para de ganhar
experiência ao chegar no 355 e no 370 até fazer o destrave; é assim no servidor
original e é o que mantém o personagem dentro da janela da quest. A receita é
posicional, nos 7 primeiros espaços da composição e nesta ordem exata:

| Espaço | Item |
|---|---|
| 1 e 2 | Poeira de Lactolerium (#413) em pilha de **exatamente 10** |
| 3 | Pergaminho Selado (#4127) |
| 4 a 7 | Poeira de Lactolerium (#413) avulsa, uma por espaço |

O destrave do 355 entrega a capa do reino (Hekalotia, Akelonia ou Aventureiros,
conforme o clã); o do 370 consome 1 de Fama e exige Fama > 0.

**Bônus da capa no 370 — regra deste servidor, não do legado.** Concluir o
destrave do 370 grava dois efeitos **na própria capa do reino**: `EF_HP 120`
(+120 de HP máximo) e `EF_RESISTALL 8` (+8 de resistência nos quatro elementos).
O servidor original não dá nada nesse destrave — a flag
`QuestInfo.Arch.Level370` é lida em apenas três lugares, todos travas de nível ou
de experiência. A divergência é deliberada: a quest custa 1 de Fama e segura a
progressão até ser feita, então deixa algo em troca.

Os efeitos vão na **instância do item**, não no personagem. É assim que joia e
refino já funcionam, e tem duas vantagens: o tooltip da capa mostra o bônus sem
precisar alterar o conteúdo do cliente, e o servidor já soma os dois efeitos ao
pontuar equipamento. A capa tem três espaços de efeito e um `+9` ocupa um deles;
se não houver espaço para os dois, o destrave conclui mesmo assim e o jogador é
avisado — o bônus não some em silêncio.

**Comando de teste:** `/gm questreset 355 | 370 | cristal | arch` limpa as flags
para refazer a quest. Ele não desfaz o que já foi concedido — refazer depois de
um reset empilha HP/MP no personagem — então anote os números antes de testar.

Se o personagem passou do nível sem ter feito o destrave (possível em contas
antigas, antes do gate de experiência existir), a NPC ainda aceita a receita —
mas o personagem **volta para o nível da quest**, perdendo os níveis ganhos
indevidamente. Isso é uma divergência deliberada do servidor original, que exige
o nível exato e deixaria a conta travada para sempre.

Para destravar o lv 40 do Cele, faça a combinação do Destrave Lv40 no Odin. Para o lv 90, use a Pedra da Fúria (500 de fama). Não existe comando de equipe para isso: quem perder o sorteio da Pedra junta os 500 de fama de novo.

Pegue lv 200 no seu Cele, faça a quest da Cythera Arcana (Pedra da Fúria + as 4 Pedras Secretas + 500 de fama). O Sub Celestial e os três resets ainda não existem neste servidor.
• Refine a capa para +9 logo após disso.

# Acessórios

Reforma decidida em 16/09/2026. O plano completo, com o diagnóstico de cada item, está no
artefato "Reforma dos Acessórios". Das famílias do primeiro espaço (anel, bracelete, pingente,
brinco e colar), Hércules e Hecate já foram refeitas e Zeus ganhou Defesa; Titã, Athena e Gaia
seguem com os status de sempre.

**Hércules e Hecate: porcentagem de dano e Defesa.** Os dois deixaram de dar Dano e Magia fixos.
Hércules dá **% de dano físico** e Hecate **% de dano mágico**, mais Defesa:

| Degrau | % de dano | Defesa | +9 | +15 |
|---|---|---|---|---|
| Bracelete | 4% | 50 | 8% · 100 | 16% · 200 |
| Pingente | 6% | 100 | 12% · 200 | 24% · 400 |
| Brinco | 8% | 150 | 16% · 300 | 32% · 600 |
| Colar | 10% | 200 | 20% · 400 | 40% · 800 |

- A porcentagem e a Defesa crescem com o refino, exatamente como o tooltip do cliente mostra.
- **Refino de acessório acima do +9 segue o cliente**, não o legado do servidor: +9 ×2,0, +10
  ×2,2, +11 ×2,5, +12 ×2,8, +13 ×3,2, +14 ×3,7, +15 ×4,0 (WYD.exe soma 1 ao nível de todo
  acessório a partir do +9). Vale para todos os acessórios que sobem além do +9: brincos,
  Místicos, Arcanos, Ankhs, planetas e Amuleto dos Amantes.
- A **% física** vale no ataque inteiro, arma incluída, e nas skills que não usam Magia (2ª
  árvore do TK e Huntress). Soma com as poções e o Assalto: poção +5% com brinco +8% dá +13%.
- A **% mágica** vale no dano pronto de toda skill que usa Magia, somada ao mesmo multiplicador
  das poções. A janela "Atq Mágico" do cliente não mostra essa porcentagem; o golpe tem.
- Os anéis de Hércules (Dano 10) e Hecate (INT 8) continuam como estão.

**Zeus: velocidade de ataque e Defesa.** Bracelete, Pingente e Brinco de Zeus mantêm a velocidade
de ataque (15/18/21) e ganham a mesma Defesa do degrau de Hércules e Hecate (50/100/150), que
também cresce com o refino: Brinco de Zeus +15 dá 84 de velocidade e 600 de Defesa. Zeus não tem
colar, e o Anel de Zeus continua como está.

**Caminho até o +15.** Poeira do +1 ao +9, máquina +10 (Ailyn), Lactolerium para o +11 e
Odin (receita do +12) do +12 ao +15. Aceitam: Brincos (591-595), Braceletes (507, 510-514),
Amuletos de Prata (551-554) e de Ouro (555-558), Amuletos Místicos
(559-562) e Arcanos (567-570), Ankhs (661-663), os sete planetas (762-768) e o Amuleto dos
Amantes (1738). A liberação é por item, não pelo espaço do equipamento: orbs, Pedras
Espirituais, Pedra Amunra e Sephirot continuam fora.

**+10 de acessório.** Dois iguais em +9, a Pedra do Sábio e **quatro joias iguais, de qualquer
uma das quatro**: Diamante (drop), Esmeralda (perfuração), Coral (XP) ou Garnet. A joia fica
gravada no item e vale em qualquer espaço. Com o acessório já +10 equipado, usar uma Gema
(Diamante, Esmeralda, Coral ou Garnet) troca a joia gravada. Custo e chance são os da +10 das
armas.

**Evolução (máquina +10).** O item em +9, uma cópia dele em qualquer refino, a Pedra do Sábio e
quatro joias iguais. O item sai **em +0** no degrau seguinte da mesma linha:

| De (+9) | Para (+0) |
|---|---|
| Amuleto de Cristal (563-566) | Amuleto Místico da mesma árvore (559-562) |
| Pedra Necromântica (654-656) | Gema da Siren do mesmo status (658-660) |
| Gema da Siren (658-660) | Ankh do mesmo status (661-663) |

Na falha ficam o item e a cópia; perdem-se a pedra e as joias. A chance é a linha "Evolução de
acessórios" da Mesa das Máquinas (sem linha: 41%), anunciada para o servidor como a +10.

**Arcano (Odin).** Amuleto Místico +15 na primeira célula, a segunda e a terceira vazias, e as
quatro Pedras Secretas (Água, Terra, Sol, Vento) da quarta à sétima. Sai o Amuleto Arcano da
mesma árvore, em +0. Na falha o Místico volta +15 e só as pedras se perdem. Chance na Mesa das
Máquinas (sem linha: 35%). O Arcano não é mais vendido nem cai de monstro.

**Status novos.**

| Item | Nível | Status |
|---|---|---|
| Amuleto de Cristal | 145 | Skill 10 (era 8) |
| Amuleto Místico | 200 (era 147) | Skill 12 (era 10) |
| Amuleto Arcano | 300 (era 180) | Skill 14 (era 15) |
| Gema da Siren | 150 (era 156-195) | igual |
| Ankh da Justiça / Eternidade / Glória | 220 (era 160) | MP 300 / HP 300 / Crítico 7% (eram 115 / 100 / 5%) |
| Planetas | 250 (era 54) | dois status cada, ver abaixo |
| Amuleto dos Amantes | 200 (era 0) | HP 150 (era 100) |

**Planetas: os míticos do quarto espaço.** Ocupam o lugar dos Ankhs como o item a ter: cada um dá
dois status perto do Ankh (HP e MP 250, contra 300 do Ankh) e Resistência a todos 10. Raridade
Mítico no tooltip, ícone próprio para cada um, nível 250.

| Planeta | Para quem | Status | +15 |
|---|---|---|---|
| Netuno | físico | HP 250 · Dano 60 | HP 1000 · Dano 240 |
| Urano | tanque | HP 250 · Defesa 120 | HP 1000 · Defesa 480 |
| Vênus | Foema | MP 250 · Magia 20 | MP 1000 · Magia 80 |
| Marte | crítico | HP 250 · Crítico 7% | HP 1000 · Crítico 28% |
| Saturno | mago com vida | HP 250 · Magia 20 | HP 1000 · Magia 80 |
| Mercúrio | híbrido | HP 250 · MP 250 | HP 1000 · MP 1000 |
| Júpiter | mago defensivo | Magia 20 · Defesa 120 | Magia 80 · Defesa 480 |

Todos com Resistência a todos 10, que **cresce com o refino** como o tooltip mostra (40 no +15).
Os pontos de Skill que Marte a Júpiter davam saíram.

O **Crítico** do tooltip é a chance real: o servidor soma o crítico do equipamento, divide por 4 e
sorteia contra 255, então 7% no tooltip (70 no catálogo) dá 17 em 255, 6,9%.

A **Resistência a todos** do Amuleto dos Amantes **não cresce com o refino** (regra deste
servidor): um +15 continua dando 10. No resto do equipamento o refino multiplica a resistência
como no legado.

**Onde se consegue.** Planetas e Amuleto dos Amantes são raros: saíram de toda loja (inclusive
a de doação) e só caem de monstro — a raridade se ajusta na Mesa de Drops. Os Ankhs continuam à
venda. O Aki de Armia passa a vender os anéis de Hércules, Titã, Athena, Hecate e Zeus, no lugar
do Remédio e do Elixir da Coragem e dos três Círculos Divinos (que seguem na loja da
CustomShop). O Anel de Gaia ficou de fora por falta de vaga.

**Cura da Foema com o Amuleto dos Amantes.** A Foema que aprendeu Renascimento (a oitava skill
da árvore de cura) e está com o Amuleto dos Amantes equipado cura **30% a mais** com Cura e
Recuperar. O bônus entra antes do teto de 1100 (Mortal e Arch) e 2200 (Celestial): quem já
cura no teto não ganha nada. Não existe no legado.
