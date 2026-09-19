// A conversa da Loja do Servidor com o tmServer.
//
// São dois pacotes nossos, fora da faixa que o WYD.exe usa (protocolo em
// tmserver/internal/protocol/lojaservidor.go): 0x0F01 pede uma página da
// vitrine e 0x0F02 traz a página. O cliente original não conhece nenhum dos
// dois — quem fala é este DLL.

#ifndef LOJAREDE_H
#define LOJAREDE_H

// Uma oferta como o servidor manda. Vendedor e Slot são o que a compra precisa:
// o id da barraca e a posição do item dentro dela.
struct LojaOferta {
    int vendedor;
    char nome[17];
    short indice;
    signed char slot;
    unsigned char refino;
    unsigned char qtd;
    unsigned char moeda;   // 0 ouro, 1 cash, 2 RMT
    unsigned char perto;   // 1 quando dá para comprar daqui
    int preco;
};

// Pede uma página ao servidor. Filtro segue o menu do painel.
void LojaRedePede(int pagina, int filtro);

// Compra uma oferta. O servidor confere tudo de novo — preço, item, alcance e
// moeda —, então isto é só o apontar do dedo.
void LojaRedeCompra(int vendedor, int slot, int moeda);

// Troca a moeda de um item da MINHA barraca.
void LojaRedeMoeda(int slot, int moeda);

// --- montar a barraca -------------------------------------------------------

// Um item do cofre, como o servidor o descreve.
struct LojaItemCofre {
    short slot;
    short indice;
    unsigned char refino;
    unsigned char qtd;
};

// Uma prateleira da barraca a montar.
struct LojaPrateleira {
    signed char cargoPos;   // -1 = vazia
    unsigned char moeda;
    int preco;
};

// Pede o cofre ao servidor (o painel precisa dele para montar a barraca).
void LojaRedePedeCofre();
int LojaRedeCofreQtd();
const LojaItemCofre* LojaRedeCofreItem(int i);

// Manda montar a barraca. Sem título: o servidor usa o nome do personagem.
void LojaRedeAbre(const char* titulo, const LojaPrateleira* prateleiras, int quantas);

// 1 quando o servidor avisou que o jogador clicou numa barraca na cidade e a
// vitrine deve abrir. Consome o aviso.
int LojaRedePedidoMercado();

// Id da barraca que subiu, 0 enquanto nenhuma subiu nesta sessão.
int LojaRedeBarracaAberta();

// O que veio na última resposta.
bool LojaRedeRespondeu();
int LojaRedeTotal();
int LojaRedePaginas();
int LojaRedePagina();
int LojaRedeQuantas();

// Saldos de quem pediu, como o servidor mandou. Cash e RMT ainda voltam zerados:
// esses saldos vivem na conta, do lado do site, e o tmServer ainda nao os le.
int LojaRedeSaldo(int moeda);
const LojaOferta* LojaRedeOferta(int i);

// Chamado pelo desvio do tratador de pacotes (alvos.cpp), para todo pacote que
// chega: devolve 1 quando o pacote era nosso.
extern "C" int __cdecl LojaRedeRecebe(const unsigned char* pacote);

#endif
