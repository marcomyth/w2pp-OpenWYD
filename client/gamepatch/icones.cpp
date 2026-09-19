// Implementacao dos icones de item. Ver icones.h para a cadeia, toda dentro do
// cliente.

#include "icones.h"

#include <windows.h>

#include <d3d9.h>

#include <cstdio>
#include <cstring>

void CamadaLog(const char* texto);

namespace {

// A tabela item -> numero do icone, ja lida pelo cliente: 0x4B81AE despeja o
// itemicon.bin inteiro aqui.
constexpr DWORD kTabelaItemIcone = 0x006EA518;
constexpr int kMaxItens = 6500;

// O conjunto [ItemIcon] e o 526 - e o proprio cliente que usa esse numero, em
// 0x40D6BF.
constexpr int kConjuntoIcone = 526;
constexpr DWORD kListaNoObjeto = 0x15E8;    // o UITextureListN.bin inteiro
constexpr DWORD kContagemDoConjunto = 0x328;
constexpr DWORD kVetorDoConjunto = 0x32C;
constexpr DWORD kTexturas = 0xE85E8;
constexpr int kBytesPorEntrada = 28;

// O comeco do UITextureListN.bin: "UI\\cursor.wyt" e o enchimento que sobra no
// resto do registro.
const BYTE kMarca[] = {'U', 'I', 92, 92, 'c', 'u', 'r', 's', 'o', 'r', '.', 'w', 'y', 't', 0,
                       0xCD, 0xCD};

BYTE* g_gerenciador = nullptr;
bool g_procurado = false;

// Varre a memoria do processo atras do objeto, uma vez so. Ele e grande, mora no
// heap e nao tem ponteiro global que valha a pena caçar.
BYTE* Gerenciador() {
    if (g_procurado) {
        return g_gerenciador;
    }
    g_procurado = true;
    SYSTEM_INFO si;
    GetSystemInfo(&si);
    BYTE* p = static_cast<BYTE*>(si.lpMinimumApplicationAddress);
    BYTE* fim = static_cast<BYTE*>(si.lpMaximumApplicationAddress);
    while (p < fim) {
        MEMORY_BASIC_INFORMATION mbi;
        if (VirtualQuery(p, &mbi, sizeof(mbi)) == 0) {
            break;
        }
        const bool legivel =
            mbi.State == MEM_COMMIT &&
            (mbi.Protect == PAGE_READWRITE || mbi.Protect == PAGE_READONLY);
        if (legivel && mbi.RegionSize < 0x4000000) {
            BYTE* ini = static_cast<BYTE*>(mbi.BaseAddress);
            const size_t n = mbi.RegionSize;
            for (size_t i = 0; i + sizeof(kMarca) < n; ++i) {
                if (ini[i] != 'U' || memcmp(ini + i, kMarca, sizeof(kMarca)) != 0) {
                    continue;
                }
                BYTE* obj = ini + i - kListaNoObjeto;
                // Confere pelo que interessa: o conjunto dos icones tem de estar
                // preenchido e caber no mundo.
                if (IsBadReadPtr(obj + kContagemDoConjunto + kConjuntoIcone * 8, 8)) {
                    continue;
                }
                const int qtd =
                    *reinterpret_cast<int*>(obj + kContagemDoConjunto + kConjuntoIcone * 8);
                if (qtd > 0 && qtd <= 4000) {
                    g_gerenciador = obj;
                    char buf[140];
                    sprintf_s(buf, "=== icones: gerenciador do cliente em %08X, %d sprites",
                              reinterpret_cast<DWORD>(obj), qtd);
                    CamadaLog(buf);
                    return g_gerenciador;
                }
            }
        }
        p = static_cast<BYTE*>(mbi.BaseAddress) + mbi.RegionSize;
    }
    CamadaLog("=== icones: nao achei o gerenciador de texturas do cliente");
    return nullptr;
}

// O retangulo do icone do item, como o cliente o tem: qual textura e onde nela.
bool Sprite(int item, IDirect3DTexture9** textura, int* x, int* y, int* l, int* a) {
    if (item < 0 || item >= kMaxItens) {
        return false;
    }
    BYTE* obj = Gerenciador();
    if (obj == nullptr) {
        return false;
    }
    // O cliente tira 1: o valor 0 quer dizer "sem icone", nao "o primeiro".
    const int numero = reinterpret_cast<const int*>(kTabelaItemIcone)[item] - 1;
    if (numero < 0) {
        return false;
    }
    const int qtd = *reinterpret_cast<int*>(obj + kContagemDoConjunto + kConjuntoIcone * 8);
    BYTE* vetor = *reinterpret_cast<BYTE**>(obj + kVetorDoConjunto + kConjuntoIcone * 8);
    if (vetor == nullptr || numero >= qtd) {
        return false;
    }
    const int* e = reinterpret_cast<const int*>(vetor + numero * kBytesPorEntrada);
    const int tex = e[0];
    if (tex < 0 || tex >= 600 || e[3] <= 0 || e[4] <= 0) {
        return false;
    }
    IDirect3DTexture9* t = *reinterpret_cast<IDirect3DTexture9**>(obj + kTexturas + tex * 4);
    if (t == nullptr) {
        return false;
    }
    *textura = t;
    *x = e[1];
    *y = e[2];
    *l = e[3];
    *a = e[4];
    return true;
}

} // namespace

int IconeDesenha(void* pixels, int telaL, int telaA, int quadX, int quadY, int quadL, int quadA,
                 int item) {
    IDirect3DTexture9* textura = nullptr;
    int sx = 0;
    int sy = 0;
    int sl = 0;
    int sa = 0;
    if (pixels == nullptr || !Sprite(item, &textura, &sx, &sy, &sl, &sa)) {
        return 0;
    }
    D3DSURFACE_DESC desc;
    if (FAILED(textura->GetLevelDesc(0, &desc))) {
        return 0;
    }
    // So os dois formatos de 32 bits que o cliente pede ao D3DX em 0x4BE9E9:
    // com alfa, e sem - e ai quem some e o preto, a cor-chave dele.
    const bool temAlfa = desc.Format == D3DFMT_A8R8G8B8;
    if (!temAlfa && desc.Format != D3DFMT_X8R8G8B8) {
        return 0;
    }
    if (sx < 0 || sy < 0 || sx + sl > static_cast<int>(desc.Width) ||
        sy + sa > static_cast<int>(desc.Height)) {
        return 0;
    }
    // A textura e MANAGED (o cliente cria assim), entao tem copia em memoria de
    // sistema e pode ser lida sem tirar nada da placa.
    D3DLOCKED_RECT lr;
    RECT r = {sx, sy, sx + sl, sy + sa};
    if (FAILED(textura->LockRect(0, &lr, &r, D3DLOCK_READONLY))) {
        return 0;
    }
    const int destX = quadX + (quadL - sl) / 2;
    const int destY = quadY + (quadA - sa) / 2;
    DWORD* destino = static_cast<DWORD*>(pixels);
    for (int ly = 0; ly < sa; ++ly) {
        const int dy = destY + ly;
        if (dy < 0 || dy >= telaA) {
            continue;
        }
        const DWORD* linha =
            reinterpret_cast<const DWORD*>(static_cast<BYTE*>(lr.pBits) + ly * lr.Pitch);
        for (int lx = 0; lx < sl; ++lx) {
            const int dx = destX + lx;
            if (dx < 0 || dx >= telaL) {
                continue;
            }
            const DWORD cor = linha[lx];
            const bool aparece = temAlfa ? (cor >> 24) != 0 : (cor & 0x00FFFFFF) != 0;
            if (aparece) {
                destino[static_cast<size_t>(dy) * telaL + dx] = cor & 0x00FFFFFF;
            }
        }
    }
    textura->UnlockRect(0);
    return 1;
}
