// Os icones dos itens, lidos dos proprios arquivos do cliente.
//
// Enquanto a loja desenhava um losango no lugar do item, ela nao parecia do
// jogo. O desenho de verdade mora em UI\itemicon01..10.wyt, e chegar nele e uma
// cadeia de tres arquivos, todos do cliente:
//
//   itemicon.bin            6500 inteiros de 32 bits, um por item; o valor e o
//                           numero do icone, e -1 quer dizer "sem icone".
//   UI\UITextureSetList.txt o conjunto [ItemIcon], 1000 retangulos de 35x35 na
//                           forma "textura,x,y,larg,alt,0,0".
//   UI\UITextureListN.bin   registros de 264 bytes: o numero da textura vira o
//                           caminho do .wyt.
//
// O .wyt e um WT10: "WT10", largura em 0x10, altura em 0x12, bits por pixel em
// 0x14 e os pixels a partir de 0x16, de cima para baixo, em BGRA (ou BGR nas
// folhas de 24 bits, onde o preto e o que fica transparente).
//
// Nada disto e desempacotado na carga: as folhas sao lidas na primeira vez que
// um icone delas aparece.

#ifndef ICONES_H
#define ICONES_H

// Desenha o icone do item no DIB de 32 bits (A8R8G8B8, de cima para baixo) que
// a camada esta pintando. Devolve 0 quando o item nao tem icone - e ai quem
// chama desenha o que desenhava antes.
//
// pixels/telaL/telaA descrevem o DIB inteiro; x,y sao o canto do icone nele.
int IconeDesenha(void* pixels, int telaL, int telaA, int x, int y, int item);

// O lado do icone, em pixels. E a medida do proprio jogo.
int IconeLado();

#endif
