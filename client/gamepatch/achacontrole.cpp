// Localizador de controles da interface do cliente. FORA do build: entra só
// quando é preciso descobrir onde mora um botão.
//
// A janela raiz (0x6F0AB0) é uma estrutura grande com ponteiros de controle em
// deslocamentos fixos — o tooltip de item usa +0x27A08 e +0x27A10, por exemplo.
// Este módulo varre esses deslocamentos procurando objetos que pareçam controle:
// ponteiro de vtable dentro do .text e retângulo de tela plausível em +0x68.
//
// O retângulo vem do nó de desenho que o painel guarda em +0x64 (+0x04 dentro do
// nó, o que dá +0x68 no controle): x, y, largura e altura em float, já na tela.
//
// Comandos chegam pelo cmd.txt ao lado do executável, como no achabotao:
//
//   controles           - lista todo controle com retângulo na tela
//   controles <x> <y>   - só os que contêm esse ponto (o botão de Loja Pessoal
//                         em 1024x768 fica em 664..693 por 670..704)

#include <windows.h>

#include <cstdarg>
#include <cstdio>
#include <cstring>

void CamadaLog(const char* texto);
extern "C" void __cdecl LojaMostraIcone(int roubar);

namespace {

constexpr DWORD kUIRoot = 0x6F0AB0;
constexpr DWORD kVarredura = 0x30000;   // até onde vasculhar a estrutura da janela
constexpr DWORD kTextoIni = 0x401000;   // .text do WYD.exe
constexpr DWORD kTextoFim = 0x5F4000;
constexpr DWORD kNoNoControle = 0x68;   // nó em +0x64, retângulo em +0x04 dele

void Logf(const char* formato, ...) {
    char buf[220];
    va_list args;
    va_start(args, formato);
    vsprintf_s(buf, formato, args);
    va_end(args);
    CamadaLog(buf);
}

bool LeDword(DWORD endereco, DWORD* saida) {
    __try {
        *saida = *reinterpret_cast<DWORD*>(endereco);
        return true;
    } __except (EXCEPTION_EXECUTE_HANDLER) {
        return false;
    }
}

bool LeFloat(DWORD endereco, float* saida) {
    DWORD cru = 0;
    if (!LeDword(endereco, &cru)) {
        return false;
    }
    memcpy(saida, &cru, sizeof(cru));
    return true;
}

bool Plausivel(float v, float limite) {
    return v >= -limite && v <= limite;
}

void Varre(int px, int py) {
    DWORD raiz = 0;
    if (!LeDword(kUIRoot, &raiz) || raiz < 0x10000) {
        CamadaLog("=== controles: janela raiz ainda nao existe");
        return;
    }
    int achados = 0;
    for (DWORD off = 0; off < kVarredura && achados < 60; off += 4) {
        DWORD obj = 0;
        if (!LeDword(raiz + off, &obj) || obj < 0x10000) {
            continue;
        }
        DWORD vtable = 0;
        if (!LeDword(obj, &vtable) || vtable < kTextoIni || vtable >= kTextoFim) {
            continue;
        }
        float x = 0.0f;
        float y = 0.0f;
        float l = 0.0f;
        float a = 0.0f;
        if (!LeFloat(obj + kNoNoControle + 0, &x) || !LeFloat(obj + kNoNoControle + 4, &y) ||
            !LeFloat(obj + kNoNoControle + 8, &l) || !LeFloat(obj + kNoNoControle + 12, &a)) {
            continue;
        }
        if (!Plausivel(x, 4000.0f) || !Plausivel(y, 4000.0f) || l <= 0.0f || a <= 0.0f ||
            l > 2000.0f || a > 2000.0f) {
            continue;
        }
        if (px >= 0) {
            const float fx = static_cast<float>(px);
            const float fy = static_cast<float>(py);
            if (fx < x || fy < y || fx > x + l || fy > y + a) {
                continue;
            }
        }
        ++achados;
        Logf("    +%05X  obj=%08X vtable=%08X  ret=(%.0f,%.0f %.0fx%.0f)", off, obj, vtable, x, y,
             l, a);
    }
    Logf("=== controles: %d achados%s", achados, px >= 0 ? " sob o ponto" : "");
}

// --- perguntando ao cliente quem esta sob o ponto --------------------------
//
// Em jogo o clique nao passa pelo tratador de mensagens do Windows: o cliente le
// o mouse por DirectInput (0x4B50AB), e com o botao apertado chama 0x4B5BD3, que
// entrega a mensagem 0x201 a RAIZ da interface (0x6F0AB0), pelo metodo virtual
// +0x08, com x e y. O mesmo objeto tem em +0xB8 o teste de acerto, que devolve o
// controle sob o ponto - e isso nos podemos chamar tambem.

extern "C" void __cdecl AnotaClique(DWORD controle);

typedef void*(__thiscall* AcertoFn)(void* self, int x, int y);

void Acerto(int x, int y) {
    DWORD raiz = 0;
    if (!LeDword(kUIRoot, &raiz) || raiz < 0x10000) {
        CamadaLog("=== acerto: raiz da interface ainda nao existe");
        return;
    }
    DWORD vtable = 0;
    LeDword(raiz, &vtable);
    DWORD msgHandler = 0;
    DWORD hitTest = 0;
    LeDword(vtable + 0x08, &msgHandler);
    LeDword(vtable + 0xB8, &hitTest);
    Logf("=== acerto: raiz=%08X vtable=%08X  msg(+08)=%08X  acerto(+B8)=%08X", raiz, vtable,
         msgHandler, hitTest);
    if (hitTest < kTextoIni || hitTest >= kTextoFim) {
        return;
    }
    void* controle = reinterpret_cast<AcertoFn>(hitTest)(reinterpret_cast<void*>(raiz), x, y);
    const DWORD c = reinterpret_cast<DWORD>(controle);
    Logf("=== acerto: (%d,%d) -> controle %08X", x, y, c);
    if (c > 0x10000) {
        AnotaClique(c);
    }
}

// --- quem foi clicado ------------------------------------------------------
//
// O despachante da interface e o tratador de mensagens em 0x41FD0D: 0x201 para
// botao pressionado, 0x202 para solto, 0x200 para movimento e 0x204 para o
// direito. O ramo do clique chama 0x410780 (metodo do gerenciador, com x e y), e
// la dentro, em 0x4107CD, o proprio cliente chama a vtable +0xB8 para descobrir
// QUAL controle esta sob o ponto. Em 0x4107D3 esse controle ja esta em EAX.
//
// Anotamos aqui os campos do controle clicado. Comparando um clique no botao de
// Loja Pessoal com cliques nos vizinhos sai o campo que identifica cada um.

constexpr DWORD kDepoisDoAcerto = 0x004107D3;
const BYTE kBytesEsperados[7] = {0x89, 0x45, 0xF0, 0x83, 0x7D, 0xF0, 0x00};
int g_cliques = 0;

extern "C" void __cdecl AnotaClique(DWORD controle) {
    if (controle < 0x10000 || g_cliques >= 24) {
        return;
    }
    ++g_cliques;
    DWORD vtable = 0;
    LeDword(controle, &vtable);
    Logf("=== clique: controle=%08X vtable=%08X", controle, vtable);
    char linha[200];
    int k = 0;
    k += sprintf_s(linha + k, sizeof(linha) - k, "    campos:");
    for (DWORD off = 0x04; off <= 0x30; off += 4) {
        DWORD v = 0;
        LeDword(controle + off, &v);
        k += sprintf_s(linha + k, sizeof(linha) - k, " +%02X=%08X", off, v);
    }
    CamadaLog(linha);
    DWORD dados = 0;
    LeDword(controle + 0x670, &dados);
    DWORD primeiro = 0;
    if (dados > 0x10000) {
        LeDword(dados, &primeiro);
    }
    float x = 0.0f;
    float y = 0.0f;
    float l = 0.0f;
    float a = 0.0f;
    LeFloat(controle + 0x68, &x);
    LeFloat(controle + 0x6C, &y);
    LeFloat(controle + 0x70, &l);
    LeFloat(controle + 0x74, &a);
    Logf("    +670=%08X (primeiro word %04X)  no=(%.0f,%.0f %.0fx%.0f)", dados, primeiro & 0xFFFF,
         x, y, l, a);
}

// Talvez 0x410780 nem seja chamada em jogo (o cliente le o mouse por
// DirectInput), ou o modo do gerenciador ([this+0x400] != 1) pule o teste de
// acerto logo na entrada. Este segundo desvio registra a entrada da funcao.
constexpr DWORD kEntradaClique = 0x00410780;
const BYTE kBytesEntrada[5] = {0x55, 0x8B, 0xEC, 0x6A, 0xFF};
int g_entradas = 0;

extern "C" void __cdecl AnotaEntrada(DWORD gerente, int x, int y) {
    if (g_entradas >= 10) {
        return;
    }
    ++g_entradas;
    DWORD modo = 0;
    LeDword(gerente + 0x400, &modo);
    Logf("=== clique: entrada em 0x410780, (%d,%d), modo=%lu", x, y, modo);
}

__declspec(naked) void EntradaHook() {
    __asm {
        mov eax, [esp + 4]
        mov edx, [esp + 8]
        pushad
        pushfd
        push edx
        push eax
        push ecx
        call AnotaEntrada
        add esp, 12
        popfd
        popad
        push ebp                  // instrucoes originais
        mov ebp, esp
        push -1
        push 0x00410785
        ret
    }
}

__declspec(naked) void CliqueHook() {
    __asm {
        pushad
        pushfd
        push eax
        call AnotaClique
        add esp, 4
        popfd
        popad
        mov dword ptr [ebp - 0x10], eax   // instrucao original 1
        cmp dword ptr [ebp - 0x10], 0     // instrucao original 2
        push 0x004107DA
        ret
    }
}

void DesviaClique() {
    if (memcmp(reinterpret_cast<void*>(kDepoisDoAcerto), kBytesEsperados,
               sizeof(kBytesEsperados)) != 0) {
        CamadaLog("=== clique: bytes diferentes, nao instalado");
        return;
    }
    BYTE buf[sizeof(kBytesEsperados)];
    memset(buf, 0x90, sizeof(buf));
    buf[0] = 0xE9;
    const DWORD rel = reinterpret_cast<DWORD>(&CliqueHook) - (kDepoisDoAcerto + 5);
    memcpy(buf + 1, &rel, sizeof(rel));
    DWORD antes = 0;
    if (!VirtualProtect(reinterpret_cast<void*>(kDepoisDoAcerto), sizeof(buf),
                        PAGE_EXECUTE_READWRITE, &antes)) {
        CamadaLog("=== clique: VirtualProtect falhou");
        return;
    }
    memcpy(reinterpret_cast<void*>(kDepoisDoAcerto), buf, sizeof(buf));
    VirtualProtect(reinterpret_cast<void*>(kDepoisDoAcerto), sizeof(buf), antes, &antes);
    FlushInstructionCache(GetCurrentProcess(), reinterpret_cast<void*>(kDepoisDoAcerto),
                          sizeof(buf));
    g_cliques = 0;
    CamadaLog("=== clique: desvio instalado em 0x4107D3");

    if (memcmp(reinterpret_cast<void*>(kEntradaClique), kBytesEntrada, sizeof(kBytesEntrada)) != 0) {
        CamadaLog("=== clique: entrada com bytes diferentes");
        return;
    }
    BYTE salto[sizeof(kBytesEntrada)];
    salto[0] = 0xE9;
    const DWORD rel2 = reinterpret_cast<DWORD>(&EntradaHook) - (kEntradaClique + 5);
    memcpy(salto + 1, &rel2, sizeof(rel2));
    DWORD antes2 = 0;
    if (!VirtualProtect(reinterpret_cast<void*>(kEntradaClique), sizeof(salto),
                        PAGE_EXECUTE_READWRITE, &antes2)) {
        return;
    }
    memcpy(reinterpret_cast<void*>(kEntradaClique), salto, sizeof(salto));
    VirtualProtect(reinterpret_cast<void*>(kEntradaClique), sizeof(salto), antes2, &antes2);
    FlushInstructionCache(GetCurrentProcess(), reinterpret_cast<void*>(kEntradaClique),
                          sizeof(salto));
    g_entradas = 0;
    CamadaLog("=== clique: desvio instalado na entrada 0x410780");
}

DWORD WINAPI Thread(LPVOID) {
    char caminho[MAX_PATH];
    GetModuleFileNameA(nullptr, caminho, MAX_PATH);
    char* barra = strrchr(caminho, 92);
    if (barra == nullptr) {
        return 0;
    }
    strcpy_s(barra + 1, MAX_PATH - (barra + 1 - caminho), "cmd.txt");
    FILETIME ultima;
    memset(&ultima, 0, sizeof(ultima));
    for (;;) {
        Sleep(300);
        WIN32_FILE_ATTRIBUTE_DATA info;
        if (!GetFileAttributesExA(caminho, GetFileExInfoStandard, &info) ||
            CompareFileTime(&info.ftLastWriteTime, &ultima) == 0) {
            continue;
        }
        ultima = info.ftLastWriteTime;
        FILE* f = nullptr;
        if (fopen_s(&f, caminho, "r") != 0 || f == nullptr) {
            continue;
        }
        char linha[128];
        if (fgets(linha, sizeof(linha), f) != nullptr) {
            if (strncmp(linha, "controles", 9) == 0) {
                int x = -1;
                int y = -1;
                sscanf_s(linha + 9, "%d %d", &x, &y);
                Varre(x, y);
            } else if (strncmp(linha, "cliques", 7) == 0) {
                DesviaClique();
            } else if (strncmp(linha, "acerto", 6) == 0) {
                int x = 0;
                int y = 0;
                if (sscanf_s(linha + 6, "%d %d", &x, &y) == 2) {
                    Acerto(x, y);
                }
            } else if (strncmp(linha, "roubo", 5) == 0) {
                const int roubar = atoi(linha + 5);
                LojaMostraIcone(roubar);
                Logf("=== clique: roubo do botao %s", roubar ? "ligado" : "desligado");
            }
        }
        fclose(f);
    }
}

struct Installer {
    Installer() { CreateThread(nullptr, 0, Thread, nullptr, 0, nullptr); }
};

Installer g_instalador;

} // namespace
