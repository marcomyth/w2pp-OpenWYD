// Olhos de Águia (Huntress, skill 91): o alcance da Garra — REGRA DO SERVIDOR,
// decidida pelo Marco em 17/09/2026.
//
// O alcance do golpe físico é decidido pelo cliente: o servidor aceita até 23
// casas (playerMeleeReach). O WYD.exe calcula o alcance em BASE_GetMobAbility
// (0x539C2C) com EF_RANGE (0x1B): o maior EF_RANGE do equipamento e, na
// Huntress com o bit 19 (Olhos de Águia, 0x80000), alcance 2 para arma de
// alcance 1, com qualquer arma. Quem chama soma +1 da Força Espectral (bit 29)
// por fora (0x459907, 0x45DE6E, 0x45ED45, 0x4971D7).
//
// A regra agora: com Garra (EF_WTYPE 41) na mão direita, +1 de alcance, ou +2
// com a Invisibilidade aprendida (bit 23, a 8ª skill da árvore). Com outra arma,
// nada. Soma com a Espectral, que continua sendo somada por quem chama.
//
// O desvio cobre os 6 primeiros bytes da função (push ebp / mov ebp,esp /
// sub esp,70h) e só age no EF_RANGE; o resto segue para a original. Instalado
// por um objeto global, como timerfields.cpp.

#include <windows.h>

#include <cstring>

namespace {

constexpr DWORD kMobAbility = 0x539C2C;    // int __cdecl BASE_GetMobAbility(STRUCT_MOB*, uchar)
constexpr DWORD kMaxAbility = 0x539F57;    // int __cdecl BASE_GetMaxAbility(STRUCT_MOB*, uchar)
constexpr DWORD kItemAbility = 0x537962;   // int __cdecl BASE_GetItemAbility(STRUCT_ITEM*, uchar)
const BYTE kExpectedPrologue[9] = {0x55, 0x8B, 0xEC, 0x83, 0xEC, 0x70, 0x56, 0xC7, 0x45};

constexpr int kEfRange = 0x1B;
constexpr int kEfWType = 21;
constexpr int kGarra = 41;
constexpr DWORD kClassOffset = 0x14;       // STRUCT_MOB.Class
constexpr DWORD kLearnedOffset = 0x30C;    // STRUCT_MOB.LearnedSkill
constexpr DWORD kWeaponOffset = 0x8C + 6 * 8; // STRUCT_MOB.Equip[6]
constexpr DWORD kOlhosDeAguia = 0x80000;   // bit 19, skill 91
constexpr DWORD kInvisibilidade = 0x800000; // bit 23, skill 95

typedef int(__cdecl* AbilityFn)(void*, int);

int __cdecl OriginalMobAbility(void* mob, int type);

int __cdecl HookedMobAbility(void* mob, int type) {
    if ((type & 0xFF) != kEfRange || mob == nullptr) {
        return OriginalMobAbility(mob, type);
    }
    const BYTE* m = static_cast<const BYTE*>(mob);
    int value = reinterpret_cast<AbilityFn>(kMaxAbility)(mob, kEfRange);
    DWORD learned = 0;
    memcpy(&learned, m + kLearnedOffset, sizeof(learned));
    if (m[kClassOffset] == 3 && (learned & kOlhosDeAguia) != 0) {
        void* weapon = static_cast<BYTE*>(mob) + kWeaponOffset;
        if (reinterpret_cast<AbilityFn>(kItemAbility)(weapon, kEfWType) == kGarra) {
            value += (learned & kInvisibilidade) != 0 ? 2 : 1;
        }
    }
    return value;
}

// Os 6 bytes cobertos e o salto de volta (literal, como em timerfields.cpp).
__declspec(naked) int __cdecl OriginalMobAbility(void*, int) {
    __asm {
        push ebp
        mov ebp, esp
        sub esp, 0x70
        push 0x539C32
        ret
    }
}

struct OlhosDeAguiaInstaller {
    OlhosDeAguiaInstaller() {
        BYTE* target = reinterpret_cast<BYTE*>(kMobAbility);
        if (memcmp(target, kExpectedPrologue, sizeof(kExpectedPrologue)) != 0) {
            return; // outra build
        }
        DWORD oldProtect = 0;
        if (!VirtualProtect(target, 6, PAGE_EXECUTE_READWRITE, &oldProtect)) {
            return;
        }
        target[0] = 0xE9; // jmp rel32
        const DWORD rel = reinterpret_cast<DWORD>(&HookedMobAbility) - (kMobAbility + 5);
        memcpy(target + 1, &rel, sizeof(rel));
        target[5] = 0x90;
        VirtualProtect(target, 6, oldProtect, &oldProtect);
        FlushInstructionCache(GetCurrentProcess(), target, 6);
    }
};

OlhosDeAguiaInstaller g_olhosDeAguia;

} // namespace
