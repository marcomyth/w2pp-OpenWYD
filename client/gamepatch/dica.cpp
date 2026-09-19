// Implementacao da dica do item. Ver dica.h para de onde vem o texto.

#include "dica.h"

#include <windows.h>

#include <cstring>

namespace {

constexpr DWORD kNomes = 0x00FB9608;     // Itemname.bin ja lido
constexpr int kPassoNome = 0x8C;
constexpr DWORD kAjuda = 0x011F9198;     // itemHelp.dat ja lido
constexpr int kPassoAjuda = 0x514;
constexpr int kTextoNoBloco = 0x14;      // as dez linhas comecam aqui
constexpr int kPassoLinha = 128;
constexpr int kMaxLinhas = 10;
constexpr int kMaxItens = 6500;

// A linha crua, dentro da tabela do cliente.
const char* Crua(int item, int i) {
    if (item <= 0 || item >= kMaxItens || i < 0 || i >= kMaxLinhas) {
        return nullptr;
    }
    const char* p = reinterpret_cast<const char*>(kAjuda + item * kPassoAjuda + kTextoNoBloco +
                                                  i * kPassoLinha);
    return p[0] != 0 ? p : nullptr;
}

// O sublinhado e como o arquivo escreve o espaco. A copia sai limpa, e e ela
// que vai para a tela.
const char* Limpa(const char* cru) {
    static char buf[kPassoLinha + 1];
    int j = 0;
    for (int i = 0; i < kPassoLinha && cru[i] != 0; ++i) {
        buf[j++] = cru[i] == '_' ? ' ' : cru[i];
    }
    buf[j] = 0;
    // Espaco sobrando no fim atrapalha a medida da caixa.
    while (j > 0 && buf[j - 1] == ' ') {
        buf[--j] = 0;
    }
    return buf;
}

} // namespace

int DicaLinhas(int item) {
    if (item <= 0 || item >= kMaxItens) {
        return 0;
    }
    const char* nome = reinterpret_cast<const char*>(kNomes + item * kPassoNome);
    if (nome[0] == 0) {
        return 0;
    }
    // A primeira linha e sempre o nome; as outras vem da tabela de ajuda, e a
    // contagem para na primeira vazia do fim.
    int ultima = 0;
    for (int i = 0; i < kMaxLinhas; ++i) {
        const char* c = Crua(item, i);
        if (c != nullptr && Limpa(c)[0] != 0) {
            ultima = i + 1;
        }
    }
    return 1 + ultima;
}

const char* DicaLinha(int item, int i) {
    if (i == 0) {
        if (item <= 0 || item >= kMaxItens) {
            return nullptr;
        }
        const char* nome = reinterpret_cast<const char*>(kNomes + item * kPassoNome);
        return nome[0] != 0 ? Limpa(nome) : nullptr;
    }
    const char* c = Crua(item, i - 1);
    return c != nullptr ? Limpa(c) : nullptr;
}

COLORREF DicaLinhaCor(int item, int i) {
    // A linha 0 e o nome, que o jogo escreve em branco; as outras trazem a cor
    // do arquivo. No bloco do item as palavras de cor vem antes do texto, uma
    // por linha - e o cliente as le assim em 0x416F0E.
    if (i <= 0 || item <= 0 || item >= kMaxItens || i > kMaxLinhas) {
        return RGB(255, 255, 255);
    }
    const WORD c = *reinterpret_cast<const WORD*>(kAjuda + item * kPassoAjuda + (i - 1) * 2);
    if (c == 0) {
        return RGB(255, 255, 255);
    }
    // R5G6B5: cinco bits de vermelho, seis de verde, cinco de azul.
    const int r = ((c >> 11) & 0x1F) * 255 / 31;
    const int g = ((c >> 5) & 0x3F) * 255 / 63;
    const int b = (c & 0x1F) * 255 / 31;
    return RGB(r, g, b);
}
