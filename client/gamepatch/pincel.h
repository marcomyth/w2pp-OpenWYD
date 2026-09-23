// Paleta e pinceis do visual do jogo, compartilhados pelas camadas.
//
// As cores sairam do CSS do layout que o Marco desenhou
// (stitch_wyd_enemy_target_ui): moldura escura com borda dupla e cantos
// dourados, cabecalho em degrade marrom, e botoes com tampa metalica nas pontas.
// O painel de alvos nasceu com tudo isto dentro dele; quando a loja do servidor
// chegou, a paleta virou este arquivo para as duas falarem a mesma lingua.

#ifndef PINCEL_H
#define PINCEL_H

#include <windows.h>

const COLORREF kFundoTopo = RGB(22, 16, 12);
const COLORREF kFundoBaixo = RGB(10, 8, 6);
const COLORREF kBordaInterna = RGB(90, 73, 51);   // #5a4933
const COLORREF kBorda1 = RGB(18, 14, 10);         // #120e0a
const COLORREF kBorda2 = RGB(74, 62, 46);         // #4a3e2e
const COLORREF kBorda3 = RGB(21, 16, 10);         // #15100a
const COLORREF kCanto = RGB(223, 199, 143);       // #dfc78f
const COLORREF kCabTopo = RGB(66, 25, 0);         // #421900
const COLORREF kCabBaixo = RGB(18, 5, 0);         // #120500
const COLORREF kCabBorda = RGB(162, 120, 69);     // #a27845
const COLORREF kLosango = RGB(236, 212, 162);     // #ecd4a2
const COLORREF kBtnTopo = RGB(107, 46, 5);        // #6b2e05
const COLORREF kBtnBaixo = RGB(32, 9, 0);         // #200900
const COLORREF kBtnBordaTopo = RGB(201, 147, 78); // #c9934e
const COLORREF kBtnBordaBaixo = RGB(26, 8, 0);    // #1a0800
const COLORREF kBtnAtivoTopo = RGB(138, 60, 9);   // #8a3c09
const COLORREF kBtnAtivoBaixo = RGB(41, 13, 2);   // #290d02
const COLORREF kTampaClara = RGB(211, 196, 158);  // #d3c49e
const COLORREF kTampaEscura = RGB(70, 59, 39);    // #463b27
const COLORREF kTampaBorda = RGB(27, 21, 12);     // #1b150c
const COLORREF kTexto = RGB(255, 232, 179);       // #ffe8b3
const COLORREF kTextoAtivo = RGB(255, 245, 160);
const COLORREF kTextoFraco = RGB(150, 136, 110);
const COLORREF kMarcaOn = RGB(126, 217, 139);
const COLORREF kMarcaOff = RGB(90, 82, 70);

// Largura da tampa metalica das pontas de um botao.
constexpr int kTampa = 9;

void Barra(HDC hdc, int x, int y, int l, int a, COLORREF cor);
void Degrade(HDC hdc, int x, int y, int l, int a, COLORREF topo, COLORREF baixo);
void Contorno(HDC hdc, int x, int y, int l, int a, COLORREF cor);
void Losango(HDC hdc, int cx, int cy, int r, COLORREF cor);
void TampaMetal(HDC hdc, int x, int y, int a, int larg);
void LinhaBotao(HDC hdc, int x, int y, int l, int a, bool ativo);


// Um DIB de 32 bits, de cima para baixo, que e o formato da textura A8R8G8B8.
// A camada pinta nele e o modulo de D3D o sobe como textura.
struct Tela {
    HDC dc;
    HBITMAP bmp;
    HGDIOBJ bmpAntigo;
    void* pixels;
    int l;
    int a;
};

bool TelaGarante(Tela* t, int l, int a);

// Moldura completa: borda tripla, fundo em degrade e os cantos dourados. Recebe
// a Tela, e nao um HDC, porque o fundo e pintado direto nos pixels - ver o
// porque em pincel.cpp.
void Moldura(Tela* t);

// Fecha a pintura: o GDI nunca escreve o canal alfa, entao ele e posto aqui.
void TelaFecha(Tela* t, BYTE alfa);

// Idem, mas o que tiver a cor-chave vira transparente. E assim que um desenho
// sem caixa - um icone solto sobre a arte do jogo - e recortado.
void TelaFechaChave(Tela* t, COLORREF chave, BYTE alfa);

#endif
