// Camadas desenhadas dentro do quadro do jogo.
//
// O painel de alvos nasceu sozinho e falava direto com o modulo de D3D. Com a
// loja do servidor passaram a existir tres coisas para desenhar - painel, icone
// e janela da loja -, entao o contrato virou este: cada modulo registra uma
// camada, o d3dpainel desenha todas em ordem e o camadas.cpp entrega o clique a
// quem estiver por cima.
//
// Toda camada pinta em GDI num DIB proprio, de 32 bits e de cima para baixo, que
// e o formato que a textura A8R8G8B8 espera.

#ifndef CAMADAS_H
#define CAMADAS_H

#include <windows.h>

struct Camada {
    // Quem tem ordem maior fica por cima, e recebe o clique primeiro.
    int ordem;

    // 1 quando a camada deve aparecer neste quadro.
    int (*visivel)();

    // Onde e de que tamanho, em coordenadas do backbuffer. Chamada antes de
    // pixels, e tambem no teste do mouse - precisa ser barata.
    void (*medida)(int telaL, int telaA, int* x, int* y, int* largura, int* altura);

    // O conteudo, no tamanho que medida acabou de dizer. versao muda quando a
    // pintura muda, e so entao a textura e reenviada.
    const void* (*pixels)(int* versao);

    // Clique dentro da camada, em coordenadas relativas ao canto dela.
    void (*clique)(int x, int y);
};

void CamadaRegistra(const Camada* c);
int CamadaTotal();
const Camada* CamadaEm(int i);   // ja em ordem de desenho, de baixo para cima

// A janela do jogo, achada pelo processo. Usada para converter o cursor.
HWND CamadaJanelaDoJogo();

// O cursor em coordenadas do backbuffer; 0 quando nao da para saber. Serve para
// uma camada acender quando o mouse passa por cima dela.
int CamadaCursor(int* x, int* y);
int CamadaTelaL();
int CamadaTelaA();

// --- amostras do quadro anterior -------------------------------------------
//
// Ler um pixel do backbuffer so e permitido FORA da cena, entao quem le e o
// modulo de D3D, logo depois do EndScene, e guarda aqui. Uma camada registra os
// pontos que lhe interessam e consulta a cor lida por ultimo. E assim que a loja
// descobre se a barra de icones do jogo esta aberta.
constexpr int kMaxAmostras = 4;

void AmostraPede(int id, int x, int y);
DWORD AmostraCor(int id);

// Um clique acabou de acontecer: a proxima leitura nao espera o intervalo. E
// assim que o icone acompanha o botao MENU sem atraso visivel.
void AmostraUrgente();

// Usados pelo modulo de D3D.
int AmostraPonto(int id, int* x, int* y);
void AmostraGuarda(int id, DWORD cor);
int AmostraConsomeUrgencia();

// Log comum dos modulos do cliente, no alvos.log ao lado do executavel.
void CamadaLog(const char* texto);

extern "C" {
// O modulo de D3D avisa o tamanho do backbuffer a cada quadro.
void __cdecl CamadaTela(int largura, int altura);
}

#endif
