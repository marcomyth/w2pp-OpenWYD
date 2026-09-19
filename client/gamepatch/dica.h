// A dica do item - a caixa que o jogo mostra quando o mouse para sobre um item.
//
// O texto nao e nosso: sao as duas tabelas que o cliente carrega na entrada e
// usa para desenhar a dica dele.
//
//   0xFB9608    nome do item, 0x8C bytes por item (Itemname.bin, lido em
//               0x4B82F2).
//   0x11F9198   a descricao, 0x514 bytes por item (itemHelp.dat, lido em
//               0x54AF99): dez linhas de 128 bytes a partir de +0x14, e dez
//               palavras de cor a partir de +0x00.
//
// No arquivo o espaco e escrito como sublinhado; o desenho desfaz isso, como o
// jogo faz.

#ifndef DICA_H
#define DICA_H

#include <windows.h>

// Quantas linhas a dica deste item tem (0 = nao ha dica). Serve para medir a
// caixa antes de pintar. A ordem e a do jogo: nome, classe, exigencias de
// equipar, os efeitos do item e, por fim, a descricao do itemHelp.
int DicaLinhas(int item);

// A linha i, ja com os espacos no lugar dos sublinhados. Devolve nulo quando a
// linha nao existe.
const char* DicaLinha(int item, int i);

// A cor da linha, como o cliente a guarda junto do texto: uma palavra de 16
// bits em R5G6B5, uma por linha, a partir do inicio do bloco do item. E ela que
// deixa o "[Item Composto]" amarelo e a descricao branca.
COLORREF DicaLinhaCor(int item, int i);

#endif
