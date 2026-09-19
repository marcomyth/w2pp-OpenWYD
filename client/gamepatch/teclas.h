// Teclas que os nossos paineis tomam do jogo.
//
// Uma tecla chega a WndProc do cliente (0x54BFBD) TRES vezes, e as tres acabam
// no mesmo repasse (0x54D334) para o segundo tratador - que e quem abre o menu
// da engrenagem no Esc:
//
//   WM_KEYDOWN  0x54C62F
//   WM_CHAR     0x54C966   nascido do TranslateMessage, ja na fila quando a
//                          WndProc devolve, entao nao adianta so cortar a descida
//   WM_KEYUP    0x54C88F
//
// Quem engole a descida marca a tecla; o caractere e a subida olham a marca, e a
// subida apaga. Cortar so a descida deixava os outros dois passarem - e, com o
// painel ja fechado, a condicao de "e minha" nem valia mais.
//
// Este modulo instala os tres desvios uma vez e reparte a tecla entre os
// paineis. E o mesmo desenho das camadas: quem tem ordem maior e perguntado
// primeiro, e o primeiro que disser "e minha" fica com ela.

#ifndef TECLAS_H
#define TECLAS_H

// trata devolve 1 quando consumiu a tecla - e entao o jogo nao a ve.
void TeclaRegistra(int vk, int ordem, int (*trata)());

#endif
