# Transforma a marca do servidor (marca_w.png) no fundo.bin que o DLL carrega.
#
# O painel e pintado com GDI sobre um fundo quase preto, e a arte entra somando
# luz - nao cobrindo. Tudo o que e feito aqui existe para que ela pareca estar
# atras da janela, e nao colada em cima dela:
#
#   - o pedestal sai. A arte tem um azul-marinho de fundo proprio; somado, ele
#     deixaria o painel inteiro azulado. O valor e medido nas linhas de cima e
#     de baixo da propria imagem, e ainda se tira uma folga (RESTO) para o que
#     sobra de bruma nao virar um retangulo visivel.
#   - as beiradas apagam. A arte e um retangulo, e retangulo de luz sobre fundo
#     escuro tem borda. Perto das quatro bordas o brilho cai ate zero, numa
#     rampa suave, e ai nao ha onde a imagem comecar.
#   - o centro e o do desenho, nao o da imagem. O W fica na metade de baixo do
#     quadro, entao a imagem e reenquadrada no centro de luz dela mesma; assim
#     o DLL centra no meio dos botoes e o W sai centrado de verdade.
#   - o brilho e espalhado, nao cortado. Arredondar cada pixel para o inteiro
#     mais proximo deixa um degrau de uma unidade justamente onde a rampa
#     morre - uma linha reta de ponta a ponta no escuro. Com a ordem de Bayer,
#     a fracao vira quantidade de pixels acesos, e a rampa morre sem degrau.
#   - o tamanho e o do painel: a arte ja sai na medida em que sera desenhada, e
#     o DLL nao escala nada em tempo de desenho.
#
# Precisa do Pillow: python -m pip install Pillow
# Rode aqui dentro:  python gera_fundo.py

from PIL import Image
import struct

ORIGEM = 'marca_w.png'
DESTINO = 'fundo.bin'
LARG = 290          # a largura do painel da loja (loja.cpp: kLargura)
CORTE = 665         # abaixo disto comeca o texto "WYD RETRY", que nao vai no fundo
ESCALA = 380        # a marca e desenhada maior que o painel e cortada nas laterais
FORCA = 0.80        # quanto do brilho da arte chega ao painel
RESTO = 6           # a bruma que ainda se desconta depois do pedestal
FUNDE_X = 46        # largura da rampa que apaga a arte nas laterais
FUNDE_Y = 54        # idem, em cima e embaixo


# A matriz de Bayer 8x8, normalizada: o limiar que cada pixel exige para subir
# um nivel. E a mesma ideia do degrade do painel (pincel.cpp), em duas dimensoes.
BAYER = [[(  # ordem classica, gerada pela recorrencia de Bayer
    ((y & 4) >> 2) | ((x & 4) >> 1) | ((y & 2) << 1) | ((x & 2) << 2) |
    ((y & 1) << 4) | ((x & 1) << 5)
) for x in range(8)] for y in range(8)]
BAYER = [[(v + 0.5) / 64.0 for v in linha] for linha in BAYER]


def rampa(d, larg):
    """1 no meio, 0 na borda, com as pontas macias (smoothstep)."""
    if d >= larg:
        return 1.0
    t = d / larg
    return t * t * (3 - 2 * t)


im = Image.open(ORIGEM).convert('RGB').crop((0, 0, 1024, CORTE))
alt = round(im.height * ESCALA / im.width)
im = im.resize((ESCALA, alt), Image.LANCZOS)
sobra = (ESCALA - LARG) // 2
im = im.crop((sobra, 0, sobra + LARG, alt))
px = im.load()

# o fundo proprio da arte, medido na primeira e na ultima linha
borda = []
for x in range(LARG):
    borda.append(px[x, 0])
    borda.append(px[x, alt - 1])
pedestal = [sorted(c[i] for c in borda)[len(borda) // 2] + RESTO for i in range(3)]

# o centro de luz: e nele que a imagem e reenquadrada
total = 0.0
momento = 0.0
for y in range(alt):
    linha = sum(sum(px[x, y]) for x in range(0, LARG, 2))
    total += linha
    momento += linha * y
centro = momento / total
meia = int(min(centro, alt - 1 - centro))
topo = int(centro) - meia
im = im.crop((0, topo, LARG, topo + meia * 2))
alt = im.height
px = im.load()

dados = bytearray()
for y in range(alt):
    fy = rampa(min(y, alt - 1 - y), FUNDE_Y)
    for x in range(LARG):
        f = fy * rampa(min(x, LARG - 1 - x), FUNDE_X) * FORCA
        r, g, b = px[x, y]
        limiar = BAYER[y & 7][x & 7]
        r = max(0, r - pedestal[0]) * f
        g = max(0, g - pedestal[1]) * f
        b = max(0, b - pedestal[2]) * f
        dados += bytes((int(b + limiar), int(g + limiar), int(r + limiar)))

with open(DESTINO, 'wb') as f:
    f.write(struct.pack('<ii', LARG, alt))   # largura, altura
    f.write(dados)                           # BGR, de cima para baixo

print('pedestal', pedestal, 'centro', round(centro, 1), '->', DESTINO,
      LARG, 'x', alt, ':', 8 + len(dados), 'bytes')
