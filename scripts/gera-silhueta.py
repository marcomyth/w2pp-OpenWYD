"""Gera a silhueta do mundo a partir do AttributeMap.dat do jogo.

O painel desenhava o mapa como um quadrado vazio com cinco circulos: nao havia
nada que dissesse onde o mundo comeca e acaba. O AttributeMap.dat e a grade de
atributos do terreno (1024x1024, um atributo por bloco 4x4 do HeightMap), e as
celulas nao-zero desenham a forma real do mundo construido — os aglomerados
batem com as cidades.

Nao dependo do significado dos bits, que o proprio repositorio marca como
UNVERIFIED: uso so "tem atributo" contra "nao tem", que separa mundo de vazio.

A saida e um template Go embutido no painel. E asset do jogo, nao muda em
tempo de execucao — mudar o mapa exige um patch de cliente — entao gerar uma
vez e embutir e mais honesto do que uma chamada por carga de pagina.

Uso, a partir da raiz do repositorio:

    python scripts/gera-silhueta.py > adminserver/internal/panel/ui/_mundo.html
"""
import sys

ORIGEM = "Release/TMsrv/run/AttributeMap.dat"
DIM = 1024        # lado do AttributeMap
MUNDO = 4096      # lado do mundo, em passos
N = 192           # resolucao da silhueta; 192 da 422 retangulos e ~9 KB


def cheia(d, gx, gy, passo):
    """A celula da silhueta tem algum atributo?"""
    salto = max(1, passo // 2)
    for yy in range(gy * passo, (gy + 1) * passo, salto):
        base = yy * DIM
        for xx in range(gx * passo, (gx + 1) * passo, salto):
            if d[base + xx]:
                return True
    return False


def main():
    d = open(ORIGEM, "rb").read()
    if len(d) != DIM * DIM:
        sys.exit("AttributeMap.dat com tamanho inesperado: %d" % len(d))

    passo = DIM // N
    escala = MUNDO / N
    partes = []
    for gy in range(N):
        gx = 0
        while gx < N:
            if not cheia(d, gx, gy, passo):
                gx += 1
                continue
            ini = gx
            while gx < N and cheia(d, gx, gy, passo):
                gx += 1
            # Uma faixa horizontal por corrida, em vez de um retangulo por
            # celula: o dado e agrupado, entao isso corta o desenho em tres.
            x, y = int(ini * escala), int(gy * escala)
            larg, alt = int((gx - ini) * escala), int(escala) + 1
            partes.append("M%d %dh%dv%dh-%dz" % (x, y, larg, alt, larg))

    caminho = "".join(partes)
    out = sys.stdout
    out.write('{{/* A silhueta do mundo, gerada do AttributeMap.dat do jogo\n')
    out.write('     (scripts/gera-silhueta.py). %d faixas em %d de resolucao.\n' % (len(partes), N))
    out.write('\n')
    out.write('     Sao as celulas que tem algum atributo de terreno, e nao o que\n')
    out.write('     cada bit quer dizer: o significado deles esta marcado como\n')
    out.write('     UNVERIFIED no repositorio, e "tem atributo" ja separa mundo de\n')
    out.write('     vazio, que e tudo que um fundo de mapa precisa saber.\n')
    out.write('\n')
    out.write('     Embutido em vez de lido em tempo de execucao: e asset do jogo,\n')
    out.write('     so muda com patch de cliente. */}}\n')
    out.write('{{define "silhueta-do-mundo"}}\n')
    out.write('<path class="terreno" d="%s"></path>\n' % caminho)
    out.write('{{end}}\n')


if __name__ == "__main__":
    main()
