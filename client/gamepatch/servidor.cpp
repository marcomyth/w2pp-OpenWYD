// Para onde o cliente se conecta, decidido por um arquivo.
//
// A lista de servidores do WYD vive num serverlist.bin cifrado, que só o editor
// deles escreve — o que torna impossível apontar o cliente para um servidor
// novo de dentro de um script. Este módulo resolve isso por fora: desvia o
// connect() do ws2_32 na tabela de importação do WYD.exe e troca o destino.
//
// FICA INERTE sem o arquivo. Só age se existir um servidor.txt ao lado do
// executável, com uma linha:
//
//     altaria.proxy.rlwy.net:55693
//
// É ferramenta de teste: serve para apontar o cliente a um ambiente de
// homologação sem gerar uma lista nova. Sem o arquivo, nada muda.

#include <winsock2.h>
#include <ws2tcpip.h>
#include <windows.h>

#include <cstdio>
#include <cstring>

void CamadaLog(const char* texto);

namespace {

typedef int(WSAAPI* ConnectFn)(SOCKET, const sockaddr*, int);

ConnectFn g_connectOrig = nullptr;
sockaddr_in g_destino;
bool g_temDestino = false;
bool g_avisou = false;

// Lê o servidor.txt: uma linha "host:porta". Sem arquivo, o módulo não faz nada.
bool LeDestino() {
    char caminho[MAX_PATH];
    GetModuleFileNameA(nullptr, caminho, MAX_PATH);
    char* barra = strrchr(caminho, 92);
    if (barra == nullptr) {
        return false;
    }
    strcpy_s(barra + 1, MAX_PATH - (barra + 1 - caminho), "servidor.txt");
    FILE* f = nullptr;
    if (fopen_s(&f, caminho, "r") != 0 || f == nullptr) {
        return false;
    }
    char linha[256] = {0};
    const bool leu = fgets(linha, sizeof(linha), f) != nullptr;
    fclose(f);
    if (!leu) {
        return false;
    }
    char* fim = linha + strlen(linha);
    while (fim > linha && (fim[-1] == 10 || fim[-1] == 13 || fim[-1] == 32)) {
        *--fim = 0;
    }
    char* doisPontos = strrchr(linha, ':');
    if (doisPontos == nullptr) {
        return false;
    }
    *doisPontos = 0;
    const int porta = atoi(doisPontos + 1);
    if (porta <= 0 || porta > 65535) {
        return false;
    }

    addrinfo pedido;
    memset(&pedido, 0, sizeof(pedido));
    pedido.ai_family = AF_INET;
    pedido.ai_socktype = SOCK_STREAM;
    addrinfo* achado = nullptr;
    if (getaddrinfo(linha, nullptr, &pedido, &achado) != 0 || achado == nullptr) {
        char buf[160];
        sprintf_s(buf, "=== servidor: nao resolvi %s", linha);
        CamadaLog(buf);
        return false;
    }
    memcpy(&g_destino, achado->ai_addr, sizeof(sockaddr_in));
    g_destino.sin_port = htons(static_cast<u_short>(porta));
    freeaddrinfo(achado);

    char texto[64];
    inet_ntop(AF_INET, &g_destino.sin_addr, texto, sizeof(texto));
    char buf[200];
    sprintf_s(buf, "=== servidor: o cliente vai para %s (%s:%d)", linha, texto, porta);
    CamadaLog(buf);
    return true;
}

int WSAAPI MeuConnect(SOCKET s, const sockaddr* nome, int tam) {
    if (g_temDestino && nome != nullptr && nome->sa_family == AF_INET) {
        sockaddr_in troca = g_destino;
        if (!g_avisou) {
            g_avisou = true;
            const sockaddr_in* antes = reinterpret_cast<const sockaddr_in*>(nome);
            char de[64];
            char para[64];
            inet_ntop(AF_INET, &antes->sin_addr, de, sizeof(de));
            inet_ntop(AF_INET, &troca.sin_addr, para, sizeof(para));
            char buf[200];
            sprintf_s(buf, "=== servidor: desviando %s:%d para %s:%d", de, ntohs(antes->sin_port),
                      para, ntohs(troca.sin_port));
            CamadaLog(buf);
        }
        return g_connectOrig(s, reinterpret_cast<sockaddr*>(&troca), sizeof(troca));
    }
    return g_connectOrig(s, nome, tam);
}

// Troca a entrada de connect na tabela de importação do executável.
bool DesviaConnect() {
    BYTE* base = reinterpret_cast<BYTE*>(GetModuleHandleA(nullptr));
    const IMAGE_DOS_HEADER* dos = reinterpret_cast<IMAGE_DOS_HEADER*>(base);
    const IMAGE_NT_HEADERS* nt = reinterpret_cast<IMAGE_NT_HEADERS*>(base + dos->e_lfanew);
    const IMAGE_DATA_DIRECTORY& dir = nt->OptionalHeader.DataDirectory[IMAGE_DIRECTORY_ENTRY_IMPORT];
    if (dir.VirtualAddress == 0) {
        return false;
    }
    const IMAGE_IMPORT_DESCRIPTOR* imp =
        reinterpret_cast<IMAGE_IMPORT_DESCRIPTOR*>(base + dir.VirtualAddress);
    for (; imp->Name != 0; ++imp) {
        const char* dll = reinterpret_cast<const char*>(base + imp->Name);
        if (_stricmp(dll, "ws2_32.dll") != 0 && _stricmp(dll, "wsock32.dll") != 0) {
            continue;
        }
        const IMAGE_THUNK_DATA* nomes = reinterpret_cast<IMAGE_THUNK_DATA*>(
            base + (imp->OriginalFirstThunk != 0 ? imp->OriginalFirstThunk : imp->FirstThunk));
        IMAGE_THUNK_DATA* enderecos = reinterpret_cast<IMAGE_THUNK_DATA*>(base + imp->FirstThunk);
        for (; nomes->u1.AddressOfData != 0; ++nomes, ++enderecos) {
            bool eConnect = false;
            if (IMAGE_SNAP_BY_ORDINAL(nomes->u1.Ordinal)) {
                // wsock32 exporta connect pelo ordinal 4.
                eConnect = IMAGE_ORDINAL(nomes->u1.Ordinal) == 4;
            } else {
                const IMAGE_IMPORT_BY_NAME* n =
                    reinterpret_cast<IMAGE_IMPORT_BY_NAME*>(base + nomes->u1.AddressOfData);
                eConnect = strcmp(reinterpret_cast<const char*>(n->Name), "connect") == 0;
            }
            if (!eConnect) {
                continue;
            }
            DWORD antes = 0;
            if (!VirtualProtect(enderecos, sizeof(IMAGE_THUNK_DATA), PAGE_READWRITE, &antes)) {
                return false;
            }
            g_connectOrig = reinterpret_cast<ConnectFn>(enderecos->u1.Function);
            enderecos->u1.Function =
                reinterpret_cast<ULONG_PTR>(reinterpret_cast<void*>(&MeuConnect));
            VirtualProtect(enderecos, sizeof(IMAGE_THUNK_DATA), antes, &antes);
            return true;
        }
    }
    return false;
}

DWORD WINAPI Thread(LPVOID) {
    // O winsock precisa estar de pé para resolver o nome; o cliente o inicia
    // cedo, mas não no ponto de entrada.
    Sleep(1500);
    WSADATA wsa;
    WSAStartup(MAKEWORD(2, 2), &wsa);
    if (!LeDestino()) {
        return 0;   // sem servidor.txt, este módulo não existe
    }
    g_temDestino = DesviaConnect();
    CamadaLog(g_temDestino ? "=== servidor: connect desviado"
                           : "=== servidor: nao achei connect na importacao");
    return 0;
}

struct Installer {
    Installer() { CreateThread(nullptr, 0, Thread, nullptr, 0, nullptr); }
};

Installer g_instalador;

} // namespace
