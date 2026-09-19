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
void Degrade(HDC hdc, int x, int y, int l, int a, COLORREF topo, COLORREF baixo) {
    if (a <= 0) {
        return;
    }
    for (int i = 0; i < a; ++i) {
        const int r = GetRValue(topo) + (GetRValue(baixo) - GetRValue(topo)) * i / a;
        const int g = GetGValue(topo) + (GetGValue(baixo) - GetGValue(topo)) * i / a;
        const int b = GetBValue(topo) + (GetBValue(baixo) - GetBValue(topo)) * i / a;
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
void Moldura(HDC hdc, int L, int A) {
    Contorno(hdc, 0, 0, L, A, kBorda1);
    Contorno(hdc, 1, 1, L - 2, A - 2, kBorda2);
    Contorno(hdc, 2, 2, L - 4, A - 4, kBorda3);
    Degrade(hdc, 3, 3, L - 6, A - 6, kFundoTopo, kFundoBaixo);
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
