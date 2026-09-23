// Localizador do botao "Loja Pessoal" dentro do cliente.
//
// O objetivo e entrar pelo caminho do proprio jogo: achar quem abre a janela da
// lojinha pessoal e trocar por nos. O executavel nao tem o nome da janela nem
// tabela de telas - elas sao montadas em codigo -, entao a busca e dinamica.
//
// O metodo e o de sempre para achar variavel de estado: fotografar a secao de
// dados do WYD.exe com a janela FECHADA, deixar o jogador abrir, e ficar com os
// enderecos que mudaram; depois fechar e ficar so com os que voltaram ao valor
// de antes. Duas ou tres rodadas e sobra quase nada.
//
// Os comandos chegam por um arquivo, cmd.txt, ao lado do executavel: assim da
// para conduzir a investigacao de fora sem inventar tecla nenhuma dentro do
// jogo. Cada comando e executado uma vez, quando o arquivo muda.
//
//   snap    - foto da memoria agora (com a janela fechada)
//   abriu   - candidatos = o que mudou desde a foto
//   fechou  - mantem so os que voltaram ao valor da foto
//   listar  - escreve os candidatos no log
//   icone 0 - esconde o nosso icone, para o botao original voltar a receber o
//   icone 1   clique; sem isto nao da para abrir a janela do jogo
//
// Este modulo e de investigacao: sai do build assim que o endereco estiver
// achado e anotado.

#include <windows.h>
#include <tlhelp32.h>

#include <cstdarg>
#include <cstdio>
#include <cstring>

void CamadaLog(const char* texto);
extern "C" void __cdecl LojaMostraIcone(int roubar);

namespace {

// Por padrao a secao .data do WYD.exe, onde moram as variaveis globais.
constexpr DWORD kDataIni = 0x60F000;
constexpr DWORD kDataTam = 0x2172351;

// Mas a interface do cliente vive no HEAP: a raiz (*0x6F0AB0) e um objeto
// grande, com os controles em deslocamentos fixos dentro dele - o tooltip usa
// +0x27A08. Estado de janela e de barra mora ali, e nao na .data. O comando
// "regiao ui" aponta a varredura para esse objeto.
constexpr DWORD kUIRoot = 0x6F0AB0;
constexpr DWORD kUITam = 0x40000;

DWORD g_base = kDataIni;
DWORD g_tam = kDataTam;

// Mais que isto nao e candidato, e ruido: o jogo inteiro muda entre duas fotos.
constexpr int kMaxCandidatos = 4096;

BYTE* g_foto = nullptr;
DWORD g_candidatos[kMaxCandidatos];
DWORD g_valorAberto[kMaxCandidatos];
int g_nCandidatos = 0;
bool g_temCandidatos = false;

void Log(const char* texto) {
    CamadaLog(texto);
}

void Logf(const char* formato, ...) {
    char buf[220];
    va_list args;
    va_start(args, formato);
    vsprintf_s(buf, formato, args);
    va_end(args);
    Log(buf);
}

// Le um dword da memoria do cliente sem explodir se a pagina sumir.
bool LeDword(DWORD endereco, DWORD* saida) {
    __try {
        *saida = *reinterpret_cast<DWORD*>(endereco);
        return true;
    } __except (EXCEPTION_EXECUTE_HANDLER) {
        return false;
    }
}

void Fotografa() {
    if (g_foto == nullptr) {
        g_foto = static_cast<BYTE*>(VirtualAlloc(nullptr, g_tam, MEM_COMMIT | MEM_RESERVE,
                                                 PAGE_READWRITE));
        if (g_foto == nullptr) {
            Log("=== acha: sem memoria para a foto");
            return;
        }
    }
    // Copia pagina a pagina: parte da .data pode nem estar mapeada.
    DWORD copiadas = 0;
    for (DWORD i = 0; i + 4096 <= g_tam; i += 4096) {
        MEMORY_BASIC_INFORMATION mbi;
        const void* p = reinterpret_cast<const void*>(g_base + i);
        if (VirtualQuery(p, &mbi, sizeof(mbi)) == sizeof(mbi) && mbi.State == MEM_COMMIT &&
            (mbi.Protect & PAGE_GUARD) == 0 && (mbi.Protect & PAGE_NOACCESS) == 0) {
            memcpy(g_foto + i, p, 4096);
            ++copiadas;
        } else {
            memset(g_foto + i, 0, 4096);
        }
    }
    g_temCandidatos = false;
    g_nCandidatos = 0;
    Logf("=== acha: foto tirada, %lu paginas", copiadas);
}

// Primeira rodada: tudo que mudou desde a foto vira candidato. Nas seguintes,
// so sobrevive quem mudou de novo.
void MarcaAbriu() {
    if (g_foto == nullptr) {
        Log("=== acha: tire a foto antes (snap)");
        return;
    }
    if (!g_temCandidatos) {
        g_nCandidatos = 0;
        for (DWORD i = 0; i + 4 <= g_tam && g_nCandidatos < kMaxCandidatos; i += 4) {
            DWORD agora = 0;
            const DWORD antes = *reinterpret_cast<DWORD*>(g_foto + i);
            if (!LeDword(g_base + i, &agora) || agora == antes) {
                continue;
            }
            g_candidatos[g_nCandidatos] = g_base + i;
            g_valorAberto[g_nCandidatos] = agora;
            ++g_nCandidatos;
        }
        g_temCandidatos = true;
        Logf("=== acha: %d candidatos na primeira rodada%s", g_nCandidatos,
             g_nCandidatos >= kMaxCandidatos ? " (teto atingido)" : "");
        return;
    }
    int n = 0;
    for (int i = 0; i < g_nCandidatos; ++i) {
        DWORD agora = 0;
        const DWORD antes = *reinterpret_cast<DWORD*>(g_foto + (g_candidatos[i] - g_base));
        if (LeDword(g_candidatos[i], &agora) && agora != antes) {
            g_candidatos[n] = g_candidatos[i];
            g_valorAberto[n] = agora;
            ++n;
        }
    }
    g_nCandidatos = n;
    Logf("=== acha: %d candidatos depois de abrir de novo", g_nCandidatos);
}

// Fechou: quem nao voltou ao valor da foto nao era o estado da janela.
void MarcaFechou() {
    if (!g_temCandidatos) {
        Log("=== acha: nao ha candidatos ainda");
        return;
    }
    int n = 0;
    for (int i = 0; i < g_nCandidatos; ++i) {
        DWORD agora = 0;
        const DWORD antes = *reinterpret_cast<DWORD*>(g_foto + (g_candidatos[i] - g_base));
        if (LeDword(g_candidatos[i], &agora) && agora == antes) {
            g_candidatos[n] = g_candidatos[i];
            g_valorAberto[n] = g_valorAberto[i];
            ++n;
        }
    }
    g_nCandidatos = n;
    Logf("=== acha: %d candidatos depois de fechar", g_nCandidatos);
}

void Lista() {
    Logf("=== acha: %d candidatos", g_nCandidatos);
    const int quantos = g_nCandidatos < 40 ? g_nCandidatos : 40;
    for (int i = 0; i < quantos; ++i) {
        DWORD agora = 0;
        LeDword(g_candidatos[i], &agora);
        const DWORD antes = *reinterpret_cast<DWORD*>(g_foto + (g_candidatos[i] - g_base));
        Logf("    %08X  fechada=%08X  aberta=%08X  agora=%08X", g_candidatos[i], antes,
             g_valorAberto[i], agora);
    }
}

// --- breakpoint de hardware na variavel da janela --------------------------
//
// Achado o endereco (0x60F4FC, o id da janela ativa: 0xFFFF quando nao ha
// nenhuma, 0x1388 com a Loja Pessoal), o que falta e QUEM escreve 0x1388 nele.
// Um breakpoint de dados dispara depois da escrita, entao basta olhar o valor
// que ficou e, no contexto da excecao, seguir a cadeia de EBP para descobrir os
// chamadores - e entre eles esta o tratador do clique no botao.

constexpr DWORD kIdJanela = 0x0060F4FC;
constexpr WORD kLojaPessoal = 0x1388;

PVOID g_tratador = nullptr;
int g_pegas = 0;

LONG CALLBACK Tratador(EXCEPTION_POINTERS* info) {
    if (info->ExceptionRecord->ExceptionCode != EXCEPTION_SINGLE_STEP ||
        (info->ContextRecord->Dr6 & 1) == 0) {
        return EXCEPTION_CONTINUE_SEARCH;
    }
    info->ContextRecord->Dr6 = 0;
    const WORD valor = *reinterpret_cast<WORD*>(kIdJanela);
    if (valor == kLojaPessoal && g_pegas < 8) {
        ++g_pegas;
        Logf("=== acha: escreveu %04X em %08X, eip=%08X", valor, kIdJanela,
             info->ContextRecord->Eip);
        DWORD ebp = info->ContextRecord->Ebp;
        for (int nivel = 0; nivel < 6 && ebp > 0x10000; ++nivel) {
            DWORD retorno = 0;
            DWORD anterior = 0;
            if (!LeDword(ebp + 4, &retorno) || !LeDword(ebp, &anterior)) {
                break;
            }
            Logf("      chamador %d: %08X", nivel, retorno);
            ebp = anterior;
        }
    }
    return EXCEPTION_CONTINUE_EXECUTION;
}

// Os registradores de depuracao sao por thread, entao o breakpoint entra em
// todas as threads do processo.
void MexeNasThreads(bool ligar) {
    HANDLE foto = CreateToolhelp32Snapshot(TH32CS_SNAPTHREAD, 0);
    if (foto == INVALID_HANDLE_VALUE) {
        return;
    }
    THREADENTRY32 te;
    te.dwSize = sizeof(te);
    const DWORD meu = GetCurrentProcessId();
    const DWORD euMesmo = GetCurrentThreadId();
    int quantas = 0;
    if (Thread32First(foto, &te)) {
        do {
            if (te.th32OwnerProcessID != meu || te.th32ThreadID == euMesmo) {
                continue;
            }
            HANDLE h = OpenThread(THREAD_GET_CONTEXT | THREAD_SET_CONTEXT | THREAD_SUSPEND_RESUME,
                                  FALSE, te.th32ThreadID);
            if (h == nullptr) {
                continue;
            }
            SuspendThread(h);
            CONTEXT ctx;
            memset(&ctx, 0, sizeof(ctx));
            ctx.ContextFlags = CONTEXT_DEBUG_REGISTERS;
            if (GetThreadContext(h, &ctx)) {
                if (ligar) {
                    ctx.Dr0 = kIdJanela;
                    // L0 ligado; RW0 = 01 (escrita); LEN0 = 01 (duas palavras)
                    ctx.Dr7 = (ctx.Dr7 & ~0xF0000u) | (1u << 0) | (1u << 16) | (1u << 18);
                } else {
                    ctx.Dr0 = 0;
                    ctx.Dr7 &= ~(1u << 0);
                }
                ctx.ContextFlags = CONTEXT_DEBUG_REGISTERS;
                if (SetThreadContext(h, &ctx)) {
                    ++quantas;
                }
            }
            ResumeThread(h);
            CloseHandle(h);
        } while (Thread32Next(foto, &te));
    }
    CloseHandle(foto);
    Logf("=== acha: breakpoint %s em %d threads", ligar ? "ligado" : "desligado", quantas);
}

void Vigia(bool ligar) {
    if (ligar && g_tratador == nullptr) {
        g_tratador = AddVectoredExceptionHandler(1, Tratador);
    }
    g_pegas = 0;
    MexeNasThreads(ligar);
}

// --- desvio nas quatro escritas --------------------------------------------
//
// O breakpoint de hardware nao pegou nada: o protetor do cliente limpa os
// registradores de depuracao. Como as instrucoes que escrevem o id da janela ja
// sao conhecidas, o caminho passa a ser desviar o proprio codigo.
//
//   00416FA9  mov word ptr [60F4FC], cx   (7 bytes)
//   004173D3  mov word ptr [60F4FC], cx   (7)
//   0041790E  mov word ptr [60F4FC], cx   (7)
//   00417281  mov word ptr [60F4FC], ax   (6)
//
// Cada desvio roda a instrucao original num trecho nosso, anota quem passou por
// ali e volta. Assim descobrimos qual deles abre a Loja Pessoal e de onde veio.

struct Sitio {
    DWORD endereco;
    int tamanho;
};

const Sitio kSitios[4] = {{0x00416FA9, 7}, {0x004173D3, 7}, {0x0041790E, 7}, {0x00417281, 6}};
int g_anotadas = 0;

extern "C" void __cdecl AnotaEscrita(int sitio, DWORD ebp) {
    if (g_anotadas >= 12) {
        return;
    }
    const WORD valor = *reinterpret_cast<WORD*>(kIdJanela);
    if (valor == 0xFFFF) {
        return;   // fechando janela: nao interessa
    }
    ++g_anotadas;
    Logf("=== acha: sitio %d escreveu id %04X", sitio, valor);
    DWORD quadro = ebp;
    for (int nivel = 0; nivel < 5 && quadro > 0x10000; ++nivel) {
        DWORD retorno = 0;
        DWORD anterior = 0;
        if (!LeDword(quadro + 4, &retorno) || !LeDword(quadro, &anterior)) {
            break;
        }
        Logf("      chamador %d: %08X", nivel, retorno);
        quadro = anterior;
    }
}

void EscreveRel(BYTE* onde, DWORD destino) {
    const DWORD rel = destino - (reinterpret_cast<DWORD>(onde) + 4);
    memcpy(onde, &rel, sizeof(rel));
}

void DesviaSitios() {
    for (int i = 0; i < 4; ++i) {
        const Sitio& s = kSitios[i];
        BYTE* trecho = static_cast<BYTE*>(
            VirtualAlloc(nullptr, 64, MEM_COMMIT | MEM_RESERVE, PAGE_EXECUTE_READWRITE));
        if (trecho == nullptr) {
            continue;
        }
        int n = 0;
        memcpy(trecho, reinterpret_cast<void*>(s.endereco), s.tamanho);   // instrucao original
        n += s.tamanho;
        trecho[n++] = 0x60;                       // pushad
        trecho[n++] = 0x9C;                       // pushfd
        trecho[n++] = 0x55;                       // push ebp
        trecho[n++] = 0x6A;                       // push i
        trecho[n++] = static_cast<BYTE>(i);
        trecho[n++] = 0xE8;                       // call AnotaEscrita
        EscreveRel(trecho + n, reinterpret_cast<DWORD>(&AnotaEscrita));
        n += 4;
        trecho[n++] = 0x83;                       // add esp, 8
        trecho[n++] = 0xC4;
        trecho[n++] = 0x08;
        trecho[n++] = 0x9D;                       // popfd
        trecho[n++] = 0x61;                       // popad
        trecho[n++] = 0xE9;                       // jmp de volta
        EscreveRel(trecho + n, s.endereco + s.tamanho);

        BYTE salto[8];
        memset(salto, 0x90, sizeof(salto));
        salto[0] = 0xE9;
        const DWORD rel = reinterpret_cast<DWORD>(trecho) - (s.endereco + 5);
        memcpy(salto + 1, &rel, sizeof(rel));
        DWORD antes = 0;
        if (!VirtualProtect(reinterpret_cast<void*>(s.endereco), s.tamanho,
                            PAGE_EXECUTE_READWRITE, &antes)) {
            Logf("=== acha: sitio %d recusou VirtualProtect (erro %lu)", i, GetLastError());
            continue;
        }
        memcpy(reinterpret_cast<void*>(s.endereco), salto, s.tamanho);
        VirtualProtect(reinterpret_cast<void*>(s.endereco), s.tamanho, antes, &antes);
        FlushInstructionCache(GetCurrentProcess(), reinterpret_cast<void*>(s.endereco), s.tamanho);
    }
    g_anotadas = 0;
    Log("=== acha: quatro escritas desviadas");
}

// Os desvios entraram mesmo? O protetor do cliente pode recusar a escrita ou
// restaurar a pagina depois.
void Confere() {
    for (int i = 0; i < 4; ++i) {
        const BYTE* p = reinterpret_cast<const BYTE*>(kSitios[i].endereco);
        Logf("=== acha: sitio %d em %08X: %02X %02X %02X %02X %02X %02X %02X", i,
             kSitios[i].endereco, p[0], p[1], p[2], p[3], p[4], p[5], p[6]);
    }
    Logf("=== acha: id da janela ativa agora = %04X", *reinterpret_cast<WORD*>(kIdJanela));
}

void Executa(const char* linha) {
    if (strncmp(linha, "snap", 4) == 0) {
        Fotografa();
    } else if (strncmp(linha, "abriu", 5) == 0) {
        MarcaAbriu();
    } else if (strncmp(linha, "fechou", 6) == 0) {
        MarcaFechou();
    } else if (strncmp(linha, "regiao", 6) == 0) {
        if (strstr(linha, "ui") != nullptr) {
            DWORD raiz = 0;
            if (!LeDword(kUIRoot, &raiz) || raiz < 0x10000) {
                Log("=== acha: a raiz da interface ainda nao existe");
                return;
            }
            g_base = raiz;
            g_tam = kUITam;
            Logf("=== acha: varredura no objeto da interface, %08X + %X", g_base, g_tam);
        } else {
            g_base = kDataIni;
            g_tam = kDataTam;
            Log("=== acha: varredura na .data do executavel");
        }
        if (g_foto != nullptr) {
            VirtualFree(g_foto, 0, MEM_RELEASE);
            g_foto = nullptr;
        }
        g_temCandidatos = false;
        g_nCandidatos = 0;
    } else if (strncmp(linha, "listar", 6) == 0) {
        Lista();
    } else if (strncmp(linha, "conferir", 8) == 0) {
        Confere();
    } else if (strncmp(linha, "espiar", 6) == 0) {
        DesviaSitios();
    } else if (strncmp(linha, "vigiar", 6) == 0) {
        Vigia(true);
    } else if (strncmp(linha, "parar", 5) == 0) {
        Vigia(false);
    } else if (strncmp(linha, "icone", 5) == 0) {
        const int mostrar = atoi(linha + 5);
        LojaMostraIcone(mostrar);
        Logf("=== acha: icone %s", mostrar ? "ligado" : "desligado");
    } else {
        Logf("=== acha: comando desconhecido '%s'", linha);
    }
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
        if (!GetFileAttributesExA(caminho, GetFileExInfoStandard, &info)) {
            continue;
        }
        if (CompareFileTime(&info.ftLastWriteTime, &ultima) == 0) {
            continue;
        }
        ultima = info.ftLastWriteTime;
        FILE* f = nullptr;
        if (fopen_s(&f, caminho, "r") != 0 || f == nullptr) {
            continue;
        }
        char linha[128];
        if (fgets(linha, sizeof(linha), f) != nullptr) {
            char* fim = linha + strlen(linha);
            while (fim > linha && (fim[-1] == 10 || fim[-1] == 13 || fim[-1] == 32)) {
                *--fim = 0;
            }
            if (linha[0] != 0) {
                Executa(linha);
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
