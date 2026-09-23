// Implementacao dos pinceis. Ver pincel.h para o porque da paleta.

#include "pincel.h"

#include <cstring>

void Barra(HDC hdc, int x, int y, int l, int a, COLORREF cor) {
    if (l <= 0 || a <= 0) {
        return;
    }
    RECT r = {x, y, x + l, y + a};
    HBRUSH b = CreateSolidBrush(cor);
    FillRect(hdc, &r, b);
    DeleteObject(b);
}

// Degradê vertical linha a linha: sem isto o painel fica chapado, e o layout do
// Marco vive de degradê.
// A conta obvia - cortar cada linha para o inteiro mais proximo - deixa um risco
// de ponta a ponta onde um valor troca pelo outro. E o que se via no fundo da
// loja: 538 linhas entre 22 e 10 tem so doze valores para percorrer, entao o
// degrade virava doze faixas chapadas com uma linha visivel entre elas, no
// escuro, justamente onde nao ha desenho nenhum para disfarcar.
//
// A saida e espalhar a troca em vez de faze-la de uma vez: perto da fronteira as
// linhas alternam entre os dois valores numa ordem fixa (Bayer), e uma faixa
// dissolve na seguinte ao longo de varias linhas. Um degrau de 1/255 alternando
// linha sim linha nao nao se enxerga; o risco reto se enxerga.
namespace {

// A ordem de Bayer em uma dimensao: o quanto de resto cada linha exige para
// subir um nivel, espalhado para que linhas vizinhas nunca subam juntas.
const int kOrdem[8] = {0, 4, 2, 6, 1, 5, 3, 7};

// A mesma ideia em duas dimensoes: a matriz de Bayer 8x8, valores de 0 a 63.
// Numa faixa larga o espalhamento so por linha nao basta - ele troca o risco
// reto por listras de ponta a ponta, que se veem igual. Espalhando tambem em x,
// a troca vira textura fina e some.
const int kBayer[8][8] = {
    { 0, 32,  8, 40,  2, 34, 10, 42},
    {48, 16, 56, 24, 50, 18, 58, 26},
    {12, 44,  4, 36, 14, 46,  6, 38},
    {60, 28, 52, 20, 62, 30, 54, 22},
    { 3, 35, 11, 43,  1, 33,  9, 41},
    {51, 19, 59, 27, 49, 17, 57, 25},
    {15, 47,  7, 39, 13, 45,  5, 37},
    {63, 31, 55, 23, 61, 29, 53, 21},
};

int Nivel(int de, int para, int i, int a) {
    const int passo = (para - de) * i;
    int v = passo / a;
    int resto = passo - v * a;
    if (resto < 0) {   // em C a divisao corta na direcao do zero; aqui tem de ser para baixo
        --v;
        resto += a;
    }
    if (resto * 8 > kOrdem[i & 7] * a) {
        ++v;
    }
    return de + v;
}

} // namespace

void Degrade(HDC hdc, int x, int y, int l, int a, COLORREF topo, COLORREF baixo) {
    if (a <= 0) {
        return;
    }
    for (int i = 0; i < a; ++i) {
        const int r = Nivel(GetRValue(topo), GetRValue(baixo), i, a);
        const int g = Nivel(GetGValue(topo), GetGValue(baixo), i, a);
        const int b = Nivel(GetBValue(topo), GetBValue(baixo), i, a);
        Barra(hdc, x, y + i, l, 1, RGB(r, g, b));
    }
}

void Contorno(HDC hdc, int x, int y, int l, int a, COLORREF cor) {
    Barra(hdc, x, y, l, 1, cor);
    Barra(hdc, x, y + a - 1, l, 1, cor);
    Barra(hdc, x, y, 1, a, cor);
    Barra(hdc, x + l - 1, y, 1, a, cor);
}

void Losango(HDC hdc, int cx, int cy, int r, COLORREF cor) {
    for (int i = -r; i <= r; ++i) {
        const int meia = r - (i < 0 ? -i : i);
        Barra(hdc, cx - meia, cy + i, meia * 2 + 1, 1, cor);
    }
}

// Tampa metálica das pontas da linha, como nos botões do jogo.
void TampaMetal(HDC hdc, int x, int y, int a, int larg) {
    for (int i = 0; i < larg; ++i) {
        const int meio = larg / 2;
        const COLORREF de = i <= meio ? kTampaClara : RGB(135, 116, 80);
        const COLORREF para = i <= meio ? RGB(135, 116, 80) : kTampaEscura;
        const int passo = i <= meio ? i : i - meio;
        const int total = meio > 0 ? meio : 1;
        const int r = GetRValue(de) + (GetRValue(para) - GetRValue(de)) * passo / total;
        const int g = GetGValue(de) + (GetGValue(para) - GetGValue(de)) * passo / total;
        const int b = GetBValue(de) + (GetBValue(para) - GetBValue(de)) * passo / total;
        Barra(hdc, x + i, y, 1, a, RGB(r, g, b));
    }
    Contorno(hdc, x, y, larg, a, kTampaBorda);
    Barra(hdc, x + 1, y + 1, larg - 2, 1, RGB(255, 255, 255));
}

void LinhaBotao(HDC hdc, int x, int y, int l, int a, bool ativo) {
    const int tampa = l < 120 ? 6 : kTampa;
    Degrade(hdc, x, y, l, a, ativo ? kBtnAtivoTopo : kBtnTopo, ativo ? kBtnAtivoBaixo : kBtnBaixo);
    Barra(hdc, x, y, l, 1, kBtnBordaTopo);
    Barra(hdc, x, y + a - 1, l, 1, kBtnBordaBaixo);
    TampaMetal(hdc, x + 2, y + 2, a - 4, tampa);
    TampaMetal(hdc, x + l - tampa - 2, y + 2, a - 4, tampa);
}


// A moldura do painel de alvos virou funcao comum quando a loja apareceu: as
// duas caixas tem a mesma borda tripla, o mesmo fundo e os mesmos cantos.
// O fundo do painel escrito nos pixels, e nao pelo GDI.
//
// O GDI pinta uma linha inteira de uma cor so, entao o unico espalhamento
// possivel por ele e de linha em linha - e isso troca o risco por listras. Aqui
// cada pixel decide sozinho, com a matriz de Bayer, se sobe um nivel ou nao: a
// faixa dissolve na seguinte como poeira, e nao ha linha nenhuma para o olho
// achar. So vale a pena para a area grande do painel; nos degrades curtos (um
// cabecalho, um quadrado de item) o do GDI basta.
static void FundoDoPainel(Tela* t, int x, int y, int L, int A, COLORREF topo, COLORREF baixo) {
    if (t->pixels == nullptr || A <= 0) {
        return;
    }
    GdiFlush();
    BYTE* tela = static_cast<BYTE*>(t->pixels);
    const int canal[3] = {GetBValue(topo), GetGValue(topo), GetRValue(topo)};
    const int fim[3] = {GetBValue(baixo), GetGValue(baixo), GetRValue(baixo)};
    for (int i = 0; i < A; ++i) {
        const int ty = y + i;
        if (ty < 0 || ty >= t->a) {
            continue;
        }
        BYTE* linha = tela + static_cast<size_t>(ty) * t->l * 4;
        for (int j = 0; j < L; ++j) {
            const int tx = x + j;
            if (tx < 0 || tx >= t->l) {
                continue;
            }
            BYTE* p = linha + static_cast<size_t>(tx) * 4;
            const int limiar = kBayer[i & 7][j & 7];
            for (int k = 0; k < 3; ++k) {
                const int passo = (fim[k] - canal[k]) * i;
                int v = passo / A;
                int resto = passo - v * A;
                if (resto < 0) {
                    --v;
                    resto += A;
                }
                if (resto * 64 > limiar * A) {
                    ++v;
                }
                p[k] = static_cast<BYTE>(canal[k] + v);
            }
        }
    }
}

void Moldura(Tela* t) {
    HDC hdc = t->dc;
    const int L = t->l;
    const int A = t->a;
    Contorno(hdc, 0, 0, L, A, kBorda1);
    Contorno(hdc, 1, 1, L - 2, A - 2, kBorda2);
    Contorno(hdc, 2, 2, L - 4, A - 4, kBorda3);
    FundoDoPainel(t, 3, 3, L - 6, A - 6, kFundoTopo, kFundoBaixo);
    Contorno(hdc, 3, 3, L - 6, A - 6, kBordaInterna);

    const int c = 12;
    const int p = 4;
    Barra(hdc, p, p, c, 2, kCanto);
    Barra(hdc, p, p, 2, c, kCanto);
    Barra(hdc, L - p - c, p, c, 2, kCanto);
    Barra(hdc, L - p - 2, p, 2, c, kCanto);
    Barra(hdc, p, A - p - 2, c, 2, kCanto);
    Barra(hdc, p, A - p - c, 2, c, kCanto);
    Barra(hdc, L - p - c, A - p - 2, c, 2, kCanto);
    Barra(hdc, L - p - 2, A - p - c, 2, c, kCanto);
}

bool TelaGarante(Tela* t, int l, int a) {
    if (t->pixels != nullptr && l == t->l && a == t->a) {
        return true;
    }
    if (t->bmp != nullptr) {
        SelectObject(t->dc, t->bmpAntigo);
        DeleteObject(t->bmp);
        t->bmp = nullptr;
        t->pixels = nullptr;
    }
    if (t->dc == nullptr) {
        t->dc = CreateCompatibleDC(nullptr);
    }
    if (t->dc == nullptr || l <= 0 || a <= 0) {
        return false;
    }
    BITMAPINFO bi;
    memset(&bi, 0, sizeof(bi));
    bi.bmiHeader.biSize = sizeof(bi.bmiHeader);
    bi.bmiHeader.biWidth = l;
    bi.bmiHeader.biHeight = -a;   // negativo: a primeira linha e a de cima
    bi.bmiHeader.biPlanes = 1;
    bi.bmiHeader.biBitCount = 32;
    bi.bmiHeader.biCompression = BI_RGB;
    t->bmp = CreateDIBSection(t->dc, &bi, DIB_RGB_COLORS, &t->pixels, nullptr, 0);
    if (t->bmp == nullptr) {
        t->pixels = nullptr;
        return false;
    }
    t->bmpAntigo = SelectObject(t->dc, t->bmp);
    t->l = l;
    t->a = a;
    return true;
}

void TelaFecha(Tela* t, BYTE alfa) {
    if (t->pixels == nullptr) {
        return;
    }
    GdiFlush();
    DWORD* p = static_cast<DWORD*>(t->pixels);
    const DWORD canal = static_cast<DWORD>(alfa) << 24;
    const int total = t->l * t->a;
    for (int i = 0; i < total; ++i) {
        p[i] = (p[i] & 0x00FFFFFF) | canal;
    }
}

void TelaFechaChave(Tela* t, COLORREF chave, BYTE alfa) {
    if (t->pixels == nullptr) {
        return;
    }
    GdiFlush();
    // O DIB guarda BGRX; a chave vem em COLORREF, que e 0x00BBGGRR.
    const DWORD alvo = (static_cast<DWORD>(GetRValue(chave)) << 16) |
                       (static_cast<DWORD>(GetGValue(chave)) << 8) |
                       static_cast<DWORD>(GetBValue(chave));
    DWORD* p = static_cast<DWORD*>(t->pixels);
    const DWORD canal = static_cast<DWORD>(alfa) << 24;
    const int total = t->l * t->a;
    for (int i = 0; i < total; ++i) {
        const DWORD cor = p[i] & 0x00FFFFFF;
        p[i] = cor == alvo ? 0u : (cor | canal);
    }
}
