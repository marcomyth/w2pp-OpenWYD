// Os icones dos itens, tirados do proprio cliente - da textura que ele ja tem
// carregada, nao de arquivo nenhum nosso.
//
// A primeira versao disto lia os .wyt da pasta por conta propria e remontava o
// desenho. Alem de ser um segundo caminho para a mesma coisa, dava o item
// errado: a folha solta no disco nao e necessariamente a que o cliente usa.
// Agora a loja pergunta ao cliente, e a cadeia e toda memoria dele:
//
//   0x6EA518                    o itemicon.bin que ele ja leu: um inteiro por
//                               item, e o numero do icone e esse valor MENOS UM
//                               - e o que o cliente faz em 0x40D6D5.
//   gerenciador + 0x328/+0x32C  os conjuntos de sprites do UITextureSetList; o
//                               [ItemIcon] e o 526, e cada entrada tem 28 bytes
//                               (textura, x, y, largura, altura, ...).
//   gerenciador + 0xE85E8       um IDirect3DTexture9* por textura, criado pelo
//                               proprio cliente em 0x4BEA19.
//
// O gerenciador nao tem ponteiro global obvio, mas tem marca: o
// UITextureListN.bin inteiro mora em +0x15E8, e comeca com "UI\\cursor.wyt".
// Achamos o objeto por esse conteudo, uma vez so.

#ifndef ICONES_H
#define ICONES_H

// Desenha o icone do item centrado no quadrado dado, dentro do DIB de 32 bits
// (A8R8G8B8, de cima para baixo) que a camada esta pintando. Devolve 0 quando
// nao ha icone - e ai quem chama desenha o que desenhava antes.
int IconeDesenha(void* pixels, int telaL, int telaA, int quadX, int quadY, int quadL, int quadA,
                 int item);

#endif
