// Registro das camadas e dono unico do mouse.
//
// O cliente le o mouse por DirectInput (0x4B50AB), direto do dispositivo: janela
// por cima e foco nao bloqueiam nada, e era por isso que o personagem caminhava
// para o ponto clicado. O desvio entra logo APOS a leitura (0x4B50EF).
//
// Como nenhuma camada tem janela propria, nao existe WM_LBUTTONDOWN: este e o
// unico lugar onde o clique e lido. Ele decide quem fica com o clique - a camada
// de cima, se o cursor estiver sobre ela, e nesse caso os bytes de botao sao
// zerados antes que o jogo os interprete.

#include "camadas.h"

#include <cstdio>
#include <cstring>

namespace {

constexpr int kMaxCamadas = 8;

const Camada* g_camadas[kMaxCamadas];
int g_nCamadas = 0;

int g_telaL = 0;
int g_telaA = 0;
bool g_botaoAntes = false;
bool g_apertoNosso = false;   // o aperto em curso comecou numa camada
HWND g_jogo = nullptr;

} // namespace

void CamadaLog(const char* texto) {
    char caminho[MAX_PATH];
    GetModuleFileNameA(nullptr, caminho, MAX_PATH);
    char* barra = strrchr(caminho, 92);
    if (barra == nullptr) {
        return;
    }
    strcpy_s(barra + 1, MAX_PATH - (barra + 1 - caminho), "alvos.log");
    FILE* f = nullptr;
    if (fopen_s(&f, caminho, "a") == 0 && f != nullptr) {
        fputs(texto, f);
        fputc(10, f);
        fclose(f);
    }
}

// Insere mantendo a ordem crescente: a ordem de registro entre modulos nao e
// garantida pelo C++, entao quem manda e o campo ordem.
void CamadaRegistra(const Camada* c) {
    if (c == nullptr || g_nCamadas >= kMaxCamadas) {
        return;
    }
    int i = g_nCamadas;
    while (i > 0 && g_camadas[i - 1]->ordem > c->ordem) {
        g_camadas[i] = g_camadas[i - 1];
        --i;
    }
    g_camadas[i] = c;
    ++g_nCamadas;
}

int CamadaTotal() {
    return g_nCamadas;
}

const Camada* CamadaEm(int i) {
    return (i >= 0 && i < g_nCamadas) ? g_camadas[i] : nullptr;
}

HWND CamadaJanelaDoJogo() {
    if (g_jogo != nullptr && IsWindow(g_jogo)) {
        return g_jogo;
    }
    const DWORD meu = GetCurrentProcessId();
    HWND h = nullptr;
    while ((h = FindWindowExA(nullptr, h, nullptr, nullptr)) != nullptr) {
        DWORD pid = 0;
        GetWindowThreadProcessId(h, &pid);
        if (pid == meu && IsWindowVisible(h) && GetWindow(h, GW_OWNER) == nullptr) {
            RECT rc;
            if (GetClientRect(h, &rc) && rc.right > 400 && rc.bottom > 300) {
                g_jogo = h;
                return h;
            }
        }
    }
    return nullptr;
}

extern "C" void __cdecl CamadaTela(int largura, int altura) {
    g_telaL = largura;
    g_telaA = altura;
}

namespace {

// O mouse vem em coordenadas da janela e as camadas vivem em coordenadas do
// backbuffer; em tela cheia os dois podem ter tamanhos diferentes, dai a regra
// de tres.
bool CursorNaTela(int* cx, int* cy) {
    if (g_telaL <= 0 || g_telaA <= 0) {
        return false;
    }
    HWND jogo = CamadaJanelaDoJogo();
    POINT p;
    RECT cli;
    if (jogo == nullptr || !GetCursorPos(&p) || !ScreenToClient(jogo, &p) ||
        !GetClientRect(jogo, &cli) || cli.right <= 0 || cli.bottom <= 0) {
        return false;
    }
    *cx = MulDiv(p.x, g_telaL, cli.right);
    *cy = MulDiv(p.y, g_telaA, cli.bottom);
    return true;
}

} // namespace

namespace {

struct Amostra {
    int x;
    int y;
    bool pedida;
    DWORD cor;
};

Amostra g_amostras[kMaxAmostras];
bool g_urgente = false;

} // namespace

void AmostraUrgente() {
    g_urgente = true;
}

int AmostraConsomeUrgencia() {
    const bool tinha = g_urgente;
    g_urgente = false;
    return tinha ? 1 : 0;
}

void AmostraPede(int id, int x, int y) {
    if (id < 0 || id >= kMaxAmostras) {
        return;
    }
    g_amostras[id].x = x;
    g_amostras[id].y = y;
    g_amostras[id].pedida = true;
}

DWORD AmostraCor(int id) {
    return (id >= 0 && id < kMaxAmostras) ? g_amostras[id].cor : 0;
}

int AmostraPonto(int id, int* x, int* y) {
    if (id < 0 || id >= kMaxAmostras || !g_amostras[id].pedida) {
        return 0;
    }
    *x = g_amostras[id].x;
    *y = g_amostras[id].y;
    return 1;
}

void AmostraGuarda(int id, DWORD cor) {
    if (id >= 0 && id < kMaxAmostras) {
        g_amostras[id].cor = cor;
    }
}

int CamadaCursor(int* x, int* y) {
    return CursorNaTela(x, y) ? 1 : 0;
}

int CamadaTelaL() {
    return g_telaL;
}

int CamadaTelaA() {
    return g_telaA;
}

namespace {

// A camada de cima sob o cursor, com o canto dela. Nulo quando o cursor esta no
// mundo.
const Camada* SobCursor(int cx, int cy, int* cantoX, int* cantoY) {
    for (int i = g_nCamadas - 1; i >= 0; --i) {
        const Camada* c = g_camadas[i];
        if (c->visivel() == 0) {
            continue;
        }
        int x = 0;
        int y = 0;
        int l = 0;
        int a = 0;
        c->medida(g_telaL, g_telaA, &x, &y, &l, &a);
        if (cx < x || cy < y || cx >= x + l || cy >= y + a) {
            continue;
        }
        *cantoX = x;
        *cantoY = y;
        return c;
    }
    return nullptr;
}

} // namespace

// Devolve 1 quando o clique pertence a alguma camada - e entao o jogo nao o ve.
//
// O dono e decidido UMA VEZ, na borda do aperto, e vale ate o botao soltar. Sem
// isto o personagem caminhava ao clicar em algumas partes da loja: o botao
// Fechar (e o icone) fazem o painel desaparecer no mesmo quadro do clique, e do
// quadro seguinte em diante o botao - ainda apertado - caia no mundo, que anda
// para o ponto clicado enquanto o botao estiver em pe. Agora o aperto inteiro
// pertence a quem o comecou. A regra vale para os dois lados: um aperto que
// comecou no mundo continua do mundo mesmo que o cursor passe por cima do
// painel, para nao cortar um arrasto pela metade.
extern "C" int __cdecl CamadaMouse(int botao) {
    int cx = 0;
    int cy = 0;
    const bool apertado = (botao & 0x80) != 0;
    const bool temCursor = CursorNaTela(&cx, &cy);

    int cantoX = 0;
    int cantoY = 0;
    const Camada* sob = temCursor ? SobCursor(cx, cy, &cantoX, &cantoY) : nullptr;

    if (apertado && !g_botaoAntes) {
        g_apertoNosso = sob != nullptr;
        if (g_apertoNosso) {
            sob->clique(cx - cantoX, cy - cantoY);
        }
        // Pode ter sido o MENU abrindo ou fechando a barra: le o quadro ja no
        // proximo EndScene, em vez de esperar o intervalo normal.
        AmostraUrgente();
    } else if (!apertado) {
        g_apertoNosso = false;
    }
    g_botaoAntes = apertado;

    // Com o botao em pe manda o dono do aperto; em repouso, a camada sob o
    // cursor - e assim o botao direito e o do meio tambem nao atravessam.
    return (apertado ? g_apertoNosso : sob != nullptr) ? 1 : 0;
}

namespace {

constexpr DWORD kAposLer = 0x4B50EF;
const BYTE kAposLerBytes[7] = {0x89, 0x45, 0xFC, 0x83, 0x7D, 0xFC, 0x00};

__declspec(naked) void CorteHook() {
    __asm {
        mov dword ptr [ebp - 4], eax      // instrucao original 1
        pushad
        pushfd
        movzx eax, byte ptr [ebp - 0x0C]  // rgbButtons[0]
        push eax
        call CamadaMouse
        add esp, 4
        test eax, eax
        je segue
        mov byte ptr [ebp - 0x0C], 0      // o clique era nosso: o jogo nao ve
        mov byte ptr [ebp - 0x0B], 0
        mov byte ptr [ebp - 0x0A], 0
    segue:
        popfd
        popad
        cmp dword ptr [ebp - 4], 0        // instrucao original 2
        push 0x4B50F6                     // volta depois das duas
        ret
    }
}

void InstalaCorteDoClique() {
    if (memcmp(reinterpret_cast<void*>(kAposLer), kAposLerBytes, sizeof(kAposLerBytes)) != 0) {
        CamadaLog("=== corte do clique: bytes diferentes, nao instalado");
        return;
    }
    BYTE buf[sizeof(kAposLerBytes)];
    memset(buf, 0x90, sizeof(buf));
    buf[0] = 0xE9;
    const DWORD rel = reinterpret_cast<DWORD>(&CorteHook) - (kAposLer + 5);
    memcpy(buf + 1, &rel, sizeof(rel));
    DWORD antes = 0;
    if (!VirtualProtect(reinterpret_cast<void*>(kAposLer), sizeof(buf), PAGE_EXECUTE_READWRITE,
                        &antes)) {
        CamadaLog("=== corte do clique: VirtualProtect falhou");
        return;
    }
    memcpy(reinterpret_cast<void*>(kAposLer), buf, sizeof(buf));
    VirtualProtect(reinterpret_cast<void*>(kAposLer), sizeof(buf), antes, &antes);
    FlushInstructionCache(GetCurrentProcess(), reinterpret_cast<void*>(kAposLer), sizeof(buf));
    CamadaLog("=== corte do clique: instalado em 0x4B50EF");
}

struct Installer {
    Installer() { InstalaCorteDoClique(); }
};

Installer g_instalador;

} // namespace
