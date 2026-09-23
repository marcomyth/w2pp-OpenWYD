# Rodar as verificações no Windows

A CI roda em Linux. Quem desenvolve nesta base costuma rodar no Windows, e as
ferramentas de verificação **mentem** aqui de quatro jeitos diferentes — todos
por codificação de texto ou por sistema de arquivos, nenhum por defeito do
código.

Este arquivo existe porque cada um deles já custou uma hora a alguém, e sempre a
mesma hora: a pessoa vê vermelho, acredita, e vai consertar o que não está
quebrado.

> **A regra que atravessa tudo:** quando o resultado local divergir da CI,
> **suspeite do ambiente antes do código.** E quando o resultado bater com a sua
> hipótese, desconfie mais, não menos — número que confirma o que a gente
> esperava é o momento em que se para de procurar.

## O resumo, para colar no terminal

```bash
export PGCLIENTENCODING=UTF8     # sem isto, migração com acento "quebra"
golangci-lint run --max-same-issues=0 --max-issues-per-linter=0
go test -p 1 ./...               # -race exige cgo, que normalmente não há aqui
```

## 1. `psql` engasga em acento e culpa a migração

**Sintoma:** aplicar as migrações para numa delas com
`0x81 na codificação "WIN1252" não tem equivalente na codificação "UTF8"`, e
parece que aquela migração está corrompida.

**Causa:** os arquivos são UTF-8. O `psql` no Windows assume a página de código
do sistema quando `PGCLIENTENCODING` não está posta, e engasga no primeiro
acento.

**Remédio:** `export PGCLIENTENCODING=UTF8` antes de qualquer `psql`.

**Por que engana:** falha de forma **reprodutível e sempre no mesmo arquivo**.
Três rodadas dando o mesmo erro parecem prova; são o mesmo defeito de ambiente
três vezes. Consistente não é verdadeiro.

## 2. `gofmt` acusa quase todo arquivo do repositório

**Sintoma:** `gofmt -l .` devolve centenas de arquivos, incluindo os que ninguém
tocou.

**Causa:** fim de linha. O repositório guarda LF; se a árvore de trabalho estiver
em CRLF, o `gofmt` considera tudo malformatado.

**Remédio:** já está resolvido pelo `.gitattributes`, que força LF nos arquivos
`.go`, `.sh`, `.sql`, `.proto` e `.yml`. Ele vale a partir do **próximo
checkout** de cada arquivo — numa árvore antiga, refaça o checkout ou clone de
novo. Confira com:

```bash
gofmt -l . | grep -v '\.pb\.go$'      # é o mesmo comando do portão da CI
```

**Por que engana:** o ruído **esconde o sinal**. Enquanto eram 982 falsos, os
sete arquivos de verdade eram invisíveis — e esses sete reprovavam a CI.

## 3. `golangci-lint` corta a saída e não avisa

**Sintoma:** ele mostra 3 apontamentos de um tipo e você conserta os 3.

**Causa:** o padrão é `max-same-issues: 3`. Havia 982.

**Remédio:** quando a pergunta for "quantos", rode sempre com
`--max-same-issues=0 --max-issues-per-linter=0`.

**Por que engana:** três, seis, dez — o corte tem exatamente o tamanho de uma
resposta plausível, e por isso ninguém desconfia dele.

> **A regra geral:** quando a saída for uma **lista** de defeitos, consertar e
> **medir de novo**. Nunca consertar e seguir. O corte também esconde **caso**, e
> não só quantidade: o sétimo arquivo de formatação só apareceu depois que os
> seis primeiros saíram da frente.

## 4. Quatro testes falham sempre, e está tudo certo

Estes **sempre** falham no Windows e **sempre** passam na CI:

```
dbserver/cmd/dbserver   TestBuildNPCDefinitionsNormalizesLegacyTemplateNames
internal/npctemplate    TestResolveTrimsLegacyTrailingDot
internal/npctemplate    TestResolveCaseInsensitive
internal/npctemplate    TestResolveAmbiguousLegacyName
```

**Causa:** eles montam uma pasta com nomes que o NTFS não sabe guardar — um
arquivo terminado em ponto (`Chefe_Treina.`, que o Windows corta) e dois nomes
que diferem só por maiúscula (`Reiners` e `reiners`, que no NTFS são o mesmo
arquivo). O código está certo; a pasta é que não pode existir aqui.

**Remédio:** nenhum. Espere as quatro, e **cite-as no PR** quando reportar o
resultado da suíte, para ninguém confundir com regressão.

## 5. `-race` não roda sem compilador C

`go test -race` exige cgo. Numa instalação Go padrão do Windows não há
compilador C, e o comando responde `-race requires cgo`.

**Consequência que importa:** corrida de concorrência **não se detecta aqui**.
Ela aparece só na CI, e um teste que passa cem vezes nesta máquina pode falhar
na primeira rodada lá. Foi assim que uma corrida que existia desde sempre num
teste apareceu no dia em que a CI passou a rodar.

**Remédio:** deixe a CI ser o juiz de corrida. Localmente, use `-p 1` para
serializar os pacotes e poupar memória.

## Como reportar o resultado num PR

Diga a versão usada e o que sobrou, separando o que é nosso do que é ambiente:

> Lint (v2.12.2, a mesma da CI, com `--max-same-issues=0`): zero apontamento
> deste PR. Suíte: as 4 falhas conhecidas de sistema de arquivos do Windows,
> nenhuma nova.

Rótulo importa: **"não medido" não é "pequeno"**. Onde nenhuma verificação nunca
rodou, o número que você vê é piso e não total.
