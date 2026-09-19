// Sistema de alvos, versão 1: escanear, escolher e travar — com a lista saindo
// na área de mensagens do jogo. O painel desenhado vem depois; aqui o objetivo é
// validar a lógica (ordem, filtros, travar/soltar) com o mínimo de invenção.
//
// Teclas:
//   '      escaneia e lista os alvos por perto, numerados por distância
//   1..9,0 escolhe o alvo daquele número e TRAVA nele (0 = décimo)
//   Shift esquerdo solta o alvo
//
// Tab e Esc foram trocados a pedido: no cliente, Tab muda a câmera e Esc abre o
// menu. O apóstrofo muda de código conforme o layout do teclado, então as duas
// teclas são configuráveis em alvo.txt e, por padrão, o scan aceita os dois
// códigos em que o apóstrofo costuma cair (0xDB e 0xDE).
//
// Como cada peça funciona:
//
//   Lista de entidades — cena(0x6F0AB0) +0x34 -> +0x10, próximo em +0xC; posição
//   em +0x28/+0x2C (float), id em +0x20, nome em +0xF0 (16 bytes). O nome em
//   +0xF0 saiu do despejo de memória: "testezzz" e "Tauron" no mesmo lugar.
//
//   Jogador ou monstro — id < 1000 é jogador (MAX_USER), como o próprio cliente
//   decide em 0x5162B3 (`cmp [eax+0x20], 0x3E8`).
//
//   Atacar — 0x5162B3(self, alvo, 1): a mesma função que o clique usa. Travar é
//   chamá-la a cada quadro, que é o que o macro já faz. Neste cliente não existe
//   "selecionar" separado de "atacar": o clique ataca.
//
//   Escrever na tela — o cliente imprime a linha da área de mensagens ao receber
//   o pacote 0x0101. Em vez de desenhar texto, montamos esse pacote e entregamos
//   ao próprio tratador do cliente (0x49E4C5), com o `this` capturado de uma
//   chamada real (desvio em 0x495AC6). O jogo imprime como se viesse do servidor.
//
// alvo.txt, na pasta do cliente:
//   modo=ambos | monstros | players
//   max=10

#include "teclas.h"

#include <windows.h>

#include <cstdio>
#include <cstring>

// O painel na tela (painel.cpp) lê estes dados; quem preenche é a varredura.
struct AlvoPainel {
    char nome[17];
    int dist;
    bool jogador;
    bool travado;
    int vidaPct;
    int rel;  // 0 monstro, 1 aliado (mesma capa), 2 inimigo
};

AlvoPainel g_painelLista[20];
int g_painelN = 0;
bool g_painelAberto = false;

void PainelInstala();
void PainelNovoQuadro();

namespace {

constexpr DWORD kScene = 0x6F0AB0;
constexpr DWORD kSelfOff = 0x4C;
constexpr DWORD kListOff = 0x34;
constexpr DWORD kIdOff = 0x20;
constexpr DWORD kNameOff = 0xF0;
constexpr DWORD kNextOff = 0xC;
constexpr int kMaxUser = 1000;

constexpr DWORD kDistance = 0x541324;
constexpr DWORD kRoute = 0x54139A;
constexpr DWORD kAttack = 0x5162B3;
// Lançar magia no alvo: a mesma chamada do macro mágico do cliente (0x497447).
// Ordem dos argumentos tirada de lá: x, y inteiros do alvo, a posição dele em
// três floats (x, z, y), dois zeros e o ponteiro do alvo; `this` é a cena.
constexpr DWORD kCast = 0x4567CE;
constexpr DWORD kHeightMap = 0x6F0AB0;
constexpr int kRouteSlope = 8;

constexpr DWORD kDispatcher = 0x495AC6;      // thiscall(this, arg1, pacote)
constexpr DWORD kDispatcherBack = 0x495ACC;  // depois de push ebp/mov ebp,esp/sub esp,0xC
const BYTE kDispatcherBytes[6] = {0x55, 0x8B, 0xEC, 0x83, 0xEC, 0x0C};
constexpr DWORD kHandlePacket = 0x49E4C5;    // o que trata o 0x101
constexpr WORD kMsgPanel = 0x0101;

constexpr int kMaxLista = 20;  // teto da lista; o painel mostra quantos couberem
constexpr int kAlcanceDefault = 15;  // teto do cliente (0x5162B3 recusa acima disso)

// Capa: o código do item fica em +0x0A58 na entidade — achado comparando o mesmo
// offset em várias delas, com a minha servindo de gabarito (546, Akelonia, igual
// ao Equip[15] da minha STRUCT_MOB). Vale também para os guardas de reino, que
// usam capa; por isso a regra de aliado cobre NPCs sem precisar do servidor.
constexpr DWORD kCapaOff = 0x0A58;
constexpr DWORD kPlayerBlock = 0x277C024;
constexpr DWORD kMyMobOff = 0x750;
constexpr DWORD kEquipCapaOff = 0x104; // Equip[15].sIndex na STRUCT_MOB

// internal/reinos/reinos.go: qualquer outra capa (branca, verde, sem capa) dá 0,
// e 0 nunca é aliado de ninguém.
const short kCapasHekalotia[] = {543, 545, 734, 736, 3191, 3194, 3197};
const short kCapasAkelonia[] = {544, 546, 735, 737, 3192, 3195, 3198};

int ReinoDaCapa(short v) {
    for (short c : kCapasHekalotia) {
        if (c == v) {
            return 7;
        }
    }
    for (short c : kCapasAkelonia) {
        if (c == v) {
            return 8;
        }
    }
    return 0;
}

typedef int(__cdecl* DistanceFn)(int, int, int, int);
typedef void(__cdecl* RouteFn)(int, int, int*, int*, void*, int);
typedef int(__thiscall* AttackFn)(void*, void*, int);
typedef int(__thiscall* CastFn)(void*, int, int, float, float, float, int, int, void*);
typedef int(__thiscall* HandleFn)(void*, int, void*);

struct Alvo {
    BYTE* ent;
    int id;
    int dist;
    char nome[17];
    int rel;
};

Alvo g_lista[kMaxLista];
int g_n = 0;
BYTE* g_travado = nullptr;
int g_travadoId = 0;

int g_modo = 0; // 0 ambos, 1 só monstros, 2 só players
int g_ocultarAliados = 1;
int g_max = kMaxLista;
int g_alcance = kAlcanceDefault;
int g_ataque = 0;   // 0 auto (segue o macro/classe), 1 fisico, 2 magico
int g_teclaScan = 0;             // 0 = padrão (apóstrofo nos dois códigos)
int g_teclaSoltar = VK_LSHIFT;
int g_descobrir = 1;             // anota no log o código de cada tecla apertada
int g_mensagens = 0;             // desligado: quem mostra a lista e o painel

// capturados de uma chamada real do tratador de pacotes
void* g_dispThis = nullptr;
int g_dispArg1 = 0;
bool g_dispPronto = false;

char g_dir[MAX_PATH] = {0};

void Log(const char* fmt, ...) {
    static int linhas = 0;
    if (linhas > 300) {
        return;
    }
    char caminho[MAX_PATH];
    sprintf_s(caminho, "%salvos.log", g_dir);
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

// ---- escrever uma linha na área de mensagens do jogo ----------------------
void Mensagem(const char* texto) {
    if (!g_mensagens || !g_dispPronto || g_dispThis == nullptr) {
        return;
    }
    BYTE pacote[128];
    memset(pacote, 0, sizeof(pacote));
    size_t n = strlen(texto);
    if (n > 90) {
        n = 90;
    }
    WORD tam = static_cast<WORD>(12 + n + 1);
    memcpy(pacote + 0, &tam, 2);      // Size
    pacote[2] = 0;                    // KeyWord
    pacote[3] = 0;                    // CheckSum
    memcpy(pacote + 4, &kMsgPanel, 2); // Type
    memcpy(pacote + 12, texto, n);    // corpo: o texto do painel
    reinterpret_cast<HandleFn>(kHandlePacket)(g_dispThis, g_dispArg1, pacote);
}

// ---- varredura -----------------------------------------------------------
int Coord(const void* p, DWORD off) {
    float v = 0.0f;
    memcpy(&v, static_cast<const BYTE*>(p) + off, sizeof(v));
    return static_cast<int>(v);
}

bool CaminhoLivre(int myX, int myY, int mx, int my) {
    int tx = mx, ty = my;
    void* map = reinterpret_cast<BYTE*>(*reinterpret_cast<DWORD*>(kHeightMap)) + 0xD4;
    reinterpret_cast<RouteFn>(kRoute)(myX, myY, &tx, &ty, map, kRouteSlope);
    return tx == mx && ty == my;
}

void Escanear() {
    g_n = 0;
    BYTE* scene = *reinterpret_cast<BYTE**>(kScene);
    if (scene == nullptr) {
        return;
    }
    BYTE* self = *reinterpret_cast<BYTE**>(scene + kSelfOff);
    void* list = *reinterpret_cast<void**>(scene + kListOff);
    if (self == nullptr || list == nullptr) {
        return;
    }
    int myX = Coord(self, 0x28), myY = Coord(self, 0x2C);

    // Meu reino sai da MINHA STRUCT_MOB (Equip[15]), que é o valor autoritativo;
    // o +0x0A58 da entidade é a cópia visual que serve para os outros.
    int meuReino = 0;
    BYTE* bloco = *reinterpret_cast<BYTE**>(kPlayerBlock);
    if (bloco != nullptr) {
        short minhaCapa = 0;
        memcpy(&minhaCapa, bloco + kMyMobOff + kEquipCapaOff, sizeof(minhaCapa));
        meuReino = ReinoDaCapa(minhaCapa);
    }

    BYTE* node = *reinterpret_cast<BYTE**>(static_cast<BYTE*>(list) + 0x10);
    for (int guard = 0; node != nullptr && *reinterpret_cast<const int*>(node + kNextOff) != 0 && guard < 300; ++guard) {
        BYTE* ent = node;
        node = *reinterpret_cast<BYTE**>(node + kNextOff);
        if (ent == self) {
            continue;
        }
        int id = *reinterpret_cast<const int*>(ent + kIdOff);
        if (id <= 0) {
            continue; // entidade sem id válido (efeito, objeto de cena)
        }
        bool jogador = (id < kMaxUser);
        if ((g_modo == 1 && jogador) || (g_modo == 2 && !jogador)) {
            continue;
        }
        // Aliado é quem usa capa do MESMO reino que a minha. Sem capa, capa
        // branca e capa verde dão reino 0 e entram na lista como alvo.
        short capa = 0;
        memcpy(&capa, ent + kCapaOff, sizeof(capa));
        const int reinoDele = ReinoDaCapa(capa);
        const bool aliado = (reinoDele != 0 && reinoDele == meuReino);
        if (g_ocultarAliados && aliado) {
            continue;
        }
        // Cor da linha no painel: aliado, inimigo ou monstro. Jogador sem capa e
        // capa de outro reino contam como inimigo; monstro comum fica neutro.
        const int rel = aliado ? 1 : ((jogador || reinoDele != 0) ? 2 : 0);
        int d = reinterpret_cast<DistanceFn>(kDistance)(myX, myY, Coord(ent, 0x28), Coord(ent, 0x2C));
        if (d <= 0 || d > g_alcance || !CaminhoLivre(myX, myY, Coord(ent, 0x28), Coord(ent, 0x2C))) {
            continue;
        }
        // insere mantendo a ordem por distância
        int pos = g_n;
        while (pos > 0 && g_lista[pos - 1].dist > d) {
            if (pos < g_max) {
                g_lista[pos] = g_lista[pos - 1];
            }
            --pos;
        }
        if (pos < g_max) {
            g_lista[pos].ent = ent;
            g_lista[pos].id = id;
            g_lista[pos].dist = d;
            g_lista[pos].rel = rel;
            memcpy(g_lista[pos].nome, ent + kNameOff, 16);
            g_lista[pos].nome[16] = '\0';
            if (g_n < g_max) {
                ++g_n;
            }
        }
    }
}

// Copia a lista para o painel da tela e marca quais linhas estão travadas.
void PublicaNoPainel() {
    g_painelN = g_n;
    for (int i = 0; i < g_n && i < kMaxLista; ++i) {
        memcpy(g_painelLista[i].nome, g_lista[i].nome, sizeof(g_painelLista[i].nome));
        g_painelLista[i].dist = g_lista[i].dist;
        g_painelLista[i].jogador = (g_lista[i].id > 0 && g_lista[i].id < kMaxUser);
        g_painelLista[i].travado = (g_travado != nullptr && g_lista[i].ent == g_travado);
        g_painelLista[i].rel = g_lista[i].rel;
        g_painelLista[i].vidaPct = -1; // ainda não sabemos ler a vida
    }
    // NAO fecha por lista vazia: some a engrenagem junto, e aí não há como
    // desfazer o filtro que esvaziou a lista. Quem abre e fecha é o usuário.
}

void MostrarLista() {
    if (g_n == 0) {
        Mensagem("[Alvos] nada por perto.");
        return;
    }
    // duas linhas de até cinco, para caber na largura do painel
    char linha[128];
    int escritos = 0;
    linha[0] = '\0';
    strcat_s(linha, "[Alvos] ");
    for (int i = 0; i < g_n; ++i) {
        char item[48];
        sprintf_s(item, "%d:%s(%d) ", (i + 1) % 10, g_lista[i].nome, g_lista[i].dist);
        if (strlen(linha) + strlen(item) > 88) {
            Mensagem(linha);
            linha[0] = '\0';
            strcat_s(linha, "[Alvos] ");
        }
        strcat_s(linha, item);
        ++escritos;
    }
    if (escritos > 0) {
        Mensagem(linha);
    }
}

void Travar(int indice) {
    if (indice < 0 || indice >= g_n) {
        return;
    }
    g_travado = g_lista[indice].ent;
    g_travadoId = g_lista[indice].id;
    for (int i = 0; i < g_painelN && i < kMaxLista; ++i) {
        g_painelLista[i].travado = (i == indice);
    }
    char msg[96];
    sprintf_s(msg, "[Alvos] travado em %s", g_lista[indice].nome);
    Mensagem(msg);
}

void Soltar() {
    if (g_travado == nullptr) {
        return;
    }
    g_travado = nullptr;
    g_travadoId = 0;
    for (int i = 0; i < g_painelN && i < kMaxLista; ++i) {
        g_painelLista[i].travado = false;
    }
    Mensagem("[Alvos] alvo solto.");
}

// O alvo travado ainda está na lista da cena e com o mesmo id? Sem essa
// conferência, um ponteiro de entidade que morreu vira lixo.
bool AlvoValido(BYTE* scene, BYTE* self) {
    void* list = *reinterpret_cast<void**>(scene + kListOff);
    if (list == nullptr) {
        return false;
    }
    BYTE* node = *reinterpret_cast<BYTE**>(static_cast<BYTE*>(list) + 0x10);
    for (int guard = 0; node != nullptr && *reinterpret_cast<const int*>(node + kNextOff) != 0 && guard < 300; ++guard) {
        if (node == g_travado && node != self) {
            return *reinterpret_cast<const int*>(node + kIdOff) == g_travadoId;
        }
        node = *reinterpret_cast<BYTE**>(node + kNextOff);
    }
    return false;
}

// Devolve true se a magia saiu. O cliente recusa (0) quando está fora de
// alcance, sem mana ou em recarga — e aí o chamador aproxima.
bool LancaMagia(BYTE* scene, BYTE* alvo) {
    float ax = 0.0f, ay = 0.0f, az = 0.0f;
    memcpy(&ax, alvo + 0x28, sizeof(ax));
    memcpy(&ay, alvo + 0x2C, sizeof(ay));
    memcpy(&az, alvo + 0x30, sizeof(az));
    return reinterpret_cast<CastFn>(kCast)(scene, static_cast<int>(ax), static_cast<int>(ay),
                                           ax, az, ay, 0, 0, alvo) != 0;
}

void Perseguir() {
    BYTE* scene = *reinterpret_cast<BYTE**>(kScene);
    if (scene == nullptr || g_travado == nullptr) {
        return;
    }
    BYTE* self = *reinterpret_cast<BYTE**>(scene + kSelfOff);
    if (self == nullptr) {
        return;
    }
    if (!AlvoValido(scene, self)) {
        g_travado = nullptr;
        g_travadoId = 0;
        Mensagem("[Alvos] alvo sumiu.");
        return;
    }

    // Físico ou mágico? Mago tem de lançar magia, não bater.
    //   ataque=auto   segue o tipo do macro (0x63A29C: 2 = mágico); sem macro
    //                 ligado, usa magia se a classe for Foema (Class 1).
    //   ataque=fisico / magico  força um dos dois.
    int usarMagia = 0;
    if (g_ataque == 2) {
        usarMagia = 1;
    } else if (g_ataque == 0) {
        if (*reinterpret_cast<const int*>(0x63A29C) == 2) {
            usarMagia = 1;
        } else {
            BYTE* bloco = *reinterpret_cast<BYTE**>(kPlayerBlock);
            if (bloco != nullptr && *(bloco + kMyMobOff + 0x14) == 1) {
                usarMagia = 1; // Foema
            }
        }
    }

    if (usarMagia && LancaMagia(scene, g_travado)) {
        return;
    }
    // Sem magia, ou magia recusada (fora de alcance): aproxima e bate, que é o
    // que o clique do jogo faz. Para o mago, chegar ao alcance já faz a magia
    // sair no tique seguinte.
    reinterpret_cast<AttackFn>(kAttack)(self, g_travado, 1);
}

// ---- teclas e tique ------------------------------------------------------
bool Apertou(int vk, bool& antes) {
    bool agora = (GetAsyncKeyState(vk) & 0x8000) != 0;
    bool novo = agora && !antes;
    antes = agora;
    return novo;
}

// Grava o alvo.txt com os valores atuais: e o que a engrenagem do painel usa,
// para a escolha valer na proxima sessao sem voce editar arquivo.
void SalvaConfig() {
    char caminho[MAX_PATH];
    sprintf_s(caminho, "%salvo.txt", g_dir);
    FILE* f = nullptr;
    if (fopen_s(&f, caminho, "w") != 0 || f == nullptr) {
        return;
    }
    fprintf(f, "# Sistema de alvos. A engrenagem do painel escreve aqui.\n");
    fprintf(f, "# modo = ambos | monstros | players\n");
    fprintf(f, "# ataque = auto | fisico | magico\n");
    fprintf(f, "# alcance = ate quantas casas procurar (3 a 15)\n");
    fprintf(f, "modo=%s\n", g_modo == 1 ? "monstros" : (g_modo == 2 ? "players" : "ambos"));
    fprintf(f, "ataque=%s\n", g_ataque == 1 ? "fisico" : (g_ataque == 2 ? "magico" : "auto"));
    fprintf(f, "max=%d\n", g_max);
    fprintf(f, "alcance=%d\n", g_alcance);
    fprintf(f, "ocultar_aliados=%d\n", g_ocultarAliados);
    fprintf(f, "mensagens=%d\n", g_mensagens);
    fprintf(f, "descobrir=%d\n", g_descobrir);
    fprintf(f, "tecla_scan=%02X\n", g_teclaScan);
    fprintf(f, "tecla_soltar=%02X\n", g_teclaSoltar);
    fclose(f);
}

void LeConfig() {
    char caminho[MAX_PATH];
    sprintf_s(caminho, "%salvo.txt", g_dir);
    FILE* f = nullptr;
    if (fopen_s(&f, caminho, "r") != 0 || f == nullptr) {
        if (fopen_s(&f, caminho, "w") == 0 && f != nullptr) {
            fprintf(f, "# Sistema de alvos. Reabra o jogo depois de mudar.\n");
            fprintf(f, "# modo = ambos | monstros | players\n");
            fprintf(f, "# max  = quantos alvos na lista (ate 10)\n");
            fprintf(f, "# tecla_scan/tecla_soltar = codigo da tecla em hex (Virtual-Key)\n");
            fprintf(f, "#   vazio no scan = apostrofo (aceita DB e DE, conforme o teclado)\n");
            fprintf(f, "#   A0 = Shift esquerdo, 09 = Tab, 1B = Esc, 60..69 = teclado numerico\n");
            fprintf(f, "modo=ambos\nmax=10\ntecla_scan=\ntecla_soltar=A0\n");
            fclose(f);
        }
        return;
    }
    char linha[128];
    while (fgets(linha, sizeof(linha), f) != nullptr) {
        if (linha[0] == '#' || linha[0] == ';') {
            continue;
        }
        char chave[32] = {0}, valor[32] = {0};
        if (sscanf_s(linha, "%31[^=]=%31s", chave, static_cast<unsigned>(sizeof(chave)),
                     valor, static_cast<unsigned>(sizeof(valor))) != 2) {
            continue;
        }
        if (_stricmp(chave, "modo") == 0) {
            if (_stricmp(valor, "monstros") == 0) {
                g_modo = 1;
            } else if (_stricmp(valor, "players") == 0) {
                g_modo = 2;
            } else {
                g_modo = 0;
            }
        } else if (_stricmp(chave, "max") == 0) {
            int v = atoi(valor);
            if (v >= 1 && v <= kMaxLista) {
                g_max = v;
            }
        } else if (_stricmp(chave, "tecla_scan") == 0) {
            int v = static_cast<int>(strtol(valor, nullptr, 16));
            if (v > 0 && v < 256) {
                g_teclaScan = v;
            }
        } else if (_stricmp(chave, "tecla_soltar") == 0) {
            int v = static_cast<int>(strtol(valor, nullptr, 16));
            if (v > 0 && v < 256) {
                g_teclaSoltar = v;
            }
        } else if (_stricmp(chave, "descobrir") == 0) {
            g_descobrir = atoi(valor);
        } else if (_stricmp(chave, "ocultar_aliados") == 0) {
            g_ocultarAliados = atoi(valor);
        } else if (_stricmp(chave, "ataque") == 0) {
            if (_stricmp(valor, "fisico") == 0) {
                g_ataque = 1;
            } else if (_stricmp(valor, "magico") == 0) {
                g_ataque = 2;
            } else {
                g_ataque = 0;
            }
        } else if (_stricmp(chave, "alcance") == 0) {
            int v = atoi(valor);
            if (v >= 3 && v <= 15) {
                g_alcance = v;
            }
        } else if (_stricmp(chave, "mensagens") == 0) {
            g_mensagens = atoi(valor);
        }
    }
    fclose(f);
}

} // namespace

extern "C" void __cdecl AlvosCapturaDispatcher(void* self, int arg1);
extern "C" int __cdecl LojaRedeRecebe(const unsigned char* pacote);

namespace {

// Desvio no começo do tratador de pacotes. Anota `this` e o primeiro argumento
// de uma chamada de verdade — é o que permite entregar um pacote nosso ao
// cliente depois — e mostra o pacote que chegou à Loja do Servidor, que fala com
// o tmServer por dois tipos que o cliente original não conhece. Nada no fluxo é
// alterado: o pacote segue para o cliente como sempre.
__declspec(naked) void DispatcherHook() {
    __asm {
        pushad
        pushfd
        mov eax, dword ptr [esp + 0x2C] // o pacote (3o argumento)
        push eax
        call LojaRedeRecebe
        add esp, 4
        mov eax, dword ptr [esp + 0x28] // arg1 (pushad 32 + pushfd 4 + retorno 4)
        push eax
        push ecx                        // this
        call AlvosCapturaDispatcher
        add esp, 8
        popfd
        popad
        push ebp
        mov ebp, esp
        sub esp, 0x0C
        push 0x495ACC // kDispatcherBack
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
        bool ok = InstalaJmp(kDispatcher, kDispatcherBytes, sizeof(kDispatcherBytes), DispatcherHook);
        Log("=== alvos: desvio 495AC6=%d", ok ? 1 : 0);
    }
};

Installer g_alvos;

} // namespace

// Chamado pela janela sobreposta (overlay.cpp) quando você clica numa linha.
// Só marca a escolha; quem age é o tique, na thread do jogo — chamar as funções
// do cliente de outra thread seria pedir travamento.
volatile int g_painelEscolha = -1;

void AlvosCliqueNaLinha(int indice) {
    g_painelEscolha = indice;
}

// O X do cabeçalho. O alvo travado continua travado: fechar é só esconder a
// lista, não desistir da caçada.
void AlvosFechaPainel() {
    g_painelAberto = false;
}

// --- opções, para a engrenagem da janela sobreposta ------------------------
// 0 = mostrar monstros, 1 = mostrar jogadores, 2 = ocultar aliados, 3 = alcance.
int AlvosOpcao(int qual) {
    switch (qual) {
        case 0: return g_modo != 2; // modo 2 = só players
        case 1: return g_modo != 1; // modo 1 = só monstros
        case 2: return g_ocultarAliados;
        case 3: return g_alcance;
        case 4: return g_ataque; // 0 auto, 1 fisico, 2 magico
        default: return 0;
    }
}

void AlvosSetOpcao(int qual, int valor) {
    switch (qual) {
        case 0: // monstros
            if (valor) {
                g_modo = (g_modo == 2) ? 0 : g_modo;
            } else {
                g_modo = 2; // só players
            }
            break;
        case 1: // jogadores
            if (valor) {
                g_modo = (g_modo == 1) ? 0 : g_modo;
            } else {
                g_modo = 1; // só monstros
            }
            break;
        case 2:
            g_ocultarAliados = valor ? 1 : 0;
            break;
        case 3:
            if (valor >= 3 && valor <= 15) {
                g_alcance = valor;
            }
            break;
        case 4:
            if (valor >= 0 && valor <= 2) {
                g_ataque = valor;
            }
            break;
        default:
            return;
    }
    SalvaConfig();
    Escanear();
    PublicaNoPainel();
}

// Chamado a cada quadro pelo desvio do macro (macromago.cpp).
void AlvosTick() {
    static bool tab = false, esc = false, num[10] = {false};
    static bool iniciado = false;
    if (!iniciado) {
        iniciado = true;
        GetModuleFileNameA(nullptr, g_dir, MAX_PATH);
        char* barra = strrchr(g_dir, '\\');
        if (barra != nullptr) {
            *(barra + 1) = '\0';
        }
        LeConfig();
        Log("=== alvos: modo=%d max=%d", g_modo, g_max);
    }

    // O desenho do painel precisa da interface já criada, por isso a instalação
    // fica aqui e não no carregamento do DLL.
    PainelInstala();
    PainelNovoQuadro();

    // Clique vindo da janela sobreposta: age aqui, na thread do jogo.
    if (g_painelEscolha >= 0) {
        int escolha = g_painelEscolha;
        g_painelEscolha = -1;
        Travar(escolha);
    }

    // Scan: a tecla escolhida em alvo.txt ou, por padrão, o apóstrofo — que cai
    // em 0xDB ou 0xDE conforme o layout, então os dois valem.
    bool scanAgora = false;
    if (g_teclaScan != 0) {
        scanAgora = Apertou(g_teclaScan, tab);
    } else {
        static bool db = false, de = false;
        bool a = Apertou(VK_OEM_4, db);
        bool b = Apertou(VK_OEM_7, de);
        scanAgora = a || b;
    }
    if (scanAgora) {
        g_painelAberto = true;
        Escanear();
        PublicaNoPainel();
        MostrarLista();
        Log("scan: %d alvos", g_n);
    }
    if (Apertou(g_teclaSoltar, esc)) {
        Soltar();
    }

    // Descoberta de tecla: anota no alvos.log o código de toda tecla apertada,
    // para acertar o apóstrofo sem chutar (o código dele muda com o layout).
    // Desligue com descobrir=0 em alvo.txt quando não precisar mais.
    if (g_descobrir) {
        static bool estado[256] = {false};
        for (int vk = 0x08; vk < 0xFF; ++vk) {
            if (vk == VK_LBUTTON || vk == VK_RBUTTON || vk == VK_MBUTTON) {
                continue;
            }
            bool agora = (GetAsyncKeyState(vk) & 0x8000) != 0;
            if (agora && !estado[vk]) {
                Log("tecla apertada: %02X", vk);
            }
            estado[vk] = agora;
        }
    }
    for (int i = 0; i < 10; ++i) {
        int vk = (i == 9) ? '0' : ('1' + i);
        if (Apertou(vk, num[i])) {
            Travar(i);
        }
    }
    Perseguir();
}

// Captura o `this` e o primeiro argumento de uma chamada real do tratador de
// pacotes — é o que permite entregar um pacote nosso ao cliente depois.
extern "C" void __cdecl AlvosCapturaDispatcher(void* self, int arg1) {
    if (!g_dispPronto) {
        g_dispThis = self;
        g_dispArg1 = arg1;
        g_dispPronto = true;
    }
}
