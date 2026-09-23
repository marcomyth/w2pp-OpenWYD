// A marca do servidor no fundo do painel da loja.
//
// O painel e pintado com GDI num DIB de 32 bits e so depois sobe como textura.
// A marca nao pode ser pintada com GDI: o desenho e um brilho, nao um retangulo
// com cor, e o GDI nao sabe somar luz. Entao ela e escrita direto nos pixels,
// do mesmo jeito que os icones dos itens (icones.cpp) - com um GdiFlush antes,
// para que o que o GDI ja desenhou esteja mesmo na memoria.
//
// A soma e o que faz a arte pertencer ao painel em vez de ficar colada sobre
// ele: onde a arte e escura, nada muda e a moldura continua aparecendo; onde
// ela brilha, o brilho entra. Por isso arte/gera_fundo.py desconta o fundo
// proprio da imagem antes de grava-la - o que sobra e so luz.

#include "fundo.h"

#include <windows.h>

namespace {

#define kFundoLoja 100

struct Marca {
    const BYTE* bgr;
    int l;
    int a;
};

// Recurso do DLL: fica mapeado enquanto o modulo existir, entao basta achar uma
// vez e guardar o ponteiro.
const Marca* Carrega() {
    static Marca marca = {nullptr, 0, 0};
    static bool procurou = false;
    if (procurou) {
        return marca.bgr != nullptr ? &marca : nullptr;
    }
    procurou = true;

    HMODULE modulo = nullptr;
    if (!GetModuleHandleExA(GET_MODULE_HANDLE_EX_FLAG_FROM_ADDRESS |
                                GET_MODULE_HANDLE_EX_FLAG_UNCHANGED_REFCOUNT,
                            reinterpret_cast<LPCSTR>(&Carrega), &modulo)) {
        return nullptr;
    }
    HRSRC achado = FindResourceA(modulo, MAKEINTRESOURCEA(kFundoLoja), RT_RCDATA);
    if (achado == nullptr) {
        return nullptr;
    }
    const DWORD tamanho = SizeofResource(modulo, achado);
    HGLOBAL bloco = LoadResource(modulo, achado);
    if (bloco == nullptr || tamanho < 8) {
        return nullptr;
    }
    const BYTE* dados = static_cast<const BYTE*>(LockResource(bloco));
    if (dados == nullptr) {
        return nullptr;
    }
    const int l = *reinterpret_cast<const int*>(dados);
    const int a = *reinterpret_cast<const int*>(dados + 4);
    if (l <= 0 || a <= 0 || static_cast<DWORD>(l) * a * 3 + 8 != tamanho) {
        return nullptr;
    }
    marca.bgr = dados + 8;
    marca.l = l;
    marca.a = a;
    return &marca;
}

BYTE Soma(BYTE tinha, BYTE vem) {
    const int total = tinha + vem;
    return total > 255 ? 255 : static_cast<BYTE>(total);
}

} // namespace

int FundoDesenha(void* pixels, int telaL, int telaA, int centroX, int centroY) {
    const Marca* marca = Carrega();
    if (pixels == nullptr || marca == nullptr) {
        return 0;
    }
    GdiFlush();

    const int esq = centroX - marca->l / 2;
    const int topo = centroY - marca->a / 2;
    BYTE* tela = static_cast<BYTE*>(pixels);

    for (int y = 0; y < marca->a; ++y) {
        const int ty = topo + y;
        if (ty < 0 || ty >= telaA) {
            continue;
        }
        const BYTE* linha = marca->bgr + static_cast<size_t>(y) * marca->l * 3;
        BYTE* saida = tela + static_cast<size_t>(ty) * telaL * 4;
        for (int x = 0; x < marca->l; ++x) {
            const int tx = esq + x;
            if (tx < 0 || tx >= telaL) {
                continue;
            }
            BYTE* p = saida + static_cast<size_t>(tx) * 4;
            p[0] = Soma(p[0], linha[x * 3 + 0]);
            p[1] = Soma(p[1], linha[x * 3 + 1]);
            p[2] = Soma(p[2], linha[x * 3 + 2]);
        }
    }
    return 1;
}
