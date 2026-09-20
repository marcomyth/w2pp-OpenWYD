// O fundo do painel da loja: a marca do servidor, desenhada atras das portas.
#ifndef FUNDO_H
#define FUNDO_H

// Soma a marca ao que ja esta pintado no DIB de 32 bits (A8R8G8B8, de cima para
// baixo), centrada no ponto dado. Some sozinha nas bordas porque a arte foi
// gerada sem fundo proprio - ver arte/gera_fundo.py. Devolve 0 quando a arte
// nao esta no DLL, e ai o painel fica como era antes dela.
int FundoDesenha(void* pixels, int telaL, int telaA, int centroX, int centroY);

#endif
