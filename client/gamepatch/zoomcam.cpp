// Zoom da câmera: solta o limite de afastamento.
//
// Caminho percorrido até aqui: o cliente lê o mouse por DirectInput (por isso a
// roda não aparece como mensagem do Windows). A leitura está em 0x4B50AB, que
// guarda o eixo Z da roda em mouse+0x14. Quem consome é 0x4B5289:
//
//   cam   = *(*(0x277C024) + 0x218D4)
//   zoom  = cam+0x34        (espelhado em cam+0x38)
//   min   = 1.2 local (2.5 montado, mais um ajuste por personagem)
//   max   = cam+0xC0        <- o limite que trava o afastamento
//   passo = roda / 240      (constante em 0x5F567C)
//
// Rodando a roda para afastar, o cliente faz zoom += passo e trava em cam+0xC0.
// Então basta aumentar esse campo. O desvio abaixo entra no começo de 0x4B5289,
// escreve o limite novo e devolve o controle — assim o valor vale exatamente no
// ponto em que é usado, sem depender de quem mais mexe na câmera.
//
// zoom.txt, na pasta do cliente:
//   fator=2.0   -> multiplica o limite original (o do jogo)
//   max=0       -> se > 0, usa este valor absoluto e ignora o fator

#include <windows.h>

#include <cstdio>
#include <cstring>

namespace {

constexpr DWORD kWheelFunc = 0x4B5289;
constexpr DWORD kWheelFuncBack = 0x4B528F;
const BYTE kWheelBytes[6] = {0x55, 0x8B, 0xEC, 0x83, 0xEC, 0x48};

constexpr DWORD kPlayerBlock = 0x277C024;
constexpr DWORD kCameraOff = 0x218D4;
constexpr DWORD kZoomOff = 0x34;
constexpr DWORD kZoomMaxOff = 0xC0;

float g_fator = 2.0f;
float g_max = 0.0f;
float g_original = 0.0f;
bool g_logado = false;
char g_dir[MAX_PATH] = {0};

void Log(const char* fmt, ...) {
    static int linhas = 0;
    if (linhas > 50) {
        return;
    }
    char caminho[MAX_PATH];
    sprintf_s(caminho, "%szoom-cam.log", g_dir);
    FILE* f = nullptr;
    if (fopen_s(&f, caminho, "a") != 0 || f == nullptr) {
        return;
    }
    va_list ap;
    va_start(ap, fmt);
    vfprintf(f, fmt, ap);
    va_end(ap);
    fputc('\n', f);
    fclose(f);
    ++linhas;
}

void LeConfig() {
    char caminho[MAX_PATH];
    sprintf_s(caminho, "%szoom.txt", g_dir);
    FILE* f = nullptr;
    if (fopen_s(&f, caminho, "r") != 0 || f == nullptr) {
        if (fopen_s(&f, caminho, "w") == 0 && f != nullptr) {
            fprintf(f, "# Zoom da camera. Reabra o jogo depois de mudar.\n");
            fprintf(f, "# fator = multiplica o limite de afastamento do jogo (1.0 = como era)\n");
            fprintf(f, "# max   = se maior que 0, usa este valor absoluto e ignora o fator\n");
            fprintf(f, "fator=2.0\nmax=0\n");
            fclose(f);
        }
        return;
    }
    char linha[128];
    while (fgets(linha, sizeof(linha), f) != nullptr) {
        if (linha[0] == '#' || linha[0] == ';') {
            continue;
        }
        char chave[32] = {0};
        double valor = 0.0;
        if (sscanf_s(linha, "%31[^=]=%lf", chave, static_cast<unsigned>(sizeof(chave)), &valor) != 2) {
            continue;
        }
        if (_stricmp(chave, "fator") == 0 && valor >= 0.1 && valor <= 20.0) {
            g_fator = static_cast<float>(valor);
        } else if (_stricmp(chave, "max") == 0 && valor >= 0.0 && valor <= 200.0) {
            g_max = static_cast<float>(valor);
        }
    }
    fclose(f);
}

void __cdecl SoltaLimite() {
    BYTE* bloco = *reinterpret_cast<BYTE**>(kPlayerBlock);
    if (bloco == nullptr) {
        return;
    }
    BYTE* cam = *reinterpret_cast<BYTE**>(bloco + kCameraOff);
    if (cam == nullptr) {
        return;
    }
    float* maxp = reinterpret_cast<float*>(cam + kZoomMaxOff);
    float alvo = (g_max > 0.0f) ? g_max : g_original * g_fator;

    if (!g_logado) {
        g_original = *maxp;
        alvo = (g_max > 0.0f) ? g_max : g_original * g_fator;
        Log("camera=%p limite original=%g novo=%g (fator=%g max=%g) zoom atual=%g",
            cam, g_original, alvo, g_fator, g_max, *reinterpret_cast<const float*>(cam + kZoomOff));
        g_logado = true;
    }
    if (alvo > 0.0f && *maxp != alvo) {
        *maxp = alvo;
    }
}

__declspec(naked) void WheelHook() {
    __asm {
        pushad
        pushfd
        call SoltaLimite
        popfd
        popad
        push ebp
        mov ebp, esp
        sub esp, 0x48
        push 0x4B528F // kWheelFuncBack
        ret
    }
}

bool InstalaJmp(DWORD at, const BYTE* esperado, size_t len, void* fn) {
    if (memcmp(reinterpret_cast<void*>(at), esperado, len) != 0) {
        return false;
    }
    DWORD old = 0;
    if (!VirtualProtect(reinterpret_cast<void*>(at), len, PAGE_EXECUTE_READWRITE, &old)) {
        return false;
    }
    BYTE buf[16];
    memset(buf, 0x90, len);
    buf[0] = 0xE9;
    DWORD rel = reinterpret_cast<DWORD>(fn) - (at + 5);
    memcpy(buf + 1, &rel, sizeof(rel));
    memcpy(reinterpret_cast<void*>(at), buf, len);
    VirtualProtect(reinterpret_cast<void*>(at), len, old, &old);
    FlushInstructionCache(GetCurrentProcess(), reinterpret_cast<void*>(at), len);
    return true;
}

struct Installer {
    Installer() {
        GetModuleFileNameA(nullptr, g_dir, MAX_PATH);
        char* barra = strrchr(g_dir, '\\');
        if (barra != nullptr) {
            *(barra + 1) = '\0';
        }
        LeConfig();
        bool ok = InstalaJmp(kWheelFunc, kWheelBytes, sizeof(kWheelBytes), WheelHook);
        Log("=== zoomcam: desvio 4B5289=%d fator=%g max=%g", ok ? 1 : 0, g_fator, g_max);
    }
};

Installer g_zoomCam;

} // namespace
