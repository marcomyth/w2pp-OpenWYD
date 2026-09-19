// Implementacao dos icones de item. Ver icones.h para a cadeia de arquivos.

#include "icones.h"

#include <windows.h>

#include <cstdio>
#include <cstdlib>
#include <cstring>

void CamadaLog(const char* texto);

namespace {

constexpr int kLado = 35;           // o icone do jogo e 35x35
constexpr int kMaxItens = 6500;     // itemicon.bin
constexpr int kMaxSprites = 1000;   // o conjunto [ItemIcon]
constexpr int kMaxFolhas = 16;

struct Sprite {
    short folha;
    short x;
    short y;
};

struct Folha {
    int numero;     // o numero da textura no UITextureListN.bin
    int largura;
    int altura;
    int bytesPorPixel;
    BYTE* pixels;   // do jeito que o arquivo guarda: BGRA ou BGR
    bool tentada;
};

int* g_doItem = nullptr;   // item -> numero do icone
Sprite g_sprites[kMaxSprites];
int g_nSprites = 0;
char g_arquivoFolha[kMaxFolhas][MAX_PATH];
Folha g_folhas[kMaxFolhas];
int g_nFolhas = 0;
bool g_pronto = false;
bool g_tentado = false;

// A pasta do WYD.exe, com a barra no fim.
void Pasta(char* saida, size_t n) {
    GetModuleFileNameA(nullptr, saida, static_cast<DWORD>(n));
    char* barra = strrchr(saida, 92);
    if (barra != nullptr) {
        barra[1] = 0;
    }
}

// O nome vem do UITextureListN.bin as vezes com a barra dobrada; aqui ele vira
// um caminho comum.
void Desdobra(const char* de, char* para, size_t n) {
    size_t j = 0;
    for (size_t i = 0; de[i] != 0 && j + 1 < n; ++i) {
        if (de[i] == 92 && de[i + 1] == 92) {
            continue;
        }
        para[j++] = de[i];
    }
    para[j] = 0;
}

bool LeItemIcon(const char* pasta) {
    char caminho[MAX_PATH];
    sprintf_s(caminho, "%sitemicon.bin", pasta);
    FILE* f = nullptr;
    if (fopen_s(&f, caminho, "rb") != 0 || f == nullptr) {
        return false;
    }
    g_doItem = static_cast<int*>(malloc(sizeof(int) * kMaxItens));
    if (g_doItem == nullptr) {
        fclose(f);
        return false;
    }
    const size_t lidos = fread(g_doItem, sizeof(int), kMaxItens, f);
    fclose(f);
    for (size_t i = lidos; i < static_cast<size_t>(kMaxItens); ++i) {
        g_doItem[i] = -1;
    }
    return lidos > 0;
}

bool LeConjunto(const char* pasta) {
    char caminho[MAX_PATH];
    sprintf_s(caminho, "%sUI\\UITextureSetList.txt", pasta);
    FILE* f = nullptr;
    if (fopen_s(&f, caminho, "r") != 0 || f == nullptr) {
        return false;
    }
    char linha[256];
    bool dentro = false;
    int faltam = -1;
    while (fgets(linha, sizeof(linha), f) != nullptr) {
        if (!dentro) {
            if (strncmp(linha, "[ItemIcon]", 10) == 0) {
                dentro = true;
            }
            continue;
        }
        if (faltam < 0) {
            if (strncmp(linha, "ItemCount:", 10) == 0) {
                faltam = atoi(linha + 10);
                if (faltam > kMaxSprites) {
                    faltam = kMaxSprites;
                }
            }
            continue;
        }
        if (faltam == 0 || linha[0] == '[') {
            break;
        }
        int t = 0;
        int x = 0;
        int y = 0;
        if (sscanf_s(linha, "%d,%d,%d", &t, &x, &y) != 3) {
            break;
        }
        g_sprites[g_nSprites].folha = static_cast<short>(t);
        g_sprites[g_nSprites].x = static_cast<short>(x);
        g_sprites[g_nSprites].y = static_cast<short>(y);
        ++g_nSprites;
        --faltam;
    }
    fclose(f);
    return g_nSprites > 0;
}

// So os nomes das texturas que o conjunto usa - sao dez, nao as 512 do arquivo.
// No fim deste passo o campo folha de cada sprite deixa de ser o numero da
// textura e passa a ser o indice da folha aqui dentro.
bool LeNomes(const char* pasta) {
    char caminho[MAX_PATH];
    sprintf_s(caminho, "%sUI\\UITextureListN.bin", pasta);
    FILE* f = nullptr;
    if (fopen_s(&f, caminho, "rb") != 0 || f == nullptr) {
        return false;
    }
    const long kRegistro = 264;
    for (int i = 0; i < g_nSprites; ++i) {
        const int t = g_sprites[i].folha;
        int qual = -1;
        for (int k = 0; k < g_nFolhas; ++k) {
            if (g_folhas[k].numero == t) {
                qual = k;
                break;
            }
        }
        if (qual < 0) {
            if (g_nFolhas >= kMaxFolhas) {
                g_sprites[i].folha = -1;
                continue;
            }
            char bruto[264];
            if (fseek(f, t * kRegistro, SEEK_SET) != 0 ||
                fread(bruto, 1, sizeof(bruto), f) != sizeof(bruto)) {
                g_sprites[i].folha = -1;
                continue;
            }
            bruto[sizeof(bruto) - 1] = 0;
            char limpo[MAX_PATH];
            Desdobra(bruto, limpo, sizeof(limpo));
            qual = g_nFolhas++;
            sprintf_s(g_arquivoFolha[qual], "%s%s", pasta, limpo);
            g_folhas[qual].numero = t;
            g_folhas[qual].pixels = nullptr;
            g_folhas[qual].tentada = false;
        }
        g_sprites[i].folha = static_cast<short>(qual);
    }
    fclose(f);
    return g_nFolhas > 0;
}

void Prepara() {
    if (g_tentado) {
        return;
    }
    g_tentado = true;
    char pasta[MAX_PATH];
    Pasta(pasta, sizeof(pasta));
    if (!LeItemIcon(pasta) || !LeConjunto(pasta) || !LeNomes(pasta)) {
        CamadaLog("=== icones: nao consegui ler os arquivos do cliente");
        return;
    }
    g_pronto = true;
    char buf[120];
    sprintf_s(buf, "=== icones: %d sprites em %d folhas", g_nSprites, g_nFolhas);
    CamadaLog(buf);
}

// A folha so e lida quando o primeiro icone dela aparece.
const Folha* Carrega(int qual) {
    if (qual < 0 || qual >= g_nFolhas) {
        return nullptr;
    }
    Folha* fo = &g_folhas[qual];
    if (fo->pixels != nullptr) {
        return fo;
    }
    if (fo->tentada) {
        return nullptr;
    }
    fo->tentada = true;
    FILE* f = nullptr;
    if (fopen_s(&f, g_arquivoFolha[qual], "rb") != 0 || f == nullptr) {
        return nullptr;
    }
    BYTE cab[0x16];
    if (fread(cab, 1, sizeof(cab), f) != sizeof(cab) || memcmp(cab, "WT10", 4) != 0) {
        fclose(f);
        return nullptr;
    }
    const int l = *reinterpret_cast<WORD*>(cab + 0x10);
    const int a = *reinterpret_cast<WORD*>(cab + 0x12);
    const int bpp = cab[0x14];
    if (l <= 0 || a <= 0 || (bpp != 32 && bpp != 24)) {
        fclose(f);
        return nullptr;
    }
    const size_t total = static_cast<size_t>(l) * a * (bpp / 8);
    BYTE* px = static_cast<BYTE*>(malloc(total));
    if (px == nullptr) {
        fclose(f);
        return nullptr;
    }
    const size_t lidos = fread(px, 1, total, f);
    fclose(f);
    if (lidos != total) {
        free(px);
        return nullptr;
    }
    fo->largura = l;
    fo->altura = a;
    fo->bytesPorPixel = bpp / 8;
    fo->pixels = px;
    return fo;
}

} // namespace

int IconeLado() {
    return kLado;
}

int IconeDesenha(void* pixels, int telaL, int telaA, int x, int y, int item) {
    Prepara();
    if (!g_pronto || pixels == nullptr || item < 0 || item >= kMaxItens) {
        return 0;
    }
    const int numero = g_doItem[item];
    if (numero < 0 || numero >= g_nSprites) {
        return 0;
    }
    const Sprite& sp = g_sprites[numero];
    const Folha* fo = Carrega(sp.folha);
    if (fo == nullptr) {
        return 0;
    }
    DWORD* destino = static_cast<DWORD*>(pixels);
    for (int ly = 0; ly < kLado; ++ly) {
        const int dy = y + ly;
        const int fy = sp.y + ly;
        if (dy < 0 || dy >= telaA || fy < 0 || fy >= fo->altura) {
            continue;
        }
        for (int lx = 0; lx < kLado; ++lx) {
            const int dx = x + lx;
            const int fx = sp.x + lx;
            if (dx < 0 || dx >= telaL || fx < 0 || fx >= fo->largura) {
                continue;
            }
            const BYTE* p =
                fo->pixels + (static_cast<size_t>(fy) * fo->largura + fx) * fo->bytesPorPixel;
            const BYTE b = p[0];
            const BYTE g = p[1];
            const BYTE r = p[2];
            // Nas folhas de 32 bits o alfa e so 0 ou 255; nas de 24 quem some e
            // o preto, que e o fundo que o jogo usa nelas.
            const bool aparece = fo->bytesPorPixel == 4 ? p[3] != 0 : (r | g | b) != 0;
            if (!aparece) {
                continue;
            }
            destino[static_cast<size_t>(dy) * telaL + dx] =
                (static_cast<DWORD>(r) << 16) | (static_cast<DWORD>(g) << 8) | b;
        }
    }
    return 1;
}
