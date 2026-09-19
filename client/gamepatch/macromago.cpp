// Macro do WYD 7662: perseguição do MG e o "Return" do MF e do MG.
//
// Vocabulário: MF = macro de ataque físico (tipo 1 em 0x63A29C), MG = macro de
// ataque mágico (tipo 2). Modo de caça em [cena+0x8A3A0]: 0 = Run (perseguir),
// 1 = Return (perseguir e voltar), 2 = parado.
//
// O que o cliente faz sozinho:
//   - A perseguição (0x4970F9) procura monstro a até 8 casas com caminho livre e
//     chama 0x5162B3(mob, 1), que anda até o corpo a corpo (ou ataca, se já
//     estiver no alcance da arma). Ela rejeita distância acima de 15.
//   - O ClientPatch manda o MG cair nessa perseguição (0x4974C7/0x4974D7), mas só
//     no modo Run.
//   - No Return sem alvo, o macro caminha de volta ao ponto salvo
//     (0x496C4A-0x496EA0, pacote 0x36C), e só se estiver a no máximo 10 casas dele.
//
// Os três desvios daqui:
//
//   A) 0x4974C7 e 0x4974D7 (reaponta o desvio que o ClientPatch instalou, por isso
//      a instalação espera o primeiro tick do macro): o MG persegue no Run e no
//      Return; parado continua parado.
//
//   B) 0x4970F9: a perseguição passa a ser a daqui, para MF e MG. Diferenças: vai
//      até kRaioCaca casas (o cliente ia a 8; 15 é o teto de 0x5162B3) e, no
//      Return, só persegue monstro a até kColeira casas DO PONTO SALVO. Sem essa
//      coleira o personagem ia embora atrás de qualquer monstro perto DELE e nunca
//      tinha motivo para voltar — foi o que aconteceu na tentativa anterior, que
//      mexia só no retorno.
//
//   C) 0x496CC3: o limite de 10 casas para caminhar de volta vira kVoltaDeAte.
//      Depois de perseguir, o personagem fica mais longe que 10 e nunca voltava.
//
//   D) 0x496C4A: o retorno ao ponto roda ANTES do ataque na mesma função. No tick
//      em que o personagem chega perto do monstro ele ainda está sem alvo e longe
//      do ponto, então o cliente mandava caminhar de volta e a magia nunca saía —
//      "vai até o mob e volta sem atacar". Aqui o retorno é pulado enquanto o
//      personagem estiver andando/agindo ou enquanto houver monstro ao alcance do
//      golpe (alcance da skill no MG, da arma no MF).
//
// Com a coleira no (B), o retorno do cliente continua como é: sem alvo e parado,
// ele caminha de volta. Quando não há mais o que caçar em volta do ponto, ele
// volta sozinho.

#include <windows.h>

#include <cstdio>
#include <cstring>

void AlvosTick(); // alvos.cpp

namespace {

// ---- ajustes (macro.txt, ao lado do WYD.exe) -----------------------------
// raio     = até onde perseguir, a partir do personagem (teto de 0x5162B3: 15)
// coleira  = no Return, distância máxima do MONSTRO ao ponto salvo
// retomar  = no Return, só volta a caçar quando estiver a até tantas casas do
//            ponto; mais longe que isso ele só caminha de volta (evita o vai e
//            volta entre o ponto e o monstro)
// volta    = de quão longe do ponto ainda caminha de volta (cliente: 10)
// janela   = ms segurando o retorno para o golpe sair
int g_raio = 10;
int g_coleira = 10;
int g_retomar = 10;
BYTE g_voltaDeAte = 40;
DWORD g_janelaGolpe = 1500;
// --------------------------------------------------------------------------

constexpr DWORD kMacroTick = 0x496306;      // thiscall, ecx = cena
constexpr DWORD kMacroTickBack = 0x49630F;
const BYTE kTickBytes[9] = {0x55, 0x8B, 0xEC, 0x81, 0xEC, 0x24, 0x01, 0x00, 0x00};

constexpr DWORD kChaseLoop = 0x4970F9;      // perseguição do cliente
const BYTE kChaseBytes[10] = {0xC7, 0x85, 0x34, 0xFF, 0xFF, 0xFF, 0x00, 0x00, 0x00, 0x00};

constexpr DWORD kReturnRangeImm = 0x496CC3 + 3;  // o 0x0A do "no máximo 10 casas do ponto"
const BYTE kReturnRangeBytes[4] = {0x83, 0x7D, 0x88, 0x0A};

constexpr DWORD kReturnBlock = 0x496C4A;    // início do "caminha de volta ao ponto"
constexpr DWORD kReturnBlockBack = 0x496C50;
constexpr DWORD kAfterReturnBlock = 0x496EA5;
const BYTE kReturnBytes[6] = {0x8B, 0x85, 0xE4, 0xFE, 0xFF, 0xFF};

constexpr DWORD kMobAbility = 0x539C2C;     // int __cdecl BASE_GetMobAbility(mob, efeito)
constexpr DWORD kSkillTable = 0x11DA838;    // STRUCT_SPELL[]; +0x10 = Range
constexpr DWORD kPlayerBlock = 0x277C024;
constexpr int kEfRange = 0x1B;

constexpr DWORD kMageHookA = 0x4974C7;      // fim da busca de alvo do MG
constexpr DWORD kMageHookB = 0x4974D7;
constexpr DWORD kMacroEnd = 0x4975C0;       // epílogo da função do macro
constexpr DWORD kMacroSkip = 0x4972E5;      // "não faz nada neste tick"

constexpr DWORD kDistance = 0x541324;       // int __cdecl BASE_GetDistance(x, y, x, y)
constexpr DWORD kRoute = 0x54139A;          // caminho livre? devolve o ponto alcançado
constexpr DWORD kAttack = 0x5162B3;         // thiscall(self, mob, perseguir): 1 atacou, 2 andando
constexpr DWORD kHeightMap = 0x6F0AB0;
constexpr int kRouteSlope = 8;              // tolerância de altura, como o cliente usa

constexpr DWORD kSelf = 0x4C;
constexpr DWORD kMobList = 0x34;
constexpr DWORD kHuntMode = 0x8A3A0;
constexpr DWORD kTarget = 0x8A374;
constexpr DWORD kSpotX = 0x8A378;           // ponto salvo ao ENTRAR no Return (0x46CF3C)
constexpr DWORD kSpotY = 0x8A37C;

typedef int(__cdecl* DistanceFn)(int, int, int, int);
typedef void(__cdecl* RouteFn)(int, int, int*, int*, void*, int);
typedef int(__thiscall* AttackFn)(void*, void*, int);
typedef int(__cdecl* AbilityFn)(void*, int);
typedef int(__thiscall* BusyFn)(void*);

int g_pulaRetorno = 0;
bool g_installed = false;
char g_cfgPath[MAX_PATH] = {0};
char g_logPath[MAX_PATH] = {0};

void Log(const char* fmt, ...) {
    static int lines = 0;
    if (lines > 300 || g_logPath[0] == '\0') {
        return;
    }
    FILE* f = nullptr;
    if (fopen_s(&f, g_logPath, "a") != 0 || f == nullptr) {
        return;
    }
    va_list ap;
    va_start(ap, fmt);
    vfprintf(f, fmt, ap);
    va_end(ap);
    fputc('\n', f);
    fclose(f);
    ++lines;
}

int Coord(const void* mob, DWORD off) {
    float v = 0.0f;
    memcpy(&v, static_cast<const BYTE*>(mob) + off, sizeof(v));
    return static_cast<int>(v);
}

int Dist(int ax, int ay, int bx, int by) {
    return reinterpret_cast<DistanceFn>(kDistance)(ax, ay, bx, by);
}

bool CaminhoLivre(int myX, int myY, int mx, int my) {
    int tx = mx, ty = my;
    void* map = reinterpret_cast<BYTE*>(*reinterpret_cast<DWORD*>(kHeightMap)) + 0xD4;
    reinterpret_cast<RouteFn>(kRoute)(myX, myY, &tx, &ty, map, kRouteSlope);
    return tx == mx && ty == my;
}

int AlcanceDoGolpe(int type);

// Perseguição: do monstro mais perto ao mais longe. No Return, só conta monstro
// perto do ponto salvo, senão o personagem se afasta e nunca tem por que voltar.
void __cdecl PerseguirTick(BYTE* scene) {
    static DWORD ultimoLog = 0;
    if (scene == nullptr) {
        return;
    }
    BYTE* self = *reinterpret_cast<BYTE**>(scene + kSelf);
    void* list = *reinterpret_cast<void**>(scene + kMobList);
    if (self == nullptr || list == nullptr) {
        return;
    }
    int myX = Coord(self, 0x28), myY = Coord(self, 0x2C);
    int mode = *reinterpret_cast<const int*>(scene + kHuntMode);
    int type = *reinterpret_cast<const int*>(0x63A29C);
    int spotX = *reinterpret_cast<const int*>(scene + kSpotX);
    int spotY = *reinterpret_cast<const int*>(scene + kSpotY);
    bool comColeira = (mode == 1 && spotX != 0 && spotY != 0);

    int vistos = 0, foraDaColeira = 0, semCaminho = 0, escolhidoDist = -1, resultado = 0;
    int longeDoPonto = comColeira ? Dist(myX, myY, spotX, spotY) : 0;

    // Longe do ponto: não caça nada, só deixa o retorno caminhar de volta. Sem
    // isso o personagem fica indo e voltando entre o ponto e o monstro.
    bool voltandoPraCasa = comColeira && longeDoPonto > g_retomar;

    // No MG o corpo a corpo não interessa: 0x5162B3 GOLPEIA se o monstro já está
    // ao alcance da arma, e era por isso que o macro mágico às vezes dava
    // pancada física. Nesse caso o monstro já está no alcance da magia e quem
    // resolve é o próprio macro, no tick seguinte.
    int semMelee = (type == 2) ? AlcanceDoGolpe(1) : 0;

    for (int raio = semMelee + 1; raio <= g_raio && resultado == 0 && !voltandoPraCasa; ++raio) {
        BYTE* node = *reinterpret_cast<BYTE**>(static_cast<BYTE*>(list) + 0x10);
        for (int guard = 0; node != nullptr && *reinterpret_cast<const int*>(node + 0xC) != 0 && guard < 300; ++guard) {
            BYTE* mob = node;
            node = *reinterpret_cast<BYTE**>(node + 0xC);
            if (mob == self) {
                continue;
            }
            int mx = Coord(mob, 0x28), my = Coord(mob, 0x2C);
            if (Dist(myX, myY, mx, my) != raio) {
                continue;
            }
            if (raio == 1) {
                ++vistos;
            }
            if (comColeira && Dist(spotX, spotY, mx, my) > g_coleira) {
                ++foraDaColeira;
                continue;
            }
            if (!CaminhoLivre(myX, myY, mx, my)) {
                ++semCaminho;
                continue;
            }
            int r = reinterpret_cast<AttackFn>(kAttack)(self, mob, 1);
            if (r == 1) {
                *reinterpret_cast<BYTE**>(scene + kTarget) = mob; // atacou: vira alvo do macro
            }
            if (r != 0) {
                resultado = r;
                escolhidoDist = raio;
                break;
            }
        }
    }

    DWORD now = GetTickCount();
    if (now - ultimoLog >= 2000) {
        ultimoLog = now;
        Log("perseguir tipo=%d modo=%d pos=%d,%d ponto=%d,%d dist_ponto=%d voltando=%d escolhido=%d "
            "r=%d fora_coleira=%d sem_caminho=%d sem_melee=%d",
            type, mode, myX, myY, spotX, spotY, comColeira ? longeDoPonto : -1,
            voltandoPraCasa ? 1 : 0, escolhidoDist, resultado, foraDaColeira, semCaminho, semMelee);
    }
}

// Alcance do golpe: a skill selecionada no MG, a arma no MF.
int AlcanceDoGolpe(int type) {
    BYTE* p = reinterpret_cast<BYTE*>(*reinterpret_cast<DWORD*>(kPlayerBlock));
    if (p == nullptr) {
        return 2;
    }
    int arma = reinterpret_cast<AbilityFn>(kMobAbility)(p + 0x750, kEfRange);
    if (type == 2) {
        int slot = *reinterpret_cast<const char*>(p + 0xF45);
        int skill = *(p + 0xF46 + slot);
        if (skill > 0x68) {
            skill += 0x5F;
        }
        int range = *reinterpret_cast<const int*>(kSkillTable + skill * 0x60 + 0x10);
        if (range > arma) {
            return range;
        }
    }
    return arma > 0 ? arma : 2;
}

bool EstaOcupado(void* self) {
    void** vt = *reinterpret_cast<void***>(self);
    return reinterpret_cast<BusyFn>(vt[0x50 / 4])(self) == 1;
}

// (D) o retorno ao ponto só pode rodar quando não há o que fazer aqui. Devolve 1
// para pular o retorno.
//
// A exceção "tem monstro ao alcance" é LIMITADA NO TEMPO (kJanelaGolpe): ela
// existe só para o ataque conseguir sair no tick seguinte. Sem esse limite, um
// monstro parado ao lado que o ataque recusa (fora da mira, tipo errado, dono de
// outro jogador) segurava o personagem para sempre e ele nunca voltava.
int __cdecl DecideRetorno(BYTE* scene) {
    static DWORD janelaInicio = 0;
    static bool janelaAberta = false;
    static DWORD ultimoLog = 0;

    if (scene == nullptr) {
        return 0;
    }
    int type = *reinterpret_cast<const int*>(0x63A29C);
    if ((type != 1 && type != 2) || *reinterpret_cast<const int*>(scene + kHuntMode) != 1) {
        janelaAberta = false;
        return 0;
    }
    BYTE* self = *reinterpret_cast<BYTE**>(scene + kSelf);
    void* list = *reinterpret_cast<void**>(scene + kMobList);
    if (self == nullptr || list == nullptr) {
        return 0;
    }
    if (*reinterpret_cast<void**>(scene + kTarget) != nullptr) {
        janelaAberta = false; // com alvo o cliente já não volta
        return 0;
    }

    DWORD now = GetTickCount();
    bool ocupado = EstaOcupado(self);
    int myX = Coord(self, 0x28), myY = Coord(self, 0x2C);
    int alcance = AlcanceDoGolpe(type) + 1; // +1 da Força Espectral
    bool noAlcance = false;
    if (!ocupado) {
        BYTE* node = *reinterpret_cast<BYTE**>(static_cast<BYTE*>(list) + 0x10);
        for (int guard = 0; node != nullptr && *reinterpret_cast<const int*>(node + 0xC) != 0 && guard < 300; ++guard) {
            if (node != self) {
                int mx = Coord(node, 0x28), my = Coord(node, 0x2C);
                int d = Dist(myX, myY, mx, my);
                if (d > 0 && d <= alcance && CaminhoLivre(myX, myY, mx, my)) {
                    noAlcance = true;
                    break;
                }
            }
            node = *reinterpret_cast<BYTE**>(node + 0xC);
        }
    }

    int decisao;
    if (ocupado) {
        janelaAberta = false;
        decisao = 1; // andando ou agindo: não interrompe
    } else if (!noAlcance) {
        janelaAberta = false;
        decisao = 0; // nada para golpear daqui: pode voltar
    } else {
        if (!janelaAberta) {
            janelaAberta = true;
            janelaInicio = now;
        }
        decisao = (now - janelaInicio < g_janelaGolpe) ? 1 : 0;
    }

    if (now - ultimoLog >= 2000) {
        ultimoLog = now;
        int spotX = *reinterpret_cast<const int*>(scene + kSpotX);
        int spotY = *reinterpret_cast<const int*>(scene + kSpotY);
        Log("retorno pula=%d ocupado=%d no_alcance=%d alcance=%d pos=%d,%d ponto=%d,%d dist_ponto=%d",
            decisao, ocupado ? 1 : 0, noAlcance ? 1 : 0, alcance, myX, myY, spotX, spotY,
            (spotX && spotY) ? Dist(myX, myY, spotX, spotY) : -1);
    }
    return decisao;
}

__declspec(naked) void ReturnGate() {
    __asm {
        pushad
        pushfd
        push dword ptr [ebp - 0x11C]
        call DecideRetorno
        add esp, 4
        mov g_pulaRetorno, eax
        popfd
        popad
        cmp g_pulaRetorno, 0
        jne pula
        mov eax, dword ptr [ebp - 0x11C]
        push 0x496C50 // kReturnBlockBack
        ret
    pula:
        push 0x496EA5 // kAfterReturnBlock
        ret
    }
}

// (A) fim da busca de alvo do MG: cai na perseguição no Run e no Return.
__declspec(naked) void MageChaseGate() {
    __asm {
        mov ecx, dword ptr [ebp - 0x11C]
        cmp dword ptr [ecx + 0x8A3A0], 2
        je parado
        push 0x4970F9 // kChaseLoop, agora desviado para ChaseHook
        ret
    parado:
        push 0x4972E5 // kMacroSkip
        ret
    }
}

// (B) perseguição inteira, no lugar da do cliente.
__declspec(naked) void ChaseHook() {
    __asm {
        pushad
        pushfd
        push dword ptr [ebp - 0x11C]
        call PerseguirTick
        add esp, 4
        popfd
        popad
        push 0x4975C0 // kMacroEnd
        ret
    }
}

bool Escreve(DWORD at, const void* bytes, size_t len) {
    DWORD old = 0;
    if (!VirtualProtect(reinterpret_cast<void*>(at), len, PAGE_EXECUTE_READWRITE, &old)) {
        return false;
    }
    memcpy(reinterpret_cast<void*>(at), bytes, len);
    VirtualProtect(reinterpret_cast<void*>(at), len, old, &old);
    FlushInstructionCache(GetCurrentProcess(), reinterpret_cast<void*>(at), len);
    return true;
}

bool InstalaJmp(DWORD at, const BYTE* esperado, size_t len, void* fn) {
    if (memcmp(reinterpret_cast<void*>(at), esperado, len) != 0) {
        return false;
    }
    BYTE buf[16];
    memset(buf, 0x90, len);
    buf[0] = 0xE9;
    DWORD rel = reinterpret_cast<DWORD>(fn) - (at + 5);
    memcpy(buf + 1, &rel, sizeof(rel));
    return Escreve(at, buf, len);
}

// Reaponta o `je` de 6 bytes que o ClientPatch instalou.
bool ReapontaJe(DWORD at, void* fn) {
    const BYTE* p = reinterpret_cast<const BYTE*>(at);
    if (p[0] != 0x0F || p[1] != 0x84) {
        return false;
    }
    BYTE buf[6] = {0x0F, 0x84};
    DWORD rel = reinterpret_cast<DWORD>(fn) - (at + 6);
    memcpy(buf + 2, &rel, sizeof(rel));
    return Escreve(at, buf, sizeof(buf));
}

// macro.txt: uma linha "chave=valor" por ajuste. Some o arquivo, valem os
// padrões; o arquivo é criado na primeira execução.
void LeConfig() {
    FILE* f = nullptr;
    if (fopen_s(&f, g_cfgPath, "r") != 0 || f == nullptr) {
        if (fopen_s(&f, g_cfgPath, "w") == 0 && f != nullptr) {
            fprintf(f, "# Macro do WYD: perseguicao e retorno. Reabra o jogo depois de mudar.\n");
            fprintf(f, "# raio    = ate onde perseguir, a partir do personagem (teto do cliente: 15)\n");
            fprintf(f, "# coleira = no Return, distancia maxima do MONSTRO ao ponto salvo\n");
            fprintf(f, "# retomar = no Return, so volta a cacar estando a ate tantas casas do ponto\n");
            fprintf(f, "# volta   = de quao longe do ponto ainda caminha de volta (cliente: 10)\n");
            fprintf(f, "# janela  = ms segurando o retorno para o golpe sair\n");
            fprintf(f, "raio=%d\ncoleira=%d\nretomar=%d\nvolta=%d\njanela=%lu\n",
                    g_raio, g_coleira, g_retomar, static_cast<int>(g_voltaDeAte), g_janelaGolpe);
            fclose(f);
        }
        return;
    }
    char linha[128];
    while (fgets(linha, sizeof(linha), f) != nullptr) {
        if (linha[0] == '#' || linha[0] == ';') {
            continue;
        }
        char chave[64] = {0};
        int valor = 0;
        if (sscanf_s(linha, "%63[^=]=%d", chave, static_cast<unsigned>(sizeof(chave)), &valor) != 2) {
            continue;
        }
        if (_stricmp(chave, "raio") == 0 && valor >= 1 && valor <= 15) {
            g_raio = valor;
        } else if (_stricmp(chave, "coleira") == 0 && valor >= 1 && valor <= 60) {
            g_coleira = valor;
        } else if (_stricmp(chave, "retomar") == 0 && valor >= 1 && valor <= 60) {
            g_retomar = valor;
        } else if (_stricmp(chave, "volta") == 0 && valor >= 1 && valor <= 120) {
            g_voltaDeAte = static_cast<BYTE>(valor);
        } else if (_stricmp(chave, "janela") == 0 && valor >= 0 && valor <= 10000) {
            g_janelaGolpe = static_cast<DWORD>(valor);
        }
    }
    fclose(f);
}

void __cdecl InstalaNoPrimeiroTick();

// Chamado a cada quadro pelo desvio abaixo: instala o que falta na primeira vez
// e passa a vez ao sistema de alvos (alvos.cpp), que precisa da mesma batida.
void __cdecl TickGeral() {
    InstalaNoPrimeiroTick();
    AlvosTick();
}

void __cdecl InstalaNoPrimeiroTick() {
    if (g_installed) {
        return;
    }
    g_installed = true;
    bool a1 = ReapontaJe(kMageHookA, MageChaseGate);
    bool a2 = ReapontaJe(kMageHookB, MageChaseGate);
    bool b = InstalaJmp(kChaseLoop, kChaseBytes, sizeof(kChaseBytes), ChaseHook);
    bool c = memcmp(reinterpret_cast<void*>(kReturnRangeImm - 3), kReturnRangeBytes, sizeof(kReturnRangeBytes)) == 0 &&
             Escreve(kReturnRangeImm, &g_voltaDeAte, sizeof(g_voltaDeAte));
    bool d = InstalaJmp(kReturnBlock, kReturnBytes, sizeof(kReturnBytes), ReturnGate);
    Log("=== macromago: mg4974C7=%d mg4974D7=%d perseguir4970F9=%d volta496CC3=%d retorno496C4A=%d "
        "(raio=%d coleira=%d retomar=%d volta=%d janela=%lu)",
        a1 ? 1 : 0, a2 ? 1 : 0, b ? 1 : 0, c ? 1 : 0, d ? 1 : 0,
        g_raio, g_coleira, g_retomar, g_voltaDeAte, g_janelaGolpe);
}

__declspec(naked) void TickHook() {
    __asm {
        push ebp
        mov ebp, esp
        sub esp, 0x124
        pushad
        pushfd
        call TickGeral
        popfd
        popad
        push 0x49630F // kMacroTickBack
        ret
    }
}

struct Installer {
    Installer() {
        GetModuleFileNameA(nullptr, g_logPath, MAX_PATH);
        strcpy_s(g_cfgPath, MAX_PATH, g_logPath);
        char* barra = strrchr(g_logPath, '\\');
        if (barra != nullptr) {
            strcpy_s(barra + 1, MAX_PATH - (barra + 1 - g_logPath), "macro-debug.log");
        }
        barra = strrchr(g_cfgPath, '\\');
        if (barra != nullptr) {
            strcpy_s(barra + 1, MAX_PATH - (barra + 1 - g_cfgPath), "macro.txt");
        }
        LeConfig();
        if (!InstalaJmp(kMacroTick, kTickBytes, sizeof(kTickBytes), TickHook)) {
            Log("=== macromago: build diferente, nada instalado");
        }
    }
};

Installer g_macroMago;

} // namespace
