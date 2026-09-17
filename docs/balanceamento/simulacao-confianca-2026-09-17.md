# Simulação do TK Confiança — 17/09/2026

## Meta

A meta é a mesma da [simulação da Huntress](simulacao-ht-2026-09-17.md): uma luta PvP entre personagens equivalentes deve durar **de 1 a 2 minutos, com poção**. A Confiança é a primeira árvore do TK redesenhada. As regras estão em `tmserver/internal/handler/arvore_confianca.go`.

## Como rodar

```
go test -tags simulacao -run TestSimulacaoConfianca -v ./tmserver/internal/handler/
```

Para gravar o relatório em Markdown, defina `SIM_OUT=<arquivo>`.

## O que mudou na ferramenta

- **Poção:** a simulação agora cura dos dois lados, até 2.000 de HP por segundo enquanto a vida não está cheia. Esse é o teto do servidor: a poção sobe o `ReqHp`, e o tique de 1 s fecha a barra em no máximo `applyCasting` = 2.000. A trava entre poções é de só 100 ms, então quem tem auto-poção chega nesse teto.
- **Aura da Vida:** a cada 5 s, com a marca de PvP de 10 s.
- **Debuff do Fanatismo e marca de PvP:** os dois entram na simulação.

## O Paladino é hipotético

Ninguém joga TK Confiança ainda. O Paladino tem os mesmos 3.700 pontos de atributo do Porradeiro, distribuídos assim: FOR 400, DES 2000, INT 1000, CON 300.

Do Porradeiro ele copia a defesa (2414) e o crítico (31,6%). Além disso:
- **HP:** 15.500, contando a CON do Destino.
- **Arma:** Lança do Triunfo +11, que vale 140% no Mortal.
- **Maestria:** 255 na Confiança.
- **Ataque físico:** 2.500, um chute, porque a DES não conta.

Ele é Mortal para lutar com as duas referências sem a Defesa de Evolução no meio.

## Resultado

| Cenário (com poção) | Vencedor | Tempo médio |
|---|---|---|
| Paladino × Porradeiro (TK físico) | Paladino 10/10 | **3,3 s** |
| Paladino × Xorimpas (HT) | Paladino 9/10 | **1,8 s** |
| Xorimpas × Porradeiro (referência) | HT 10/10 | **4,4 s** |
| Paladino × Cav. Lugefer como está | Paladino 10/10 | 2,0 s |
| Paladino × Lugefer proposto (1 mi de vida, dano ×2, defesa ×1,5) | Lugefer 10/10 | 9,6 s |

### Dano médio de cada skill do Paladino
- **Em PvP:** cerca de 4.600 a 4.800 por golpe, no Porradeiro e na Xorimpas.
- **No Lugefer atual:** cerca de 10.500 a 10.900.

As quatro skills batem quase igual. O valor próprio de cada uma (5, 13, 75, 190) some perto de 3× (0,6 DES + 0,4 INT) + 3× arma.

### O que os números dizem
1. **Estamos a 20-30 vezes da meta.** Nenhuma luta chega a 10 s, nem com poção. Os 2.000 de HP por segundo da poção não seguram golpes de 4 a 9 mil a cada 0,8 s. É o dano de todas as classes, e não só o da Confiança, que precisa descer (ou a vida subir) antes de a meta ser alcançável.
2. **A esquiva do Paladino está no teto.** Com DES 2000 ele já esquiva 65% (o teto do sorteio) contra o Porradeiro e a Xorimpas. O +42% da régua de Destreza não acrescenta nada nesse nível. Ele só pesa para quem tem pouca DES.
3. **O Paladino ganha da Xorimpas 9 em 10.** A HT erra a maior parte dos golpes (Golpe Felino 0 de 6) e tem metade da vida dele.
4. **A Aura quase não aparece.** As lutas acabam antes do segundo tique de 5 s. Com lutas de 1 a 2 minutos ela vai pesar: 775 a cada 5 s em PvP, 2.325 fora de PvP.
5. **O Lugefer proposto mata o Paladino em 10 s,** mesmo com poção. O Lugefer ×2 bate mais que a poção cura.

## Limites
- **Mana:** fica de fora; ninguém fica sem mana.
- **Montaria:** a absorção da montaria não entra.
- **Buffs de grupo e posição:** também ficam de fora.
- **Skills de apoio do Paladino:** ele não usa Samaritano nem Fúria Divina.
- **Arch:** o cajado com martelo não foi simulado, porque não há referência Arch. No Arch ele vale 140%, o mesmo que a lança no Mortal.

## Corte de dano em PvP

```
go test -tags simulacao -run TestSimulacaoCortePvP -v ./tmserver/internal/handler/
```

O teste mexe nos mesmos controles do painel (`/rates/combate`): a % de skill e a % de físico contra jogador, os dois com o mesmo valor. Cada célula resume 10 lutas com poção.

| % em jogador | Paladino × Xorimpas (HT) | Paladino × Porradeiro (TK) | Xorimpas × Porradeiro |
|---|---|---|---|
| 100% | 2 s (Paladino 10) | 3 s (Paladino 10) | 4 s (HT 10) |
| 50% | 7 s (Paladino 10) | 13 s (Paladino 10) | 21 s (HT 10) |
| 45% | 15 s (Paladino 10) | 18 s (Paladino 10) | 17 s (HT 10) |
| 40% | 41 s (Paladino 10) | 32 s (Paladino 10) | 24 s (HT 10) |
| 37% | 200 s (Paladino 9, HT 1) | 59 s (Paladino 10) | 46 s (HT 10) |
| 35% | empate (15 min) | 144 s (Paladino 10) | 38 s (HT 10) |
| 32% | empate | empate | 177 s (HT 10) |
| 30% | empate (HT 1) | empate | 279 s (HT 10) |
| 20% | empate | empate | empate |

### O que o corte mostra
- **A poção é um muro.** Ela cura até 2.000 por segundo. Quando o dano por segundo fica abaixo disso, ninguém morre; quando passa, a luta acaba em segundos. A faixa de 1 a 2 minutos existe, mas é estreita: entre 35% e 40%. Ela muda de um confronto para outro e vai mudar com qualquer troca de equipamento.
- **Só o corte não segura a meta.** Para a duração crescer aos poucos e não num degrau, a cura da poção em PvP também precisa entrar na conta: um teto menor ou uma recarga maior.
- **O Paladino vence a Xorimpas em todos os cortes em que a luta termina.** A esquiva dele (65%, no teto) e o HP (o dobro do dela) pesam mais que o dano da HT.
