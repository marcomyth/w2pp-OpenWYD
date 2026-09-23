// Implementacao do dono das teclas. Ver teclas.h para os tres caminhos.

#include "teclas.h"

#include <windows.h>

#include <cstdio>
#include <cstring>

void CamadaLog(const char* texto);

namespace {

constexpr int kMaxDonos = 8;

struct Dono {
    int vk;
    int ordem;
    int (*trata)();
};

int (*g_texto)(int caractere) = nullptr;
Dono g_donos[kMaxDonos];
int g_nDonos = 0;
bool g_engolindo[256] = {false};
bool g_instalado = false;

void Instala();

} // namespace

void TeclaRegistra(int vk, int ordem, int (*trata)()) {
    if (trata == nullptr || vk < 0 || vk > 255 || g_nDonos >= kMaxDonos) {
        return;
    }
    int i = g_nDonos;
    while (i > 0 && g_donos[i - 1].ordem < ordem) {
        g_donos[i] = g_donos[i - 1];
        --i;
    }
    g_donos[i].vk = vk;
    g_donos[i].ordem = ordem;
    g_donos[i].trata = trata;
    ++g_nDonos;
    Instala();
}

void TeclaTexto(int (*trata)(int caractere)) {
    g_texto = trata;
    if (trata != nullptr) {
        Instala();
    }
}

namespace {

// A descida: pergunta aos donos, de cima para baixo.
extern "C" int __cdecl TeclaDesce(int vk) {
    if (vk < 0 || vk > 255) {
        return 0;
    }
    for (int i = 0; i < g_nDonos; ++i) {
        if (g_donos[i].vk == vk && g_donos[i].trata() != 0) {
            g_engolindo[vk] = true;
            return 1;
        }
    }
    // Com um campo de texto aberto o teclado inteiro e nosso, menos o Esc, que
    // ja passou pelos donos acima.
    if (g_texto != nullptr && vk != VK_ESCAPE) {
        g_engolindo[vk] = true;
        return 1;
    }
    return 0;
}

// O caractere que o TranslateMessage gerou da descida que engolimos.
extern "C" int __cdecl TeclaCaractere(int c) {
    // O campo de texto vem antes da marca: e ele quem recebe o caractere.
    if (g_texto != nullptr && g_texto(c) != 0) {
        return 1;
    }
    return (c >= 0 && c <= 255 && g_engolindo[c]) ? 1 : 0;
}

// A subida fecha o ciclo e apaga a marca.
extern "C" int __cdecl TeclaSobe(int vk) {
    if (vk < 0 || vk > 255 || !g_engolindo[vk]) {
        return 0;
    }
    g_engolindo[vk] = false;
    return 1;
}

constexpr DWORD kRamoDesce = 0x0054C62F;
constexpr DWORD kRamoDesceVolta = 0x0054C635;
constexpr DWORD kRamoSobe = 0x0054C88F;
constexpr DWORD kRamoSobeVolta = 0x0054C895;
constexpr DWORD kRamoChar = 0x0054C966;
constexpr DWORD kRamoCharVolta = 0x0054C96C;
constexpr DWORD kSaiDaWndProc = 0x0054D3AB;   // o pop edi/leave/ret 0x10 dela

const BYTE kRamoEdx[6] = {0x8B, 0x95, 0x70, 0xFE, 0xFF, 0xFF};   // mov edx,[ebp-0x190]
const BYTE kRamoEcx[6] = {0x8B, 0x8D, 0x70, 0xFE, 0xFF, 0xFF};   // mov ecx,[ebp-0x190]

int g_engoliu = 0;

__declspec(naked) void DesceHook() {
    __asm {
        pushad
        pushfd
        mov eax, [ebp + 0x10]             // wParam da WndProc: o codigo da tecla
        push eax
        call TeclaDesce
        add esp, 4
        mov g_engoliu, eax
        popfd
        popad
        cmp g_engoliu, 0
        je segue
        xor eax, eax                      // era nossa: a WndProc devolve 0
        push kSaiDaWndProc
        ret
    segue:
        mov edx, dword ptr [ebp - 0x190]  // instrucao original
        push kRamoDesceVolta
        ret
    }
}

__declspec(naked) void CharHook() {
    __asm {
        pushad
        pushfd
        movzx eax, byte ptr [ebp + 0x10]
        push eax
        call TeclaCaractere
        add esp, 4
        mov g_engoliu, eax
        popfd
        popad
        cmp g_engoliu, 0
        je segue
        xor eax, eax
        push kSaiDaWndProc
        ret
    segue:
        mov ecx, dword ptr [ebp - 0x190]
        push kRamoCharVolta
        ret
    }
}

__declspec(naked) void SobeHook() {
    __asm {
        pushad
        pushfd
        mov eax, [ebp + 0x10]
        push eax
        call TeclaSobe
        add esp, 4
        mov g_engoliu, eax
        popfd
        popad
        cmp g_engoliu, 0
        je segue
        xor eax, eax
        push kSaiDaWndProc
        ret
    segue:
        mov edx, dword ptr [ebp - 0x190]
        push kRamoSobeVolta
        ret
    }
}

void Desvia(DWORD onde, const BYTE* esperado, void* destino, const char* nome) {
    if (memcmp(reinterpret_cast<void*>(onde), esperado, 6) != 0) {
        char buf[120];
        sprintf_s(buf, "=== teclas: %s com bytes diferentes, nao instalado", nome);
        CamadaLog(buf);
        return;
    }
    BYTE salto[6];
    memset(salto, 0x90, sizeof(salto));
    salto[0] = 0xE9;
    const DWORD rel = reinterpret_cast<DWORD>(destino) - (onde + 5);
    memcpy(salto + 1, &rel, sizeof(rel));
    DWORD antes = 0;
    if (!VirtualProtect(reinterpret_cast<void*>(onde), sizeof(salto), PAGE_EXECUTE_READWRITE,
                        &antes)) {
        return;
    }
    memcpy(reinterpret_cast<void*>(onde), salto, sizeof(salto));
    VirtualProtect(reinterpret_cast<void*>(onde), sizeof(salto), antes, &antes);
    FlushInstructionCache(GetCurrentProcess(), reinterpret_cast<void*>(onde), sizeof(salto));
    char buf[120];
    sprintf_s(buf, "=== teclas: %s desviado em 0x%06X", nome, onde);
    CamadaLog(buf);
}

void Instala() {
    if (g_instalado) {
        return;
    }
    g_instalado = true;
    Desvia(kRamoDesce, kRamoEdx, reinterpret_cast<void*>(&DesceHook), "descida");
    Desvia(kRamoChar, kRamoEcx, reinterpret_cast<void*>(&CharHook), "caractere");
    Desvia(kRamoSobe, kRamoEdx, reinterpret_cast<void*>(&SobeHook), "subida");
}

} // namespace
