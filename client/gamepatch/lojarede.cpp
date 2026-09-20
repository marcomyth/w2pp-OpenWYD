// A conversa da Loja do Servidor com o tmServer. Ver lojarede.h.
//
// O envio usa a própria função do cliente (0x54DA23, __cdecl(char*, int)), a
// mesma que ele chama para mandar qualquer pacote: ela cuida da embaralhada e do
// checksum, então aqui só montamos o cabeçalho de 12 bytes e o corpo.
//
// A recepção pega carona no desvio que o sistema de alvos já tem no tratador de
// pacotes (0x495AC6): todo pacote que chega passa por lá antes do cliente, e o
// que for nosso é copiado e devolvido como "era nosso".

#include "lojarede.h"

#include <windows.h>

#include <cstdio>
#include <cstring>

void CamadaLog(const char* texto);

namespace {

typedef void(__cdecl* EnviaFn)(char*, int);
constexpr DWORD kEnviaPacote = 0x54DA23;

constexpr WORD kMsgPede = 0x0F01;
constexpr WORD kMsgLista = 0x0F02;
constexpr WORD kMsgMoeda = 0x0F03;
constexpr WORD kMsgCompra = 0x0F04;
constexpr WORD kMsgCofre = 0x0F05;
constexpr WORD kMsgCofreLista = 0x0F06;
constexpr WORD kMsgAbrir = 0x0F07;
constexpr WORD kMsgAbriu = 0x0F08;
constexpr WORD kMsgMercado = 0x0F09;
constexpr WORD kMsgFecha = 0x0F0A;

constexpr int kMaxCofre = 128;
constexpr int kMaxPrateleiras = 12;
constexpr int kTamItemCofre = 8;
constexpr int kTamPrateleira = 8;
constexpr int kTamTitulo = 24;

constexpr int kCabecalho = 12;
constexpr int kPorPagina = 32;      // igual ao servidor
constexpr int kTamOferta = 32;
constexpr int kCabecalhoLista = 8 + 12;   // contagem + os tres saldos

LojaOferta g_ofertas[kPorPagina];
int g_total = 0;
int g_paginas = 1;
int g_pagina = 0;
int g_quantas = 0;
int g_saldo[3] = {0, 0, 0};
LojaItemCofre g_cofre[kMaxCofre];
int g_cofreQtd = 0;
int g_barracaAberta = 0;
int g_pedidoMercado = 0;
bool g_respondeu = false;

} // namespace

// Monta o cabecalho de 12 bytes e manda pela funcao do proprio cliente, que
// cuida da embaralhada e do checksum.
void Envia(WORD tipo, const void* corpo, int tamCorpo) {
    // 256 e folga para o maior que temos: o pedido de abrir a barraca leva 24
    // bytes de titulo mais 12 prateleiras de 8, e com o cabecalho da 132. Com os
    // 64 de antes ele nao cabia - e a saida era um return mudo, que gastou uma
    // rodada de teste inteira: o painel dizia que tinha mandado e o servidor
    // nunca via o pacote.
    BYTE pacote[256];
    const int total = kCabecalho + tamCorpo;
    if (total > static_cast<int>(sizeof(pacote))) {
        char buf[120];
        sprintf_s(buf, "=== loja: pacote %04X grande demais (%d bytes), nao enviado", tipo, total);
        CamadaLog(buf);
        return;
    }
    memset(pacote, 0, sizeof(pacote));
    *reinterpret_cast<WORD*>(pacote + 0) = static_cast<WORD>(total);   // Size
    *reinterpret_cast<WORD*>(pacote + 4) = tipo;                       // Type
    *reinterpret_cast<WORD*>(pacote + 6) = 0;                          // ID
    *reinterpret_cast<DWORD*>(pacote + 8) = GetTickCount();            // ClientTick
    if (tamCorpo > 0) {
        memcpy(pacote + kCabecalho, corpo, tamCorpo);
    }
    reinterpret_cast<EnviaFn>(kEnviaPacote)(reinterpret_cast<char*>(pacote), total);
}

void LojaRedePede(int pagina, int filtro) {
    short corpo[2] = {static_cast<short>(pagina), static_cast<short>(filtro)};
    Envia(kMsgPede, corpo, sizeof(corpo));
}

void LojaRedeCompra(int vendedor, int slot, int moeda) {
    BYTE corpo[8];
    memset(corpo, 0, sizeof(corpo));
    *reinterpret_cast<int*>(corpo + 0) = vendedor;
    corpo[4] = static_cast<BYTE>(slot);
    corpo[5] = static_cast<BYTE>(moeda);
    Envia(kMsgCompra, corpo, sizeof(corpo));
}

void LojaRedeMoeda(int slot, int moeda) {
    BYTE corpo[2] = {static_cast<BYTE>(slot), static_cast<BYTE>(moeda)};
    Envia(kMsgMoeda, corpo, sizeof(corpo));
}

void LojaRedePedeCofre() {
    Envia(kMsgCofre, nullptr, 0);
}

int LojaRedeCofreQtd() {
    return g_cofreQtd;
}

const LojaItemCofre* LojaRedeCofreItem(int i) {
    return (i >= 0 && i < g_cofreQtd) ? &g_cofre[i] : nullptr;
}

int LojaRedeBarracaAberta() {
    return g_barracaAberta;
}

void LojaRedeAbre(const char* titulo, const LojaPrateleira* prateleiras, int quantas) {
    BYTE corpo[kTamTitulo + kMaxPrateleiras * kTamPrateleira];
    memset(corpo, 0, sizeof(corpo));
    // Titulo vazio: o servidor poe o nome do personagem.
    if (titulo != nullptr) {
        size_t n = strlen(titulo);
        if (n > kTamTitulo - 1) {
            n = kTamTitulo - 1;
        }
        memcpy(corpo, titulo, n);
    }
    for (int i = 0; i < kMaxPrateleiras; ++i) {
        BYTE* p = corpo + kTamTitulo + i * kTamPrateleira;
        const bool tem = (i < quantas && prateleiras[i].cargoPos >= 0);
        p[0] = tem ? static_cast<BYTE>(prateleiras[i].cargoPos) : 0xFF;   // -1
        p[1] = tem ? prateleiras[i].moeda : 0;
        *reinterpret_cast<int*>(p + 4) = tem ? prateleiras[i].preco : 0;
    }
    Envia(kMsgAbrir, corpo, sizeof(corpo));
}

void LojaRedeFecha() {
    Envia(kMsgFecha, nullptr, 0);
}

bool LojaRedeRespondeu() {
    return g_respondeu;
}

int LojaRedeTotal() {
    return g_total;
}

int LojaRedePaginas() {
    return g_paginas;
}

int LojaRedePagina() {
    return g_pagina;
}

int LojaRedeQuantas() {
    return g_quantas;
}

int LojaRedeSaldo(int moeda) {
    return (moeda >= 0 && moeda < 3) ? g_saldo[moeda] : 0;
}

const LojaOferta* LojaRedeOferta(int i) {
    return (i >= 0 && i < g_quantas) ? &g_ofertas[i] : nullptr;
}

extern "C" int __cdecl LojaRedeRecebe(const unsigned char* pacote) {
    if (pacote == nullptr) {
        return 0;
    }
    const WORD tipo = *reinterpret_cast<const WORD*>(pacote + 4);
    const unsigned char* corpo = pacote + kCabecalho;
    if (tipo == kMsgCofreLista) {
        g_cofreQtd = *reinterpret_cast<const short*>(corpo + 0);
        if (g_cofreQtd < 0) {
            g_cofreQtd = 0;
        }
        if (g_cofreQtd > kMaxCofre) {
            g_cofreQtd = kMaxCofre;
        }
        for (int i = 0; i < g_cofreQtd; ++i) {
            const unsigned char* o = corpo + 4 + i * kTamItemCofre;
            g_cofre[i].slot = *reinterpret_cast<const short*>(o + 0);
            g_cofre[i].indice = *reinterpret_cast<const short*>(o + 2);
            g_cofre[i].refino = o[4];
            g_cofre[i].qtd = o[5];
        }
        return 1;
    }
    if (tipo == kMsgMercado) {
        // O jogador clicou numa barraca na cidade. A janela antiga do cliente
        // nao existe mais; o gesto leva a vitrine.
        g_pedidoMercado = 1;
        CamadaLog("=== loja: clique numa barraca, indo para o mercado");
        return 1;
    }
    if (tipo == kMsgAbriu) {
        g_barracaAberta = *reinterpret_cast<const int*>(corpo + 0);
        CamadaLog("=== loja: a barraca subiu");
        return 1;
    }
    if (tipo != kMsgLista) {
        return 0;
    }
    g_pagina = *reinterpret_cast<const short*>(corpo + 0);
    g_paginas = *reinterpret_cast<const short*>(corpo + 2);
    g_total = *reinterpret_cast<const short*>(corpo + 4);
    g_quantas = *reinterpret_cast<const short*>(corpo + 6);
    if (g_quantas < 0) {
        g_quantas = 0;
    }
    if (g_quantas > kPorPagina) {
        g_quantas = kPorPagina;
    }
    g_saldo[0] = *reinterpret_cast<const int*>(corpo + 8);
    g_saldo[1] = *reinterpret_cast<const int*>(corpo + 12);
    g_saldo[2] = *reinterpret_cast<const int*>(corpo + 16);
    for (int i = 0; i < g_quantas; ++i) {
        const unsigned char* o = corpo + kCabecalhoLista + i * kTamOferta;
        LojaOferta& dest = g_ofertas[i];
        dest.vendedor = *reinterpret_cast<const int*>(o + 0);
        memcpy(dest.nome, o + 4, 16);
        dest.nome[16] = 0;
        dest.indice = *reinterpret_cast<const short*>(o + 20);
        dest.slot = static_cast<signed char>(o[22]);
        dest.refino = o[23];
        dest.qtd = o[24];
        dest.moeda = o[25];
        dest.perto = o[26];
        dest.preco = *reinterpret_cast<const int*>(o + 28);
    }
    if (!g_respondeu) {
        g_respondeu = true;
        CamadaLog("=== loja: primeira lista recebida do servidor");
    }
    return 1;
}

// O servidor pediu que a vitrine abrisse - alguem clicou numa barraca. A
// resposta e consumida uma vez so.
int LojaRedePedidoMercado() {
    const int tinha = g_pedidoMercado;
    g_pedidoMercado = 0;
    return tinha;
}
