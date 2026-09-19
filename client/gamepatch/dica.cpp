// Implementacao da dica do item. Ver dica.h para de onde vem o texto.

#include "dica.h"

#include <windows.h>

#include <cstdio>
#include <cstdlib>
#include <cstring>

namespace {

// --- os atributos do item, do ItemList do cliente ---------------------------
//
// A caixa do jogo nao sai de um arquivo de texto: ela e montada a partir dos
// atributos do item, e o ItemList.bin e onde eles moram - 140 bytes por item,
// guardados com um XOR de 0x5A. Os campos abaixo foram conferidos contra o
// ItemList.csv do servidor em 3.199 itens; os 23 que divergem sao itens que o
// servidor editou e o cliente ainda nao recebeu, e nesses o que o jogador ve e
// o do cliente - que e justamente o que esta sendo lido aqui.
constexpr int kPassoItem = 140;
constexpr int kOffReqLvl = 70;   // o jogo mostra este valor MAIS UM
constexpr int kOffReqStr = 72;
constexpr int kOffReqInt = 74;
constexpr int kOffReqDex = 76;
constexpr int kOffReqCon = 78;
constexpr int kOffEfeitos = 80;  // oito pares (id, valor) de 2+2 bytes
constexpr int kMaxEfeitos = 8;

// Os efeitos que a dica sabe nomear, com o texto do strdef.txt do cliente. O
// que nao estiver aqui nao aparece: numero sem nome ao lado seria pior do que
// linha nenhuma, porque o jogador leria como se fosse outra coisa.
struct NomeEfeito {
    int id;
    const char* texto;
    bool porCento;
};

const NomeEfeito kEfeitos[] = {
    {2, "Aumento de Dano", false},
    {3, "Defesa", false},
    {4, "Aumento de HP maximo", false},
    {5, "Aumento de MP maximo", false},
    {7, "Forca", false},
    {8, "Inteligencia", false},
    {9, "Destreza", false},
    {10, "Constituicao", false},
    {26, "Aumento da Velocidade de Ataque", false},
    {29, "Aumento da Velocidade de Movimento", false},
    {42, "Critico", false},
    {45, "Indice de aumento de HP maximo", true},
    {46, "Indice de aumento de MP maximo", true},
    {47, "Indice de regeneracao de HP", false},
    {48, "Indice de regeneracao de MP", false},
    {50, "Resistencia a Fogo", false},
    {51, "Resistencia a Gelo", false},
    {52, "Resistencia a Sagrado", false},
    {53, "Defesa", false},
    {54, "Aumento de Imunidades", false},
    {60, "Ataque Magico", false},
    {67, "Aumento de Dano", false},
    {68, "Ataque Magico", false},
};

constexpr int kEfClasse = 18;    // EF_CLASS: mascara de classe
const char* const kClasses[4] = {"TransKnight", "Foema", "BeastMaster", "Huntress"};

BYTE* g_itens = nullptr;
bool g_itensLidos = false;

// O ItemList.bin, do lado do WYD.exe. E o mesmo arquivo que o cliente le na
// entrada: se ele estiver velho, a dica do jogo tambem estara.
const BYTE* Itens() {
    if (g_itensLidos) {
        return g_itens;
    }
    g_itensLidos = true;
    char caminho[MAX_PATH];
    GetModuleFileNameA(nullptr, caminho, MAX_PATH);
    char* barra = strrchr(caminho, 92);
    if (barra == nullptr) {
        return nullptr;
    }
    strcpy_s(barra + 1, MAX_PATH - (barra + 1 - caminho), "ItemList.bin");
    FILE* f = nullptr;
    if (fopen_s(&f, caminho, "rb") != 0 || f == nullptr) {
        return nullptr;
    }
    fseek(f, 0, SEEK_END);
    const long tam = ftell(f);
    fseek(f, 0, SEEK_SET);
    if (tam < kPassoItem) {
        fclose(f);
        return nullptr;
    }
    BYTE* p = static_cast<BYTE*>(malloc(static_cast<size_t>(tam)));
    if (p == nullptr) {
        fclose(f);
        return nullptr;
    }
    const size_t lidos = fread(p, 1, static_cast<size_t>(tam), f);
    fclose(f);
    for (size_t i = 0; i < lidos; ++i) {
        p[i] ^= 0x5A;
    }
    g_itens = p;
    return g_itens;
}

short CampoItem(int item, int off) {
    const BYTE* base = Itens();
    if (base == nullptr || item < 0) {
        return 0;
    }
    return *reinterpret_cast<const short*>(base + item * kPassoItem + off);
}

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

// --- a caixa, linha a linha ------------------------------------------------
//
// Montada uma vez por item e guardada, porque a dica repinta so quando o item
// sob o cursor muda. A ordem e a do jogo: nome, classe, o que exige para
// equipar, o que o item da, e a descricao no fim.
constexpr int kMaxMontadas = 24;
char g_montadas[kMaxMontadas][96];
COLORREF g_coresMontadas[kMaxMontadas];
int g_nMontadas = 0;
int g_itemMontado = -1;

void Poe(const char* texto, COLORREF cor) {
    if (g_nMontadas >= kMaxMontadas || texto == nullptr || texto[0] == 0) {
        return;
    }
    strcpy_s(g_montadas[g_nMontadas], sizeof(g_montadas[0]), texto);
    g_coresMontadas[g_nMontadas] = cor;
    ++g_nMontadas;
}

void PoeNumero(const char* rotulo, int valor, bool porCento, COLORREF cor) {
    if (valor == 0) {
        return;
    }
    char linha[96];
    sprintf_s(linha, porCento ? "%s : %d%%" : "%s : %d", rotulo, valor);
    Poe(linha, cor);
}

void Monta(int item) {
    if (g_itemMontado == item) {
        return;
    }
    g_itemMontado = item;
    g_nMontadas = 0;
    if (item <= 0 || item >= kMaxItens) {
        return;
    }
    const char* nome = reinterpret_cast<const char*>(kNomes + item * kPassoNome);
    if (nome[0] == 0) {
        return;
    }
    Poe(Limpa(nome), RGB(255, 236, 140));

    // Classe: EF_CLASS traz uma mascara, e o jogo escreve o nome dela em
    // vermelho.
    const BYTE* base = Itens();
    if (base != nullptr) {
        for (int i = 0; i < kMaxEfeitos; ++i) {
            const short id = CampoItem(item, kOffEfeitos + i * 4);
            const short valor = CampoItem(item, kOffEfeitos + i * 4 + 2);
            if (id != kEfClasse || valor == 0) {
                continue;
            }
            for (int c = 0; c < 4; ++c) {
                if ((valor & (1 << c)) != 0) {
                    char linha[96];
                    sprintf_s(linha, "Classe : %s", kClasses[c]);
                    Poe(linha, RGB(232, 72, 72));
                }
            }
        }
        const int nivel = CampoItem(item, kOffReqLvl);
        if (nivel > 0) {
            PoeNumero("Level necessario", nivel + 1, false, RGB(255, 255, 255));
        }
        PoeNumero("Forca necessaria", CampoItem(item, kOffReqStr), false, RGB(255, 255, 255));
        PoeNumero("Inteligencia necessaria", CampoItem(item, kOffReqInt), false,
                  RGB(255, 255, 255));
        PoeNumero("Destreza necessaria", CampoItem(item, kOffReqDex), false, RGB(255, 255, 255));
        PoeNumero("Constituicao necessaria", CampoItem(item, kOffReqCon), false,
                  RGB(255, 255, 255));
        for (int i = 0; i < kMaxEfeitos; ++i) {
            const short id = CampoItem(item, kOffEfeitos + i * 4);
            const short valor = CampoItem(item, kOffEfeitos + i * 4 + 2);
            if (id == 0 || valor == 0) {
                continue;
            }
            for (size_t k = 0; k < sizeof(kEfeitos) / sizeof(kEfeitos[0]); ++k) {
                if (kEfeitos[k].id == id) {
                    PoeNumero(kEfeitos[k].texto, valor, kEfeitos[k].porCento,
                              RGB(150, 230, 150));
                    break;
                }
            }
        }
    }

    // E, por fim, a descricao do itemHelp, com a cor que ela traz.
    for (int i = 0; i < kMaxLinhas; ++i) {
        const char* c = Crua(item, i);
        if (c == nullptr) {
            continue;
        }
        const char* t = Limpa(c);
        if (t[0] == 0) {
            continue;
        }
        const WORD cor = *reinterpret_cast<const WORD*>(kAjuda + item * kPassoAjuda + i * 2);
        COLORREF rgb = RGB(255, 255, 255);
        if (cor != 0) {
            rgb = RGB(((cor >> 11) & 0x1F) * 255 / 31, ((cor >> 5) & 0x3F) * 255 / 63,
                      (cor & 0x1F) * 255 / 31);
        }
        Poe(t, rgb);
    }
}

} // namespace

int DicaLinhas(int item) {
    Monta(item);
    return g_nMontadas;
}

const char* DicaLinha(int item, int i) {
    Monta(item);
    return (i >= 0 && i < g_nMontadas) ? g_montadas[i] : nullptr;
}

COLORREF DicaLinhaCor(int item, int i) {
    Monta(item);
    return (i >= 0 && i < g_nMontadas) ? g_coresMontadas[i] : RGB(255, 255, 255);
}
