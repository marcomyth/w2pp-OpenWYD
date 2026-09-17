// A porcentagem de dano dos acessórios de Hércules e Hecate na conta de dano de
// skill do cliente.
//
// Desde a reforma dos acessórios (16/09/2026) o Hércules dá % de dano físico
// (EF_DANOFISICO, 89) e o Hecate % de dano mágico (EF_DANOMAGICO, 90). O servidor
// aplica as duas dentro de SkillBaseDamage (tmserver/internal/combat/skill.go),
// mas o WYD.exe 7662 tem a própria cópia de BASE_GetSkillDamage (0x542AA7) e é
// ela que a janela C imprime como "Atq Mágico" (0x44A9DF) e os tooltips de skill
// mostram (0x418CEA, 0x453BCE). Sem isto, trocar um Brinco de Hecate +9 por um
// +15 não mexia no número da janela, embora o golpe subisse.
//
// Dois desvios, cada um no ponto exato em que o servidor aplica a sua %:
//
//   - 0x542FCC, ramo sem Magia (2ª árvore do TK e Huntress): antes do 5/4,
//     dam = dam × (100 + físico) / 100;
//   - 0x542FF4, ramo com Magia: logo depois de (4×Magia+100)×dam/100 e antes do
//     5/4, dam = dam × (100 + mágico) / 100.
//
// A mesma aritmética inteira do servidor, na mesma ordem, então a janela bate com
// o golpe. As poções continuam fora da janela, como já estavam.
//
// A soma percorre o Equip do STRUCT_MOB (+0x8C, 16 × 8 bytes) com a leitora de
// efeito do tooltip (0x53821E), que já aplica o refino do cliente — a mesma conta
// que faz o tooltip mostrar 32% num Brinco +15. O slot da montaria (14) fica fora,
// como no servidor.
//
// Instalado por um objeto global, como timerfields.cpp.

#include <windows.h>

#include <cstring>

namespace {

constexpr DWORD kGetItemAbility = 0x53821E; // int __cdecl(STRUCT_ITEM*, int efeito)
constexpr int kEquipOffset = 0x8C;
constexpr int kEquipSlots = 16;
constexpr int kItemSize = 8;
constexpr int kMountSlot = 14;

constexpr int kEfDanoFisico = 89;
constexpr int kEfDanoMagico = 90;

constexpr DWORD kFisicoAt = 0x542FCC;
constexpr DWORD kMagicoAt = 0x542FF4;

// mov eax,[ebp-14h] / imul eax,eax,5
const BYTE kFisicoBytes[6] = {0x8B, 0x45, 0xEC, 0x6B, 0xC0, 0x05};
// mov [ebp-14h],eax / mov eax,[ebp-14h]
const BYTE kMagicoBytes[6] = {0x89, 0x45, 0xEC, 0x8B, 0x45, 0xEC};

// SomaPct soma o efeito em todo o equipamento do personagem, montaria fora.
int __cdecl SomaPct(const BYTE* mob, int efeito) {
    if (mob == nullptr) {
        return 0;
    }
    auto ler = reinterpret_cast<int(__cdecl*)(const BYTE*, int)>(kGetItemAbility);
    int soma = 0;
    for (int slot = 0; slot < kEquipSlots; slot++) {
        if (slot == kMountSlot) {
            continue;
        }
        soma += ler(mob + kEquipOffset + slot * kItemSize, efeito);
    }
    return soma;
}

// O STRUCT_MOB é o segundo argumento de BASE_GetSkillDamage ([ebp+0Ch]). Os
// saltos de volta são literais (push/ret), como em timerfields.cpp. ecx e edx
// podem sair sujos: o código seguinte refaz os dois antes de ler.
__declspec(naked) void FisicoHook() {
    __asm {
        push dword ptr [ebp - 0x14]
        push 89 // kEfDanoFisico
        push dword ptr [ebp + 0xC]
        call SomaPct
        add esp, 8
        mov ecx, eax
        pop eax
        test ecx, ecx
        jle fisico_fim
        add ecx, 100
        imul eax, ecx
        cdq
        mov ecx, 100
        idiv ecx
    fisico_fim:
        imul eax, eax, 5
        push 0x542FD2 // kFisicoAt + 6: cdq / and edx,3 / add / sar
        ret
    }
}

__declspec(naked) void MagicoHook() {
    __asm {
        push eax
        push 90 // kEfDanoMagico
        push dword ptr [ebp + 0xC]
        call SomaPct
        add esp, 8
        mov ecx, eax
        pop eax
        test ecx, ecx
        jle magico_fim
        add ecx, 100
        imul eax, ecx
        cdq
        mov ecx, 100
        idiv ecx
    magico_fim:
        mov dword ptr [ebp - 0x14], eax
        push 0x542FFA // kMagicoAt + 6: imul eax,eax,5
        ret
    }
}

// Confere os bytes antes de gravar: se não forem os da 7662, a janela fica como o
// cliente a faz, em vez de o jogo cair.
bool Desviar(DWORD at, const BYTE* esperado, size_t n, void* hook) {
    BYTE* target = reinterpret_cast<BYTE*>(at);
    if (memcmp(target, esperado, n) != 0) {
        return false;
    }
    DWORD oldProtect = 0;
    if (!VirtualProtect(target, n, PAGE_EXECUTE_READWRITE, &oldProtect)) {
        return false;
    }
    target[0] = 0xE9; // jmp rel32
    *reinterpret_cast<DWORD*>(target + 1) = reinterpret_cast<DWORD>(hook) - (at + 5);
    memset(target + 5, 0x90, n - 5);
    VirtualProtect(target, n, oldProtect, &oldProtect);
    FlushInstructionCache(GetCurrentProcess(), target, n);
    return true;
}

struct AcessorioPctInstaller {
    AcessorioPctInstaller() {
        Desviar(kFisicoAt, kFisicoBytes, sizeof(kFisicoBytes), &FisicoHook);
        Desviar(kMagicoAt, kMagicoBytes, sizeof(kMagicoBytes), &MagicoHook);
    }
} g_acessorioPctInstaller;

} // namespace
