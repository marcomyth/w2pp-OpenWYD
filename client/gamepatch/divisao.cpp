// Dividir pilha: a lista de itens divisíveis do cliente passa a ser a do servidor.
//
// Shift+clique numa pilha abre a caixa de quantidade só se o item estiver numa
// lista fixa do WYD.exe 7662 (0x42052A-0x420626): Poeiras 412/413, 415, Restos
// 419/420, 4049, Âmagos 2390-2419 e 3200-3220. É a lista do legado
// (_MSG_SplitItem.cpp:45-52), e o servidor já divide bem mais que isso
// (internal/pilha): Jóias 2441-2444, Pedra do Sábio, Classes A-E e (P), e o
// resto. Sem isto, o pedido 0x02E5 dessas pilhas nunca saía do cliente — e a +10,
// que quer quatro jóias em quatro células, não tinha como ser montada com uma
// pilha só.
//
// O envio (0x46B41C) não tem lista nenhuma: só confere que a quantidade é menor
// que a pilha. Então basta trocar a decisão.
//
// O desvio entra no lugar de "mov byte [ebp-5Ch],0 / mov ecx,[ebp-60h]", grava em
// [ebp-5Ch] se o item divide e volta em 0x420626, onde o cliente lê essa flag.
// A lista original fica inteira: tirar dela mudaria o que já funcionava.
//
// Instalado por um objeto global, como timerfields.cpp.

#include <windows.h>

#include <cstring>

namespace {

constexpr DWORD kHookAt = 0x42052A;
constexpr DWORD kHookBack = 0x420626; // mov ecx,[ebp-5Ch] / and ecx,0FFh / test
// mov byte ptr [ebp-5Ch],0 / mov ecx,[ebp-60h]
const BYTE kExpected[7] = {0xC6, 0x45, 0xA4, 0x00, 0x8B, 0x4D, 0xA0};

// Divide diz se Shift+clique pode dividir a pilha do item. Espelha
// pilha.Empilha (internal/pilha/pilha.go) e mantém a lista original do cliente.
int __cdecl Divide(int index) {
    switch (index) {
    case 412: case 413: case 414: case 415: case 416: case 419: case 420:
    case 1774:                         // Pedra do Sábio
    case 4010: case 4011: case 4028: case 4029: // Barras de Prata
    case 4049:
        return 1;
    }
    if (index >= 2390 && index <= 2419) return 1; // Âmagos
    if (index >= 2441 && index <= 2444) return 1; // Diamante, Esmeralda, Coral, Garnet
    if (index >= 777 && index <= 785) return 1;   // Pergaminho da Água (M)
    if (index >= 3173 && index <= 3190) return 1; // Pergaminho da Água (N) e (A)
    if (index >= 3200 && index <= 3220) return 1; // lista original do cliente
    if (index >= 4016 && index <= 4025) return 1; // Classe A-E e (P)
    if (index >= 4117 && index <= 4121) return 1; // troféus da Quest 256
    return 0;
}

// [ebp-60h] é o slot sob o mouse; [slot+670h] aponta o STRUCT_ITEM, cujo
// primeiro word é o índice — as mesmas leituras do trecho substituído.
__declspec(naked) void DivisaoHook() {
    __asm {
        mov ecx, dword ptr [ebp - 0x60]
        mov edx, dword ptr [ecx + 0x670]
        movsx eax, word ptr [edx]
        push eax
        call Divide
        add esp, 4
        mov byte ptr [ebp - 0x5C], al
        push 0x420626 // kHookBack
        ret
    }
}

struct DivisaoInstaller {
    DivisaoInstaller() {
        BYTE* target = reinterpret_cast<BYTE*>(kHookAt);
        if (memcmp(target, kExpected, sizeof(kExpected)) != 0) {
            return; // não é a 7662: o cliente fica com a lista dele
        }
        DWORD oldProtect = 0;
        if (!VirtualProtect(target, sizeof(kExpected), PAGE_EXECUTE_READWRITE, &oldProtect)) {
            return;
        }
        target[0] = 0xE9; // jmp rel32
        *reinterpret_cast<DWORD*>(target + 1) =
            reinterpret_cast<DWORD>(&DivisaoHook) - (kHookAt + 5);
        memset(target + 5, 0x90, sizeof(kExpected) - 5);
        VirtualProtect(target, sizeof(kExpected), oldProtect, &oldProtect);
        FlushInstructionCache(GetCurrentProcess(), target, sizeof(kExpected));
    }
} g_divisaoInstaller;

} // namespace
