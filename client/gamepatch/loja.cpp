// Loja do Servidor: a vitrine global, desenhada dentro do quadro do jogo.
//
// A ideia combinada: cada jogador continua abrindo a lojinha pessoal como sempre
// (autotrade), as barracas ficam na cidade, e este painel agrupa o que todas
// elas estao vendendo - com preco em Ouro, Cash ou RMT, do jeito que o dono
// escolheu. A lista vem do servidor (lojarede.h); nao ha nada inventado aqui.
//
// Sao duas camadas. A janela, pintada em GDI com os pinceis de pincel.h - dai
// ter a mesma moldura, o mesmo cabecalho e os mesmos botoes do painel de alvos.
// E uma camada sem desenho nenhum sobre o botao de Loja Pessoal da barra: o
// jogo continua desenhando o icone dele, e nos so ficamos com o clique.
//
// Chegamos a desenhar um icone proprio ali, mas ele nunca passou por nativo e
// ainda dependia de adivinhar, lendo a cor do quadro, se a barra estava aberta.
// Roubar o clique resolveu as duas coisas de uma vez.

#include "camadas.h"

extern "C" void __cdecl D3DDesenhaCamadasAgora();

#include "fundo.h"
#include "icones.h"
#include "lojarede.h"
#include "pincel.h"
#include "teclas.h"

#include <cstdio>
#include <cstring>

namespace {

// --- medidas da janela -----------------------------------------------------
//
// A loja tem a medida exata da janela do Banco do cliente e fica encostada nela
// pela esquerda. Os numeros nao sao chute nem medida de captura de tela: o
// desvio do AppendNode anotou o retangulo que o proprio cliente desenha - Banco
// em x=367 y=44, 290x538, e o inventario colado a direita dele, em x=657 -,
// numa tela de 1024x768.
//
// A janela e em pe, entao a planta mudou de forma junto: os botoes que viviam
// numa coluna a esquerda viraram duas fileiras em cima, e os saldos desceram
// para o rodape, logo acima dos botoes de acao.
constexpr int kBorda = 5;            // as tres linhas da borda dupla
constexpr int kPad = 6;
constexpr int kMargem = kBorda + kPad;
constexpr int kLargura = 290;
constexpr int kAltura = 538;
constexpr int kBancoX = 367;         // canto esquerdo da janela do Banco
constexpr int kBancoY = 44;

constexpr int kAltCabecalho = 17;
constexpr int kAltSaldos = 26;       // a faixa das moedas, no rodape
constexpr int kAltBotao = 22;
constexpr int kEspacoBotao = 4;
constexpr int kMenuLinha1 = 4;       // quantos botoes cabem na fileira de cima
constexpr int kSlot = 48;            // quadrado de um item
constexpr int kEspacoSlot = 4;
constexpr int kColunas = 5;
constexpr int kLinhas = 7;
constexpr int kPorPagina = kColunas * kLinhas;
constexpr int kMaxPrateleiras = 12;   // MAX_AUTOTRADE, as prateleiras da barraca
constexpr int kAltDetalhe = 16;
constexpr int kAltRodape = 22;

constexpr int kUtil = kLargura - kMargem * 2;
constexpr int kTopoMenu = kMargem + kAltCabecalho + 6;
constexpr int kTopoMenu2 = kTopoMenu + kAltBotao + kEspacoBotao;
constexpr int kTopoConteudo = kTopoMenu2 + kAltBotao + 8;
constexpr int kAltGrade = kLinhas * kSlot + (kLinhas - 1) * kEspacoSlot;
constexpr int kLargGrade = kColunas * kSlot + (kColunas - 1) * kEspacoSlot;
constexpr int kEsqGrade = (kLargura - kLargGrade) / 2;
// A janela tem altura fixa (e a do Banco), entao o que a faixa das moedas
// ganhou de altura saiu das folgas entre ela, o detalhe e o rodape: 6 pixels de
// respiro viraram 4, e o rodape continua terminando onde terminava.
constexpr int kFolga = 4;
constexpr int kTopoDetalhe = kTopoConteudo + kAltGrade + kFolga;
constexpr int kTopoSaldos = kTopoDetalhe + kAltDetalhe + kFolga;

// --- a tela de montar a barraca --------------------------------------------
//
// Ela nao usa a grade da vitrine. Sao duas areas, uma em cima da outra, que e
// como a barraca de verdade funciona: em cima o cofre inteiro - as quatro abas
// do banco numa lista so, com barra de rolagem -, e embaixo a previa da
// lojinha, com as doze prateleiras que a barraca tem. O que se tira de uma vai
// para a outra.
constexpr int kCofreColunas = 5;
constexpr int kCofreLinhasVis = 6;      // quantas linhas do cofre cabem de uma vez
constexpr int kCofreSlot = 46;
constexpr int kEspacoCofre = 4;
constexpr int kBarraRolagem = 9;        // a barra de rolagem, a direita do cofre
constexpr int kPrateleiraColunas = 6;
constexpr int kPrateleiraLinhas = 2;    // 6 x 2 = as 12 prateleiras da barraca
constexpr int kPrateleiraSlot = 40;
constexpr int kAltRotuloPrat = 14;
constexpr int kAltAbrir = 25;           // o botao "Abrir loja", sozinho no rodape
constexpr int kLargAbrir = 180;

// --- cores das moedas ------------------------------------------------------
constexpr int kOuro = 0;
constexpr int kCash = 1;
constexpr int kRmt = 2;

const COLORREF kCorMoeda[3] = {
    RGB(232, 196, 106),   // ouro
    RGB(111, 168, 255),   // cash
    RGB(126, 217, 139),   // rmt
};
const char* const kNomeMoeda[3] = {"Ouro", "Cash", "RMT"};

// --- menu da esquerda ------------------------------------------------------
// Os quatro primeiros filtram a vitrine; os tres ultimos sao acoes.
enum {
    kMenuTodos = 0,
    kMenuOuro,
    kMenuCash,
    kMenuRmt,
    kMenuMeus,
    kMenuSaque,
    kMenuVoltar,
    kMenuTotal,
};

const char* const kMenu[kMenuTotal] = {"Todos",      "Ouro",           "Cash",
                                       "RMT",        "Meus itens",     "Realizar saque",
                                       "Voltar"};

// A fileira de botoes de preco ("Zerar", "+1 mil", "+100 mil", "+1 milhao",
// "Moeda") foi embora junto com a Barraca: quem pergunta moeda e valor agora e
// a caixa que abre ao clicar duas vezes no item, e la o valor e digitado.
constexpr int kPrecoMax = 1999999999;

// --- estado ----------------------------------------------------------------
Tela g_tela;
HFONT g_fonte = nullptr;
HFONT g_negrito = nullptr;
HFONT g_miudo = nullptr;
HFONT g_medio = nullptr;
HFONT g_grande = nullptr;
bool g_aberta = false;
int g_filtro = kMenuTodos;
int g_pagina = 0;
int g_escolhido = -1;
int g_versao = 0;

// Montar a barraca: o painel virou a janela de montagem, porque a do cliente so
// sabia de ouro. Nao ha onde digitar - o cliente le o teclado por conta dele e
// cada numero e um atalho do jogo -, entao o preco entra por botoes e o titulo
// e o nome do personagem, posto pelo servidor.
// --- as tres telas do painel ------------------------------------------------
//
// Uma janela so com tudo dentro ficou dificil de ler: filtros de moeda, cofre,
// preco, nome e barraca disputavam a mesma tela. Agora sao tres telas no mesmo
// lugar, e a escolha de quem abre e do jogador:
//
//   Menu      duas portas, so isso: o Mercado Global e a Criar sua Lojinha.
//   Mercado   a vitrine da cidade, com os filtros por moeda.
//   Montagem  o cofre, o preco, a moeda e o nome - o caminho de abrir barraca.
enum {
    kTelaMenu = 0,
    kTelaMercado,
    kTelaMontagem,
};
int g_qualTela = kTelaMenu;

bool Montando() {
    return g_qualTela == kTelaMontagem;
}
// O nome da barraca, digitado na tela de montagem. Vazio: o servidor poe o nome
// do personagem, como a lojinha do jogo faz.
char g_nomeBarraca[24] = {0};
int g_nomeTam = 0;
int g_cofreRolagem = 0;       // primeira linha visivel do cofre
int g_cofreEscolhido = -1;    // indice na lista do cofre

// --- as caixas que perguntam -------------------------------------------------
//
// Duas perguntas saiam do painel e viraram caixa por cima dele, como as do
// jogo: o nome da loja, antes de montar coisa nenhuma, e o preco de cada item,
// no momento em que ele e escolhido. O painel atras escurece, o Esc fecha a
// caixa (e so ela), e enquanto uma estiver aberta nenhum clique chega ao que
// esta embaixo.
enum {
    kCaixaNenhuma = 0,
    kCaixaNome,
    kCaixaPreco,
};
int g_caixa = kCaixaNenhuma;

// O ultimo clique numa prateleira do cofre, para reconhecer os dois cliques que
// abrem a caixa do preco.
DWORD g_cliqueEm = 0;
int g_cliqueQual = -1;
int g_precoEdicao = 0;
int g_moedaEdicao = kOuro;
LojaPrateleira g_prateleiras[kMaxPrateleiras];
int g_prateleirasUsadas = 0;

bool g_roubaClique = true;   // desligado durante investigacoes no botao original

// O item sob o cursor, que e o que a dica do jogo descreve.
int g_dicaItem = 0;
int g_dicaRefino = 0;
int g_camadaDaDica = -1;
RECT g_dicaSlot = {0, 0, 0, 0};

// --- diagnostico -----------------------------------------------------------
//
// Ligado por diagnostico=1 no loja.txt, e so entao: anota no alvos.log o
// retangulo das janelas grandes que o cliente desenha, a troca da janela ativa
// e cada caractere que chega ao OnChar. E assim que se descobre a medida exata
// de uma janela do jogo - a do Banco, por exemplo - sem chutar em cima de uma
// captura de tela, e por onde uma tecla realmente passa.
int g_diag = 0;

// Em jogo, ou ainda na tela de servidor/personagem? A cena do cliente
// (0x6F0AB0) so existe depois de entrar com o personagem, e o +0x4C guarda a
// nossa propria entidade - os mesmos enderecos que o sistema de alvos usa.
constexpr DWORD kCena = 0x6F0AB0;
constexpr DWORD kSelfOff = 0x4C;

// --- tomando o lugar do botao do jogo --------------------------------------
//
// 0x60F4FC guarda o id da janela ativa do cliente: 0xFFFF quando nao ha nenhuma
// e 0x1388 (5000) quando a Loja Pessoal abre. O endereco saiu de um cruzamento
// de fotos da memoria - com a janela fechada e aberta, tres rodadas - e foi o
// UNICO valor que acompanhou o estado da janela.
//
// Desviar as instrucoes que escrevem ali nao pegou o clique, entao a leitura e
// por conta propria, a cada quadro: vendo 5000, devolvemos 0xFFFF (que e o que o
// cliente escreve ao fechar) e abrimos a nossa loja no lugar. E por isso que nao
// existe mais icone nosso na barra: quem abre a loja e o botao original.
constexpr DWORD kIdJanelaAtiva = 0x0060F4FC;
constexpr WORD kIdLojaPessoal = 0x1388;
constexpr WORD kIdNenhuma = 0xFFFF;

bool EmJogo() {
    BYTE* cena = *reinterpret_cast<BYTE**>(kCena);
    if (cena == nullptr) {
        return false;
    }
    return *reinterpret_cast<BYTE**>(cena + kSelfOff) != nullptr;
}

// O icone ocupa o lugar do antigo botao de Loja Pessoal na barra: e por ele que
// se abre a lojinha agora.
//
// Medido num print do proprio cliente em 1024x768: a celula da Loja Pessoal tem
// moldura clara em x 660..663 e nas linhas y 669 e y 705, e o interior - onde a
// arte do icone mora - vai de x 664 a 693 e de y 670 a 704. Nao e quadrado: 30
// por 35. Como a barra e ancorada embaixo, os valores seguem valendo em outras
// resolucoes; ficam em loja.txt para dar para acertar sem recompilar.
int g_iconeDx = 152;    // 664 - 512, do centro da tela para a direita
int g_iconeDy = 98;     // 768 - 670, da base da tela para cima
int g_iconeL = 30;
int g_iconeA = 35;

void Log(const char* texto) {
    CamadaLog(texto);
}

void CarregaConfig() {
    char caminho[MAX_PATH];
    GetModuleFileNameA(nullptr, caminho, MAX_PATH);
    char* barra = strrchr(caminho, 92);
    if (barra == nullptr) {
        return;
    }
    strcpy_s(barra + 1, MAX_PATH - (barra + 1 - caminho), "loja.txt");
    FILE* f = nullptr;
    if (fopen_s(&f, caminho, "r") != 0 || f == nullptr) {
        // Nao existe ainda: escreve com os valores de fabrica, para servir de
        // exemplo de onde mexer.
        if (fopen_s(&f, caminho, "w") == 0 && f != nullptr) {
            fputs("# Loja do Servidor.\n", f);
            fputs("# icone_dx = deslocamento do icone a partir do centro da tela\n", f);
            fputs("# icone_dy = altura do icone contada a partir da base da tela\n", f);
            fputs("# icone_larg / icone_alt = tamanho do icone\n", f);
            fprintf(f, "icone_dx=%d\n", g_iconeDx);
            fprintf(f, "icone_dy=%d\n", g_iconeDy);
            fprintf(f, "icone_larg=%d\n", g_iconeL);
            fprintf(f, "icone_alt=%d\n", g_iconeA);
            fclose(f);
        }
        return;
    }
    char linha[128];
    while (fgets(linha, sizeof(linha), f) != nullptr) {
        if (linha[0] == '#') {
            continue;
        }
        char* igual = strchr(linha, '=');
        if (igual == nullptr) {
            continue;
        }
        *igual = 0;
        const int valor = atoi(igual + 1);
        if (strcmp(linha, "icone_dx") == 0) {
            g_iconeDx = valor;
        } else if (strcmp(linha, "icone_dy") == 0) {
            g_iconeDy = valor;
        } else if (strcmp(linha, "icone_larg") == 0) {
            g_iconeL = valor;
        } else if (strcmp(linha, "icone_alt") == 0) {
            g_iconeA = valor;
        } else if (strcmp(linha, "diagnostico") == 0) {
            g_diag = valor;
        }
    }
    fclose(f);
}

void CriaFontes() {
    if (g_fonte != nullptr) {
        return;
    }
    g_fonte = CreateFontA(13, 0, 0, 0, FW_NORMAL, FALSE, FALSE, FALSE, DEFAULT_CHARSET,
                          OUT_DEFAULT_PRECIS, CLIP_DEFAULT_PRECIS, ANTIALIASED_QUALITY,
                          DEFAULT_PITCH | FF_DONTCARE, "Segoe UI");
    g_negrito = CreateFontA(13, 0, 0, 0, FW_BOLD, FALSE, FALSE, FALSE, DEFAULT_CHARSET,
                            OUT_DEFAULT_PRECIS, CLIP_DEFAULT_PRECIS, ANTIALIASED_QUALITY,
                            DEFAULT_PITCH | FF_DONTCARE, "Segoe UI");
    g_miudo = CreateFontA(11, 0, 0, 0, FW_NORMAL, FALSE, FALSE, FALSE, DEFAULT_CHARSET,
                          OUT_DEFAULT_PRECIS, CLIP_DEFAULT_PRECIS, ANTIALIASED_QUALITY,
                          DEFAULT_PITCH | FF_DONTCARE, "Segoe UI");
    // Entre a miuda e a normal: os saldos precisam de corpo, mas tres deles
    // dividem 268 pixels e a de 13 estouraria a celula em "Ouro 999,3M".
    // A frase que nomeia as duas portas: e a unica coisa escrita naquela tela,
    // entao pede corpo maior que o resto do painel.
    g_grande = CreateFontA(16, 0, 0, 0, FW_BOLD, FALSE, FALSE, FALSE, DEFAULT_CHARSET,
                           OUT_DEFAULT_PRECIS, CLIP_DEFAULT_PRECIS, ANTIALIASED_QUALITY,
                           DEFAULT_PITCH | FF_DONTCARE, "Segoe UI");
    g_medio = CreateFontA(12, 0, 0, 0, FW_NORMAL, FALSE, FALSE, FALSE, DEFAULT_CHARSET,
                          OUT_DEFAULT_PRECIS, CLIP_DEFAULT_PRECIS, ANTIALIASED_QUALITY,
                          DEFAULT_PITCH | FF_DONTCARE, "Segoe UI");
}

// 250000000 -> "250.000.000", que e como o jogo escreve dinheiro.
void Pontuado(long long v, char* saida, size_t n) {
    char cru[32];
    sprintf_s(cru, "%lld", v);
    const int len = static_cast<int>(strlen(cru));
    int k = 0;
    for (int i = 0; i < len && k < static_cast<int>(n) - 1; ++i) {
        if (i > 0 && (len - i) % 3 == 0) {
            saida[k++] = '.';
        }
        saida[k++] = cru[i];
    }
    saida[k] = 0;
}

// No quadrado do item nao cabe o numero inteiro: 250000000 vira "250M".
// Numero curto, do jeito que cabe num quadrado ou no rodape. Abaixo de dez mil
// ele vai inteiro - e a faixa onde cash e RMT vivem, e arredondar ali seria
// esconder justamente o que interessa.
void Curto(long long v, char* saida, size_t n) {
    if (v >= 1000000000LL) {
        sprintf_s(saida, n, "%lld,%lldB", v / 1000000000LL, (v / 100000000LL) % 10);
    } else if (v >= 1000000LL) {
        sprintf_s(saida, n, "%lld,%lldM", v / 1000000LL, (v / 100000LL) % 10);
    } else if (v >= 10000LL) {
        sprintf_s(saida, n, "%lldk", v / 1000LL);
    } else {
        sprintf_s(saida, n, "%lld", v);
    }
}

// --- a vitrine, como o servidor mandou -------------------------------------
//
// O filtro e a pagina sao decididos LA: o pedido leva os dois e a resposta ja
// vem pronta. Aqui so guardamos o que foi pedido, para repetir o pedido quando o
// jogador mexe no menu ou vira a pagina.
DWORD g_ultimoPedido = 0;

void PedeAoServidor() {
    LojaRedePede(g_pagina, g_filtro);
    g_ultimoPedido = GetTickCount();
}

// Enquanto o painel esta aberto a lista e renovada de tempos em tempos: barraca
// que abre ou fecha muda a vitrine, e ninguem avisa.
// O painel nao pergunta mais de tempos em tempos, e a vitrine que ele mostra e
// dele: fica guardada aqui enquanto a sessao durar.
//
// Perguntar de tres em tres segundos era varrer o mercado inteiro do servidor
// por jogador com o painel aberto, quase sempre para receber a mesma lista. E
// receber a pagina pronta a cada mudanca era pior ainda do lado do servidor:
// 1,26 ms e 2,4 MB por pessoa olhando, em cada compra do servidor inteiro.
//
// O acordo agora e outro: o servidor manda um bilhete de quatro bytes dizendo
// que o mercado mudou, e este painel decide se vale pedir - so a pagina visivel,
// no maximo uma por segundo. Ele pergunta quando abre, quando troca de pagina ou
// de filtro, depois das proprias acoes e quando o bilhete chega; e avisa ao
// fechar, para o servidor tirar ele da lista de quem esta olhando.
void RenovaSePreciso() {
    // Fora do mundo nao ha cache: o que o painel guardou morre com a sessao.
    static bool estavaEmJogo = false;
    const bool agoraEmJogo = EmJogo();
    if (estavaEmJogo && !agoraEmJogo) {
        LojaRedeEsquece();
    }
    estavaEmJogo = agoraEmJogo;

    static bool estavaAberta = false;
    if (g_aberta != estavaAberta) {
        estavaAberta = g_aberta;
        if (g_aberta) {
            PedeAoServidor();
        } else {
            LojaRedeFecha();
        }
        return;
    }
    // O servidor mandou o bilhete: a pagina que esta na tela envelheceu. Pede a
    // nova, mas com freio - no maximo uma por segundo, por mais que o mercado se
    // agite. Sem o freio, um mercado movimentado viraria um pedido por compra de
    // qualquer pessoa do servidor.
    if (g_aberta && LojaRedeVitrineVelha() &&
        GetTickCount() - g_ultimoPedido >= 1000) {
        LojaRedeVitrineEmDia();
        PedeAoServidor();
    }
}

int Paginas() {
    const int p = LojaRedePaginas();
    return p < 1 ? 1 : p;
}

// --- areas clicaveis -------------------------------------------------------
RECT AreaFechar() {
    RECT r = {kLargura - kMargem - 16, kMargem + 2, kLargura - kMargem, kMargem + 15};
    return r;
}

// Os botoes em duas fileiras: quatro em cima, tres embaixo. A de cima leva os
// nomes curtos (os filtros, e os degraus de preco na montagem) justamente
// porque e a estreita.
RECT AreaMenu(int i) {
    const bool cima = i < kMenuLinha1;
    const int quantos = cima ? kMenuLinha1 : kMenuTotal - kMenuLinha1;
    const int coluna = cima ? i : i - kMenuLinha1;
    const int largura = (kUtil - (quantos - 1) * kEspacoBotao) / quantos;
    const int x = kMargem + coluna * (largura + kEspacoBotao);
    const int y = cima ? kTopoMenu : kTopoMenu2;
    // O ultimo da fileira come a sobra da divisao, para a fileira fechar certo
    // na margem direita.
    const int fim = coluna + 1 == quantos ? kLargura - kMargem : x + largura;
    RECT r = {x, y, fim, y + kAltBotao};
    return r;
}

RECT AreaSlot(int i) {
    const int col = i % kColunas;
    const int lin = i / kColunas;
    const int x = kEsqGrade + col * (kSlot + kEspacoSlot);
    const int y = kTopoConteudo + lin * (kSlot + kEspacoSlot);
    RECT r = {x, y, x + kSlot, y + kSlot};
    return r;
}

int TopoRodape() {
    return kTopoSaldos + kAltSaldos + kFolga;
}

// O rodape e uma fileira so: paginacao a esquerda, Comprar e Fechar a direita.
RECT AreaSeta(bool direita) {
    const int y = TopoRodape();
    const int x = direita ? kMargem + 64 : kMargem;
    RECT r = {x, y, x + 18, y + kAltRodape};
    return r;
}

RECT AreaBotaoFechar() {
    RECT r = {kLargura - kMargem - 76, TopoRodape(), kLargura - kMargem,
              TopoRodape() + kAltRodape};
    return r;
}

RECT AreaBotaoComprar() {
    const RECT f = AreaBotaoFechar();
    RECT r = {f.left - 86, f.top, f.left - 6, f.bottom};
    return r;
}

// --- pintura ---------------------------------------------------------------
// --- o menu inicial ---------------------------------------------------------
//
// Duas portas, no meio do painel, do tamanho que se le de longe. Nada mais:
// quem abre a loja escolhe primeiro o que veio fazer, e so entao ve os
// controles daquilo. A janela e a mesma nas tres telas - o que muda e o que
// esta dentro dela.
constexpr int kPortaL = 200;
constexpr int kPortaA = 46;
constexpr int kPortaEspaco = 18;

const char* const kPortas[2] = {"Mercado Global", "Criar sua Lojinha"};

// A frase fica no meio do vao entre o cabecalho e a primeira porta - nem colada
// no titulo, nem encostada nos botoes. E onde a Josiel apontou, e tambem o unico
// ponto que nao precisa de numero escolhido a dedo: e a metade do espaco vazio.
constexpr int kAltRotulo = 26;

RECT AreaPorta(int qual) {
    const int x = (kLargura - kPortaL) / 2;
    const int meio = kTopoConteudo + kAltGrade / 2;
    const int y = meio - (kPortaA * 2 + kPortaEspaco) / 2 + qual * (kPortaA + kPortaEspaco);
    RECT r = {x, y, x + kPortaL, y + kPortaA};
    return r;
}

RECT AreaRotulo() {
    const int abaixoDoTitulo = kMargem + kAltCabecalho;
    const int meio = (abaixoDoTitulo + AreaPorta(0).top) / 2;
    RECT r = {kMargem, meio - kAltRotulo / 2, kLargura - kMargem, meio + kAltRotulo / 2};
    return r;
}

// --- a tela de montagem: onde fica cada coisa -------------------------------
//
// De cima para baixo: o "Voltar", o cofre com a barra de rolagem, o rotulo da
// previa, as doze prateleiras e, no rodape, so o "Abrir loja". Tudo sai de
// numeros ja existentes - o topo do menu e o topo dos saldos -, entao mexer na
// altura da janela nao desalinha nada.
constexpr int kTopoCofre = kTopoMenu + kAltBotao + 8;
constexpr int kAltCofre = kCofreLinhasVis * kCofreSlot + (kCofreLinhasVis - 1) * kEspacoCofre;
constexpr int kLargCofre = kCofreColunas * kCofreSlot + (kCofreColunas - 1) * kEspacoCofre;
constexpr int kEsqCofre = kMargem + 2;
constexpr int kEsqBarra = kMargem + kUtil - 2 - kBarraRolagem;
constexpr int kAltPrateleiras =
    kPrateleiraLinhas * kPrateleiraSlot + (kPrateleiraLinhas - 1) * kEspacoCofre;
constexpr int kLargPrateleiras =
    kPrateleiraColunas * kPrateleiraSlot + (kPrateleiraColunas - 1) * kEspacoCofre;
constexpr int kEsqPrateleiras = kMargem + (kUtil - kLargPrateleiras) / 2;
constexpr int kTopoPrateleiras = kTopoSaldos - kFolga - kAltPrateleiras;
// O rotulo nao fica encostado na grade de baixo: ele e a legenda do que separa
// as duas, entao mora no meio do vao entre elas.
constexpr int kMeioDoVao = (kTopoCofre + kAltCofre + kTopoPrateleiras) / 2;
constexpr int kTopoRotuloPrat = kMeioDoVao - kAltRotuloPrat / 2;

RECT AreaVoltar() {
    RECT r = {kMargem, kTopoMenu, kMargem + 70, kTopoMenu + kAltBotao};
    return r;
}

RECT AreaSlotCofre(int visivel) {
    const int col = visivel % kCofreColunas;
    const int lin = visivel / kCofreColunas;
    const int x = kEsqCofre + col * (kCofreSlot + kEspacoCofre);
    const int y = kTopoCofre + lin * (kCofreSlot + kEspacoCofre);
    RECT r = {x, y, x + kCofreSlot, y + kCofreSlot};
    return r;
}

RECT AreaSlotPrateleira(int i) {
    const int col = i % kPrateleiraColunas;
    const int lin = i / kPrateleiraColunas;
    const int x = kEsqPrateleiras + col * (kPrateleiraSlot + kEspacoCofre);
    const int y = kTopoPrateleiras + lin * (kPrateleiraSlot + kEspacoCofre);
    RECT r = {x, y, x + kPrateleiraSlot, y + kPrateleiraSlot};
    return r;
}

RECT AreaTrilho() {
    RECT r = {kEsqBarra, kTopoCofre, kEsqBarra + kBarraRolagem, kTopoCofre + kAltCofre};
    return r;
}

RECT AreaAbrirLoja() {
    const int y = kAltura - kMargem - kAltAbrir;
    RECT r = {(kLargura - kLargAbrir) / 2, y, (kLargura + kLargAbrir) / 2, y + kAltAbrir};
    return r;
}

int CofreVisiveis() {
    return kCofreColunas * kCofreLinhasVis;
}

int CofreLinhas() {
    const int n = LojaRedeCofreQtd();
    const int l = (n + kCofreColunas - 1) / kCofreColunas;
    return l < 1 ? 1 : l;
}

int RolagemMax() {
    const int sobra = CofreLinhas() - kCofreLinhasVis;
    return sobra > 0 ? sobra : 0;
}

// O polegar da barra: alto na proporcao do que se ve, e na posicao da rolagem.
RECT AreaPolegar() {
    const RECT t = AreaTrilho();
    const int trilho = t.bottom - t.top;
    const int linhas = CofreLinhas();
    int alto = linhas > 0 ? trilho * kCofreLinhasVis / linhas : trilho;
    if (alto > trilho) {
        alto = trilho;
    }
    if (alto < 20) {
        alto = 20;
    }
    const int max = RolagemMax();
    const int y = max > 0 ? t.top + (trilho - alto) * g_cofreRolagem / max : t.top;
    RECT r = {t.left, y, t.right, y + alto};
    return r;
}

// --- as caixas que perguntam: onde fica cada coisa --------------------------
constexpr int kCaixaL = 238;
constexpr int kAltCaixaCab = 20;
constexpr int kAltCaixaBotao = 24;
constexpr int kAltCaixaCampo = 24;

int CaixaAltura() {
    return g_caixa == kCaixaNome ? 120 : 156;
}

RECT AreaCaixa() {
    const int a = CaixaAltura();
    RECT r = {(kLargura - kCaixaL) / 2, (kAltura - a) / 2, (kLargura + kCaixaL) / 2,
              (kAltura + a) / 2};
    return r;
}

// 0 e o da esquerda (Voltar, Cancelar), 1 o da direita (Prosseguir, Confirmar).
RECT AreaCaixaBotao(int qual) {
    const RECT p = AreaCaixa();
    const int larg = (kCaixaL - 30) / 2;
    const int y = p.bottom - 12 - kAltCaixaBotao;
    const int x = qual == 0 ? p.left + 10 : p.right - 10 - larg;
    RECT r = {x, y, x + larg, y + kAltCaixaBotao};
    return r;
}

RECT AreaCaixaCampo() {
    const RECT p = AreaCaixa();
    const int y = p.top + (g_caixa == kCaixaNome ? 48 : 86);
    RECT r = {p.left + 10, y, p.right - 10, y + kAltCaixaCampo};
    return r;
}

RECT AreaCaixaMoeda(int i) {
    const RECT p = AreaCaixa();
    const int larg = (kCaixaL - 20 - 2 * 6) / 3;
    const int x = p.left + 10 + i * (larg + 6);
    const int y = p.top + 48;
    RECT r = {x, y, x + larg, y + 22};
    return r;
}

// Apaga um pedaco do que ja foi pintado: e assim que um botao fica cinza sem
// precisar de uma segunda paleta so para ele. O GDI nao mistura, entao a conta
// e feita nos pixels, como no fundo e nas caixas.
void Apaga(const RECT& r) {
    GdiFlush();
    BYTE* tela = static_cast<BYTE*>(g_tela.pixels);
    if (tela == nullptr) {
        return;
    }
    for (int y = r.top; y < r.bottom; ++y) {
        if (y < 0 || y >= g_tela.a) {
            continue;
        }
        BYTE* linha = tela + static_cast<size_t>(y) * g_tela.l * 4;
        for (int x = r.left; x < r.right; ++x) {
            if (x < 0 || x >= g_tela.l) {
                continue;
            }
            BYTE* p = linha + static_cast<size_t>(x) * 4;
            p[0] = static_cast<BYTE>(p[0] * 45 / 100);
            p[1] = static_cast<BYTE>(p[1] * 45 / 100);
            p[2] = static_cast<BYTE>(p[2] * 45 / 100);
        }
    }
}

void PintaMenuInicial(HDC hdc) {
    // A marca do servidor fica atras das portas, e so aqui: nas outras telas o
    // que ocupa este espaco e a grade de itens, e brilho atras de icone e
    // sujeira. Ela e somada aos pixels, entao vem antes do que o GDI desenha
    // por cima - ver fundo.cpp.
    FundoDesenha(g_tela.pixels, g_tela.l, g_tela.a, kLargura / 2,
                 kTopoConteudo + kAltGrade / 2);

    SelectObject(hdc, g_grande);
    SetTextColor(hdc, kTexto);
    RECT rr = AreaRotulo();
    // A cedilha e o til vao em hexadecimal de proposito: DrawTextA le os bytes na
    // pagina ANSI do sistema, e assim a cedilha e o til nao dependem de como o
    // compilador leu este arquivo.
    DrawTextA(hdc, "Escolha uma op\xE7\xE3o", -1, &rr,
              DT_CENTER | DT_VCENTER | DT_SINGLELINE | DT_NOPREFIX);

    // So se monta uma barraca por vez. Com uma de pe a porta apaga e diz o
    // porque, em vez de deixar clicar e recusar la dentro.
    const bool jaTemBarraca = LojaRedeBarracaAberta() != 0;
    for (int i = 0; i < 2; ++i) {
        const RECT r = AreaPorta(i);
        const bool apagada = i == 1 && jaTemBarraca;
        LinhaBotao(hdc, r.left, r.top, kPortaL, kPortaA, false);
        SelectObject(hdc, g_negrito);
        SetTextColor(hdc, apagada ? kTextoFraco : kTexto);
        RECT rt = {r.left, r.top, r.right, r.bottom};
        DrawTextA(hdc, apagada ? "Lojinha j\xE1 aberta" : kPortas[i], -1, &rt,
                  DT_CENTER | DT_VCENTER | DT_SINGLELINE | DT_NOPREFIX);
        if (apagada) {
            Apaga(r);
        }
    }
}

void PintaCabecalho(HDC hdc) {
    const int x = kMargem;
    const int y = kMargem;
    const int l = kUtil;

    Degrade(hdc, x, y, l, kAltCabecalho, kCabTopo, kCabBaixo);
    Contorno(hdc, x, y, l, kAltCabecalho, kCabBorda);
    Losango(hdc, x + 9, y + kAltCabecalho / 2, 3, kLosango);

    SelectObject(hdc, g_negrito);
    SetTextColor(hdc, RGB(255, 255, 255));
    RECT rt = {x + 18, y, x + l - 30, y + kAltCabecalho};
    const char* titulo = g_qualTela == kTelaMenu
                             ? "Loja do Servidor"
                             : (Montando() ? "Criar sua Lojinha" : "Mercado Global");
    DrawTextA(hdc, titulo, -1, &rt,
              DT_LEFT | DT_VCENTER | DT_SINGLELINE | DT_NOPREFIX);

    const RECT f = AreaFechar();
    SelectObject(hdc, g_fonte);
    SetTextColor(hdc, kLosango);
    RECT rx = {f.left, f.top - 3, f.right, f.bottom + 3};
    DrawTextA(hdc, "X", -1, &rx, DT_CENTER | DT_VCENTER | DT_SINGLELINE | DT_NOPREFIX);
}

// Os saldos moraram no alto enquanto a janela era deitada; agora ficam no
// rodape, logo acima de Comprar e Fechar. Em 268 pixels os tres nao cabem por
// extenso, entao o valor vem curto - 1,2M no lugar de 1.257.100.
void PintaSaldos(HDC hdc) {
    const int x = kMargem;
    const int y = kTopoSaldos;
    const int l = kUtil;

    Degrade(hdc, x, y, l, kAltSaldos, RGB(30, 23, 16), RGB(16, 12, 8));
    Contorno(hdc, x, y, l, kAltSaldos, RGB(58, 48, 36));

    // Cada moeda ocupa um terco da faixa, e dentro do terco o conjunto inteiro -
    // losango mais texto - fica centrado. Comecar sempre na mesma margem da
    // esquerda, como antes, deixava os tres empurrados para um lado, porque
    // "Cash 1400" e bem mais curto que "Ouro 999,3M".
    const int largura = l / 3;
    const int kLosangoL = 9;    // o losango de raio 4
    const int kRespiro = 6;     // entre o losango e a palavra
    for (int i = 0; i < 3; ++i) {
        const int cx = x + i * largura;
        char valor[40];
        Curto(LojaRedeSaldo(i), valor, sizeof(valor));
        char texto[64];
        sprintf_s(texto, "%s %s", kNomeMoeda[i], valor);

        SelectObject(hdc, g_medio);
        SIZE medida = {0, 0};
        GetTextExtentPoint32A(hdc, texto, static_cast<int>(strlen(texto)), &medida);
        const int conjunto = kLosangoL + kRespiro + medida.cx;
        int inicio = cx + (largura - conjunto) / 2;
        if (inicio < cx + 2) {
            inicio = cx + 2;
        }

        Losango(hdc, inicio + kLosangoL / 2, y + kAltSaldos / 2, 4, kCorMoeda[i]);
        SetTextColor(hdc, kCorMoeda[i]);
        RECT rt = {inicio + kLosangoL + kRespiro, y, cx + largura, y + kAltSaldos};
        DrawTextA(hdc, texto, -1, &rt, DT_LEFT | DT_VCENTER | DT_SINGLELINE | DT_NOPREFIX);
    }
}

void PintaMenu(HDC hdc) {
    for (int i = 0; i < kMenuTotal; ++i) {
        const RECT r = AreaMenu(i);
        char texto[32];
        sprintf_s(texto, "%s", kMenu[i]);
        const bool ativo = (i == g_filtro) && i <= kMenuMeus;
        LinhaBotao(hdc, r.left, r.top, r.right - r.left, kAltBotao, ativo);
        // A fileira de baixo carrega os nomes compridos - "Realizar saque" -,
        // que so cabem na fonte miuda.
        const bool comprido = i >= kMenuLinha1;
        SelectObject(hdc, ativo ? (comprido ? g_fonte : g_negrito) : (comprido ? g_miudo : g_fonte));
        SetTextColor(hdc, ativo ? kTextoAtivo : kTexto);
        RECT rt = {r.left + 3, r.top, r.right - 3, r.bottom};
        DrawTextA(hdc, texto, -1, &rt, DT_CENTER | DT_VCENTER | DT_SINGLELINE | DT_NOPREFIX);
    }
}

// O desenho do item, tirado das folhas do proprio cliente. Ele nao passa pelo
// GDI: e escrito direto nos pixels do DIB, entao o que o GDI ja pintou precisa
// estar no lugar - dai o GdiFlush. Item sem icone volta ao losango na cor da
// moeda, que era o que a loja desenhava antes.
void DesenhaIcone(HDC hdc, const RECT& r, int item, COLORREF corMoeda, int lado) {
    GdiFlush();
    if (IconeDesenha(g_tela.pixels, g_tela.l, g_tela.a, r.left, r.top, lado, lado, item) == 0) {
        Losango(hdc, r.left + lado / 2, r.top + lado / 2, 9, corMoeda);
    }
}

// O quadrado do item: moldura de encaixe, um losango na cor da moeda no lugar
// do desenho do item (que virá das texturas do cliente) e o preco curto embaixo.
void PintaSlot(HDC hdc, const RECT& r, const LojaOferta* o, bool escolhido) {
    Degrade(hdc, r.left, r.top, kSlot, kSlot, RGB(28, 22, 16), RGB(14, 11, 8));
    Contorno(hdc, r.left, r.top, kSlot, kSlot, escolhido ? kCanto : RGB(64, 53, 39));
    if (o == nullptr) {
        return;
    }
    DesenhaIcone(hdc, r, o->indice, kCorMoeda[o->moeda % 3], kSlot);
    // A moeda precisa estar no quadrado, nao so na cor do preco: enquanto o
    // desenho do item era um losango colorido isso se via de longe, mas com o
    // icone de verdade a cor sumiu. Uma marca no canto resolve, e a borda toma
    // o tom da moeda.
    Contorno(hdc, r.left + 1, r.top + 1, kSlot - 2, kSlot - 2, kCorMoeda[o->moeda % 3]);
    Losango(hdc, r.left + 6, r.top + kSlot - 8, 4, kCorMoeda[o->moeda % 3]);
    if (o->perto == 0) {
        // Fora do alcance de compra: a vitrine junta a cidade toda, mas levar
        // exige chegar perto da barraca.
        Contorno(hdc, r.left + 1, r.top + 1, kSlot - 2, kSlot - 2, RGB(90, 60, 40));
    }
    if (o->refino > 0) {
        char ref[8];
        sprintf_s(ref, "+%d", o->refino);
        SelectObject(hdc, g_miudo);
        SetTextColor(hdc, kCanto);
        RECT rr = {r.left + 2, r.top + 1, r.left + kSlot - 2, r.top + 12};
        DrawTextA(hdc, ref, -1, &rr, DT_LEFT | DT_TOP | DT_SINGLELINE | DT_NOPREFIX);
    }
    if (o->qtd > 1) {
        char qtd[8];
        sprintf_s(qtd, "%d", o->qtd);
        SelectObject(hdc, g_miudo);
        SetTextColor(hdc, RGB(255, 255, 255));
        RECT rq = {r.left + 2, r.top + 1, r.left + kSlot - 3, r.top + 12};
        DrawTextA(hdc, qtd, -1, &rq, DT_RIGHT | DT_TOP | DT_SINGLELINE | DT_NOPREFIX);
    }
    char preco[16];
    Curto(o->preco, preco, sizeof(preco));
    SelectObject(hdc, g_miudo);
    SetTextColor(hdc, o->perto ? kCorMoeda[o->moeda % 3] : kTextoFraco);
    RECT rp = {r.left + 1, r.top + kSlot - 14, r.right - 1, r.bottom - 2};
    DrawTextA(hdc, preco, -1, &rp, DT_CENTER | DT_BOTTOM | DT_SINGLELINE | DT_NOPREFIX);
}

// Um item do cofre na grade da montagem. O que ja esta numa prateleira aparece
// com a marca da moeda escolhida, para nao entrar duas vezes.
void PintaSlotCofre(HDC hdc, const RECT& r, const LojaItemCofre* it, bool escolhido,
                    int prateleira, int lado) {
    Degrade(hdc, r.left, r.top, lado, lado, RGB(28, 22, 16), RGB(14, 11, 8));
    Contorno(hdc, r.left, r.top, lado, lado, escolhido ? kCanto : RGB(64, 53, 39));
    if (it == nullptr) {
        return;
    }
    const bool naBarraca = prateleira >= 0;
    DesenhaIcone(hdc, r, it->indice,
                 naBarraca ? kCorMoeda[g_prateleiras[prateleira].moeda % 3] : RGB(120, 104, 78),
                 lado);
    if (naBarraca) {
        const COLORREF cor = kCorMoeda[g_prateleiras[prateleira].moeda % 3];
        Contorno(hdc, r.left + 1, r.top + 1, lado - 2, lado - 2, cor);
        Losango(hdc, r.left + 6, r.top + lado - 8, 4, cor);
    }
    if (it->refino > 0) {
        char ref[8];
        sprintf_s(ref, "+%d", it->refino);
        SelectObject(hdc, g_miudo);
        SetTextColor(hdc, kCanto);
        RECT rr = {r.left + 2, r.top + 1, r.left + lado - 2, r.top + 12};
        DrawTextA(hdc, ref, -1, &rr, DT_LEFT | DT_TOP | DT_SINGLELINE | DT_NOPREFIX);
    }
    if (it->qtd > 1) {
        char qtd[8];
        sprintf_s(qtd, "%d", it->qtd);
        SelectObject(hdc, g_miudo);
        SetTextColor(hdc, RGB(255, 255, 255));
        RECT rq = {r.left + 2, r.top + 1, r.left + lado - 3, r.top + 12};
        DrawTextA(hdc, qtd, -1, &rq, DT_RIGHT | DT_TOP | DT_SINGLELINE | DT_NOPREFIX);
    }
    // Embaixo so aparece preco: o da prateleira, se o item ja entrou, ou o que
    // esta sendo digitado nos botoes, no quadrado escolhido. O numero do item
    // era coisa de depuracao - quem olha a barraca quer o desenho e a
    // quantidade, como em qualquer janela do jogo.
    const bool mostraEdicao = false;
    if (naBarraca || mostraEdicao) {
        char rodape[16];
        Curto(naBarraca ? g_prateleiras[prateleira].preco : g_precoEdicao, rodape,
              sizeof(rodape));
        SelectObject(hdc, g_miudo);
        SetTextColor(hdc, kCorMoeda[(naBarraca ? g_prateleiras[prateleira].moeda
                                               : g_moedaEdicao) % 3]);
        RECT rp = {r.left + 1, r.top + lado - 14, r.right - 1, r.bottom - 2};
        DrawTextA(hdc, rodape, -1, &rp, DT_CENTER | DT_BOTTOM | DT_SINGLELINE | DT_NOPREFIX);
    }
}

// Em que prateleira este slot do cofre ja esta, ou -1.
int PrateleiraDoSlot(int cargoPos) {
    for (int i = 0; i < g_prateleirasUsadas; ++i) {
        if (g_prateleiras[i].cargoPos == cargoPos) {
            return i;
        }
    }
    return -1;
}

// O item do cofre que ocupa esta posicao - e assim que a prateleira acha o
// desenho e a quantidade do que ela vende.
const LojaItemCofre* ItemDoSlot(int cargoPos) {
    for (int i = 0; i < LojaRedeCofreQtd(); ++i) {
        const LojaItemCofre* it = LojaRedeCofreItem(i);
        if (it != nullptr && it->slot == cargoPos) {
            return it;
        }
    }
    return nullptr;
}

// --- a tela de montagem -----------------------------------------------------
//
// Em cima o cofre da conta inteiro - as quatro abas do banco numa lista so, que
// rola -, embaixo a previa da lojinha com as doze prateleiras, e no rodape so o
// "Abrir loja". Um item entra na previa pela caixa de preco, que abre com dois
// cliques nele; sai dela com um clique.
void PintaMontagemTela(HDC hdc) {
    const RECT v = AreaVoltar();
    LinhaBotao(hdc, v.left, v.top, v.right - v.left, kAltBotao, false);
    SelectObject(hdc, g_fonte);
    SetTextColor(hdc, kTexto);
    RECT rv = {v.left, v.top, v.right, v.bottom};
    DrawTextA(hdc, "Voltar", -1, &rv, DT_CENTER | DT_VCENTER | DT_SINGLELINE | DT_NOPREFIX);

    // A dica ocupa o resto da fileira do "Voltar", centrada nela: encostada na
    // direita ela passava da borda e ficava miuda ao lado do botao.
    SelectObject(hdc, g_fonte);
    SetTextColor(hdc, kTextoFraco);
    RECT rd = {v.right + 6, v.top, kMargem + kUtil, v.bottom};
    DrawTextA(hdc, "dois cliques no item para vende-lo", -1, &rd,
              DT_CENTER | DT_VCENTER | DT_SINGLELINE | DT_NOPREFIX);

    const int base = g_cofreRolagem * kCofreColunas;
    for (int i = 0; i < CofreVisiveis(); ++i) {
        const RECT r = AreaSlotCofre(i);
        const int qual = base + i;
        const LojaItemCofre* it = LojaRedeCofreItem(qual);
        PintaSlotCofre(hdc, r, it, it != nullptr && qual == g_cofreEscolhido,
                       it != nullptr ? PrateleiraDoSlot(it->slot) : -1, kCofreSlot);
    }

    // A barra de rolagem so ganha polegar quando ha o que rolar; sem isso ela
    // mentiria dizendo que o cofre continua para baixo.
    const RECT t = AreaTrilho();
    Degrade(hdc, t.left, t.top, t.right - t.left, t.bottom - t.top, RGB(16, 13, 9),
            RGB(10, 8, 6));
    Contorno(hdc, t.left, t.top, t.right - t.left, t.bottom - t.top, RGB(58, 48, 36));
    if (RolagemMax() > 0) {
        const RECT p = AreaPolegar();
        Degrade(hdc, p.left + 1, p.top, kBarraRolagem - 2, p.bottom - p.top, RGB(107, 46, 5),
                RGB(41, 13, 2));
        Contorno(hdc, p.left + 1, p.top, kBarraRolagem - 2, p.bottom - p.top, kBtnBordaTopo);
    }

    char rotulo[64];
    sprintf_s(rotulo, "Sua lojinha   %d/%d", g_prateleirasUsadas, kMaxPrateleiras);
    SelectObject(hdc, g_fonte);
    SetTextColor(hdc, kTexto);
    RECT rr = {kMargem, kTopoRotuloPrat, kMargem + kUtil, kTopoRotuloPrat + kAltRotuloPrat};
    DrawTextA(hdc, rotulo, -1, &rr, DT_CENTER | DT_VCENTER | DT_SINGLELINE | DT_NOPREFIX);

    for (int i = 0; i < kMaxPrateleiras; ++i) {
        const RECT r = AreaSlotPrateleira(i);
        const bool tem = i < g_prateleirasUsadas && g_prateleiras[i].cargoPos >= 0;
        const LojaItemCofre* it = tem ? ItemDoSlot(g_prateleiras[i].cargoPos) : nullptr;
        PintaSlotCofre(hdc, r, it, false, tem ? i : -1, kPrateleiraSlot);
    }

    const RECT a = AreaAbrirLoja();
    const bool pode = g_prateleirasUsadas > 0;
    LinhaBotao(hdc, a.left, a.top, a.right - a.left, kAltAbrir, pode);
    SelectObject(hdc, g_grande);
    SetTextColor(hdc, pode ? kTextoAtivo : kTextoFraco);
    RECT ra = {a.left, a.top, a.right, a.bottom};
    DrawTextA(hdc, "Abrir loja", -1, &ra, DT_CENTER | DT_VCENTER | DT_SINGLELINE | DT_NOPREFIX);
}

// --- as caixas que perguntam ------------------------------------------------
//
// O painel atras escurece porque a caixa e modal de verdade: enquanto ela
// estiver aberta nenhum clique chega ao que esta embaixo, e mostrar isso e mais
// honesto do que deixar o painel aceso e engolir os cliques em silencio.
void Escurece() {
    GdiFlush();
    BYTE* tela = static_cast<BYTE*>(g_tela.pixels);
    if (tela == nullptr) {
        return;
    }
    for (int y = 0; y < g_tela.a; ++y) {
        BYTE* linha = tela + static_cast<size_t>(y) * g_tela.l * 4;
        for (int x = 0; x < g_tela.l; ++x) {
            BYTE* p = linha + static_cast<size_t>(x) * 4;
            p[0] = static_cast<BYTE>(p[0] * 40 / 100);
            p[1] = static_cast<BYTE>(p[1] * 40 / 100);
            p[2] = static_cast<BYTE>(p[2] * 40 / 100);
        }
    }
}

// O campo de digitar: o mesmo desenho nas duas caixas, com o cursor piscando no
// fim do que ja foi escrito.
void PintaCampoCaixa(HDC hdc, const char* texto, bool cheio, COLORREF cor) {
    const RECT r = AreaCaixaCampo();
    const int l = r.right - r.left;
    const int a = r.bottom - r.top;
    Degrade(hdc, r.left, r.top, l, a, RGB(30, 23, 16), RGB(16, 12, 8));
    Contorno(hdc, r.left, r.top, l, a, kCanto);
    char comCursor[64];
    sprintf_s(comCursor, "%s%s", texto, (GetTickCount() / 500) % 2 == 0 ? "|" : " ");
    SelectObject(hdc, cheio ? g_negrito : g_miudo);
    SetTextColor(hdc, cheio ? cor : kTextoFraco);
    RECT rt = {r.left + 8, r.top, r.right - 6, r.bottom};
    DrawTextA(hdc, comCursor, -1, &rt, DT_LEFT | DT_VCENTER | DT_SINGLELINE | DT_NOPREFIX);
}

void PintaBotaoCaixa(HDC hdc, int qual, const char* texto, bool aceso) {
    const RECT r = AreaCaixaBotao(qual);
    LinhaBotao(hdc, r.left, r.top, r.right - r.left, kAltCaixaBotao, aceso);
    SelectObject(hdc, aceso ? g_negrito : g_fonte);
    SetTextColor(hdc, aceso ? kTextoAtivo : kTexto);
    RECT rt = {r.left, r.top, r.right, r.bottom};
    DrawTextA(hdc, texto, -1, &rt, DT_CENTER | DT_VCENTER | DT_SINGLELINE | DT_NOPREFIX);
}

void PintaCaixa(HDC hdc) {
    if (g_caixa == kCaixaNenhuma) {
        return;
    }
    Escurece();

    const RECT p = AreaCaixa();
    const int L = p.right - p.left;
    const int A = p.bottom - p.top;
    Contorno(hdc, p.left, p.top, L, A, kBorda1);
    Contorno(hdc, p.left + 1, p.top + 1, L - 2, A - 2, kBorda2);
    Degrade(hdc, p.left + 2, p.top + 2, L - 4, A - 4, kFundoTopo, kFundoBaixo);
    Contorno(hdc, p.left + 2, p.top + 2, L - 4, A - 4, kBordaInterna);

    Degrade(hdc, p.left + 4, p.top + 4, L - 8, kAltCaixaCab, kCabTopo, kCabBaixo);
    Contorno(hdc, p.left + 4, p.top + 4, L - 8, kAltCaixaCab, kCabBorda);
    Losango(hdc, p.left + 13, p.top + 4 + kAltCaixaCab / 2, 3, kLosango);
    SelectObject(hdc, g_negrito);
    SetTextColor(hdc, RGB(255, 255, 255));
    RECT rc = {p.left + 22, p.top + 4, p.right - 8, p.top + 4 + kAltCaixaCab};
    DrawTextA(hdc, g_caixa == kCaixaNome ? "Nome da Loja" : "Pre\xE7o do item", -1, &rc,
              DT_LEFT | DT_VCENTER | DT_SINGLELINE | DT_NOPREFIX);

    SelectObject(hdc, g_miudo);
    SetTextColor(hdc, kTextoFraco);

    if (g_caixa == kCaixaNome) {
        RECT rl = {p.left + 10, p.top + 28, p.right - 10, p.top + 44};
        DrawTextA(hdc, "Como a sua loja vai se chamar:", -1, &rl,
                  DT_LEFT | DT_VCENTER | DT_SINGLELINE | DT_NOPREFIX);
        PintaCampoCaixa(hdc, g_nomeTam > 0 ? g_nomeBarraca : "", g_nomeTam > 0, kTexto);
        PintaBotaoCaixa(hdc, 0, "Voltar", false);
        PintaBotaoCaixa(hdc, 1, "Prosseguir", g_nomeTam > 0);
        return;
    }

    RECT rm = {p.left + 10, p.top + 28, p.right - 10, p.top + 44};
    DrawTextA(hdc, "Em que moeda:", -1, &rm, DT_LEFT | DT_VCENTER | DT_SINGLELINE | DT_NOPREFIX);
    for (int i = 0; i < 3; ++i) {
        const RECT r = AreaCaixaMoeda(i);
        const bool aceso = i == g_moedaEdicao;
        LinhaBotao(hdc, r.left, r.top, r.right - r.left, r.bottom - r.top, aceso);
        SelectObject(hdc, aceso ? g_negrito : g_fonte);
        SetTextColor(hdc, aceso ? kCorMoeda[i] : kTextoFraco);
        RECT rt = {r.left, r.top, r.right, r.bottom};
        DrawTextA(hdc, kNomeMoeda[i], -1, &rt,
                  DT_CENTER | DT_VCENTER | DT_SINGLELINE | DT_NOPREFIX);
    }
    SelectObject(hdc, g_miudo);
    SetTextColor(hdc, kTextoFraco);
    RECT rp = {p.left + 10, p.top + 70, p.right - 10, p.top + 84};
    DrawTextA(hdc, "Por quanto:", -1, &rp, DT_LEFT | DT_VCENTER | DT_SINGLELINE | DT_NOPREFIX);
    char valor[32];
    Pontuado(g_precoEdicao, valor, sizeof(valor));
    PintaCampoCaixa(hdc, g_precoEdicao > 0 ? valor : "", g_precoEdicao > 0,
                    kCorMoeda[g_moedaEdicao % 3]);
    PintaBotaoCaixa(hdc, 0, "Cancelar", false);
    PintaBotaoCaixa(hdc, 1, "Confirmar", g_precoEdicao > 0);
}

// A grade da vitrine. A tela de montagem tem a sua propria, com medidas
// proprias - ver PintaMontagemTela.
void PintaGrade(HDC hdc) {
    for (int i = 0; i < kPorPagina; ++i) {
        const RECT r = AreaSlot(i);
        const LojaOferta* o = LojaRedeOferta(i);
        PintaSlot(hdc, r, o, o != nullptr && i == g_escolhido);
    }
}

// A linha de detalhe so existe na vitrine, onde ela diz o estado da busca
// ("nenhuma barraca aberta na cidade") e o que o comprador escolheu. Na tela de
// montagem ela foi embora: o preco em edicao aparece no proprio quadrado, e o
// resto era numero de depuracao.
void PintaDetalhe(HDC hdc) {
    if (Montando()) {
        return;
    }
    const int x = kMargem;
    const int y = kTopoDetalhe;
    const int l = kUtil;
    Barra(hdc, x, y, l, 1, RGB(58, 48, 36));

    char texto[160];
    const LojaOferta* esc = LojaRedeOferta(g_escolhido);
    if (esc != nullptr) {
        char valor[40];
        Pontuado(esc->preco, valor, sizeof(valor));
        sprintf_s(texto, "%s %s   %s%s", valor, kNomeMoeda[esc->moeda % 3], esc->nome,
                  esc->perto ? "" : "   (longe)");
        SelectObject(hdc, g_negrito);
        SetTextColor(hdc, esc->perto ? kCorMoeda[esc->moeda % 3] : kTextoFraco);
    } else if (!LojaRedeRespondeu()) {
        sprintf_s(texto, "falando com o servidor...");
        SelectObject(hdc, g_fonte);
        SetTextColor(hdc, kTextoFraco);
    } else if (LojaRedeTotal() == 0) {
        sprintf_s(texto, "nenhuma barraca aberta na cidade");
        SelectObject(hdc, g_fonte);
        SetTextColor(hdc, kTextoFraco);
    } else {
        sprintf_s(texto, "escolha um item para ver preco e vendedor");
        SelectObject(hdc, g_fonte);
        SetTextColor(hdc, kTextoFraco);
    }
    RECT rt = {x + 4, y + 2, x + l - 4, y + kAltDetalhe};
    DrawTextA(hdc, texto, -1, &rt, DT_LEFT | DT_VCENTER | DT_SINGLELINE | DT_NOPREFIX);
}

// O campo do nome, na faixa que a linha de detalhe deixou livre. Nao ha cursor
// piscando: a barra no fim do texto ja diz onde a proxima letra entra.
// Os dois campos que se digita na montagem, lado a lado: o nome da barraca e o
// preco do item escolhido. Um clique escolhe qual recebe o teclado; a barra no
// fim do texto diz qual e.
// A oferta e minha? O servidor manda o id da barraca em cada oferta, e o id da
// nossa veio quando ela subiu - entao da para saber sem comparar nomes, que
// dois personagens podem repetir.
bool OfertaMinha(const LojaOferta* o) {
    const int minha = LojaRedeBarracaAberta();
    return o != nullptr && minha != 0 && o->vendedor == minha;
}

void PintaRodape(HDC hdc) {
    const int y = TopoRodape();

    const RECT esq = AreaSeta(false);
    const RECT dir = AreaSeta(true);
    LinhaBotao(hdc, esq.left, esq.top, esq.right - esq.left, kAltRodape, false);
    LinhaBotao(hdc, dir.left, dir.top, dir.right - dir.left, kAltRodape, false);
    SelectObject(hdc, g_negrito);
    SetTextColor(hdc, kTexto);
    RECT re = {esq.left, esq.top, esq.right, esq.bottom};
    DrawTextA(hdc, "<", -1, &re, DT_CENTER | DT_VCENTER | DT_SINGLELINE | DT_NOPREFIX);
    RECT rd = {dir.left, dir.top, dir.right, dir.bottom};
    DrawTextA(hdc, ">", -1, &rd, DT_CENTER | DT_VCENTER | DT_SINGLELINE | DT_NOPREFIX);

    char pag[16];
    sprintf_s(pag, "%d/%d", g_pagina + 1, Paginas());
    SelectObject(hdc, g_fonte);
    SetTextColor(hdc, kTexto);
    RECT rp = {esq.right, y, dir.left, y + kAltRodape};
    DrawTextA(hdc, pag, -1, &rp, DT_CENTER | DT_VCENTER | DT_SINGLELINE | DT_NOPREFIX);

    // Comprar so acende com uma oferta escolhida que esteja ao alcance: a vitrine
    // junta a cidade, mas levar o item exige estar perto da barraca.
    const LojaOferta* esc = LojaRedeOferta(g_escolhido);
    // O que e da minha barraca aparece na vitrine como o de todo mundo - ela e
    // do servidor inteiro -, mas comprar de si mesmo nao existe. O botao apaga,
    // como a porta da lojinha quando ja ha uma de pe.
    // Em "Meus itens" o mesmo botao e "Trocar moeda", e ali TODA oferta e minha:
    // apagar por ser minha desligaria justamente a unica coisa que aquela tela
    // faz.
    const bool minha = g_filtro != kMenuMeus && OfertaMinha(esc);
    const bool podeComprar =
        esc != nullptr && esc->perto != 0 && g_filtro != kMenuMeus && !minha;
    const RECT c = AreaBotaoComprar();
    LinhaBotao(hdc, c.left, c.top, c.right - c.left, kAltRodape, podeComprar);
    SelectObject(hdc, podeComprar ? g_negrito : g_fonte);
    SetTextColor(hdc, podeComprar ? kTextoAtivo : kTextoFraco);
    RECT rc = {c.left, c.top, c.right, c.bottom};
    DrawTextA(hdc, g_filtro == kMenuMeus ? "Trocar moeda" : "Comprar", -1, &rc,
              DT_CENTER | DT_VCENTER | DT_SINGLELINE | DT_NOPREFIX);
    if (minha) {
        Apaga(c);
    }

    const RECT f = AreaBotaoFechar();
    LinhaBotao(hdc, f.left, f.top, f.right - f.left, kAltRodape, false);
    SelectObject(hdc, g_fonte);
    SetTextColor(hdc, kTexto);
    RECT rf = {f.left, f.top, f.right, f.bottom};
    DrawTextA(hdc, "Fechar", -1, &rf, DT_CENTER | DT_VCENTER | DT_SINGLELINE | DT_NOPREFIX);
}

void Pinta(HDC hdc) {
    SetBkMode(hdc, TRANSPARENT);
    Moldura(&g_tela);
    PintaCabecalho(hdc);
    PintaSaldos(hdc);
    if (g_qualTela == kTelaMenu) {
        PintaMenuInicial(hdc);
    } else if (Montando()) {
        PintaMontagemTela(hdc);
    } else {
        PintaMenu(hdc);
        PintaGrade(hdc);
        PintaDetalhe(hdc);
        PintaRodape(hdc);
    }
    // Por ultimo, sempre: a caixa que pergunta fica por cima de qualquer tela.
    PintaCaixa(hdc);
}

void Repinta() {
    CriaFontes();
    if (!TelaGarante(&g_tela, kLargura, kAltura)) {
        return;
    }
    Pinta(g_tela.dc);
    // Opaca de proposito: a loja e uma janela do jogo, nao uma sobreposicao.
    // O painel de alvos e translucido porque fica sobre a cena e precisa
    // deixar ver o que esta atras; aqui o que esta atras nao deve aparecer
    // nem ser clicavel.
    TelaFecha(&g_tela, 255);
    ++g_versao;
}

// --- o icone na barra ------------------------------------------------------
//
// Os icones do jogo sao arte solta sobre a celula, sem moldura e sem fundo
// proprio: a primeira versao daqui era um quadrado escuro com borda, e ficava
// obvio que nao pertencia a barra. Agora a barraca e desenhada sobre uma cor de
// chave que vira transparente no fim, e so ela aparece.
void IconeCanto(int telaL, int telaA, int* x, int* y) {
    *x = telaL / 2 + g_iconeDx;
    *y = telaA - g_iconeDy;
}

// --- a barra de icones esta aberta? ----------------------------------------
//
// Antes isto era adivinhado lendo a cor de quatro pontos do quadro anterior.
// Agora quem responde e o proprio desenho do cliente: toda peca de interface
// passa por AppendNode (0x40C43D, documentado no gamepatch.cpp), que recebe o no
// com o retangulo ja em coordenadas de tela em +0x04. Se num quadro passou por
// ali um no do tamanho da faixa de icones, cobrindo a celula da Loja Pessoal, a
// barra esta aberta.

constexpr DWORD kAppendNode = 0x0040C43D;
const BYTE kAppendNodeBytes[7] = {0x55, 0x8B, 0xEC, 0x83, 0x7D, 0x10, 0x1E};
constexpr DWORD kNoRetangulo = 0x04;

DWORD g_faixaVistaEm = 0;

// A faixa de icones, como o cliente a desenhou neste quadro. Guardada para o
// diagnostico do icone poder dizer onde o clique caiu DENTRO dela.

float g_faixaX = 0.0f;
float g_faixaY = 0.0f;
float g_faixaL = 0.0f;
float g_faixaA = 0.0f;

// --- o pedaco que a dica pinta ---------------------------------------------
//
// A janela da dica e sempre 307x346, mas ela so pinta onde ha texto: o resto e
// transparente. Abrir o vao do painel pelo tamanho da janela deixava o mundo
// aparecendo em volta da caixa - foi o que aconteceu nas duas tentativas.
//
// O tamanho de verdade sai do desenho: enquanto a dica esta de pe, toda peca
// que cai dentro da janela dela e somada num retangulo so. E esse retangulo, e
// nao a janela, que o painel evita.
float g_caixaX = 0.0f;
float g_caixaY = 0.0f;
float g_caixaX2 = 0.0f;
float g_caixaY2 = 0.0f;
float g_caixaJanelaX = -1.0f;
float g_caixaJanelaY = -1.0f;
DWORD g_caixaVistaEm = 0;

// A janela da dica, em coordenadas de tela. Devolve 0 quando nao ha.
bool JanelaDaDica(float* x, float* y, float* l, float* a) {
    if (!g_aberta || g_dicaItem <= 0 || !EmJogo()) {
        return false;
    }
    const DWORD cena = *reinterpret_cast<const DWORD*>(kCena);
    if (cena < 0x10000) {
        return false;
    }
    const DWORD janela = *reinterpret_cast<const DWORD*>(cena + 0x58);
    if (janela < 0x10000) {
        return false;
    }
    const float* r = reinterpret_cast<const float*>(janela + 0x4C);
    if (r[2] < 20.0f || r[3] < 20.0f) {
        return false;
    }
    *x = r[0];
    *y = r[1];
    *l = r[2];
    *a = r[3];
    return true;
}

void SomaNaCaixa(float x, float y, float l, float a) {
    float jx = 0.0f;
    float jy = 0.0f;
    float jl = 0.0f;
    float ja = 0.0f;
    if (!JanelaDaDica(&jx, &jy, &jl, &ja)) {
        return;
    }
    // Peca de fora da janela nao conta; a propria janela tambem nao, que e o
    // retangulo grande e transparente.
    if (x < jx - 2.0f || y < jy - 2.0f || x + l > jx + jl + 2.0f || y + a > jy + ja + 2.0f) {
        return;
    }
    if (l >= jl - 2.0f && a >= ja - 2.0f) {
        return;
    }
    const DWORD agora = GetTickCount();
    const bool mudou = jx != g_caixaJanelaX || jy != g_caixaJanelaY;
    if (mudou || agora - g_caixaVistaEm > 300) {
        g_caixaJanelaX = jx;
        g_caixaJanelaY = jy;
        g_caixaX = x;
        g_caixaY = y;
        g_caixaX2 = x + l;
        g_caixaY2 = y + a;
    } else {
        if (x < g_caixaX) {
            g_caixaX = x;
        }
        if (y < g_caixaY) {
            g_caixaY = y;
        }
        if (x + l > g_caixaX2) {
            g_caixaX2 = x + l;
        }
        if (y + a > g_caixaY2) {
            g_caixaY2 = y + a;
        }
    }
    g_caixaVistaEm = agora;
}

// O vao que o painel deve evitar: o que a dica pintou, com uma folga de dois
// pixels para a borda dela nao ficar cortada.
extern "C" int __cdecl LojaVaoDaDica(int* x, int* y, int* largura, int* altura) {
    float jx = 0.0f;
    float jy = 0.0f;
    float jl = 0.0f;
    float ja = 0.0f;
    if (!JanelaDaDica(&jx, &jy, &jl, &ja)) {
        return 0;
    }
    if (GetTickCount() - g_caixaVistaEm > 300 || g_caixaX2 <= g_caixaX ||
        g_caixaY2 <= g_caixaY) {
        return 0;
    }
    *x = static_cast<int>(g_caixaX) - 2;
    *y = static_cast<int>(g_caixaY) - 2;
    *largura = static_cast<int>(g_caixaX2 - g_caixaX) + 4;
    *altura = static_cast<int>(g_caixaY2 - g_caixaY) + 4;
    return 1;
}

// --- entrar na fila de desenho do cliente -----------------------------------
//
// O painel era desenhado no fim do quadro, depois de tudo - e por isso passava
// por cima da caixa de informacao do item, que e do cliente. Recortar o painel
// no lugar dela foi tentado tres vezes e nunca ficou certo, porque o problema
// nao era o recorte: era a loja estar por cima de tudo, por definicao.
//
// O cliente resolve isso com trinta camadas de desenho (AppendNode, 0x40C43D):
// ele percorre uma de cada vez, em ordem, no laco de 0x4B9507. As janelas ficam
// numa camada, a caixa de informacao numa mais alta - e e so por isso que ela
// aparece por cima do Banco.
//
// Entao o painel entra nessa fila: quando o laco chega na camada da dica, ele e
// desenhado ANTES dela. Fica sobre o mundo e sobre as janelas, e sob a caixa -
// exatamente como as janelas do proprio jogo se comportam.
constexpr DWORD kLacoDasCamadas = 0x004B9519;
constexpr DWORD kLacoVolta = 0x004B9523;
const BYTE kLacoBytes[10] = {0x83, 0x7D, 0xFC, 0x1E, 0x0F, 0x8D, 0x8B, 0x00, 0x00, 0x00};

extern "C" void __cdecl LojaCamadaDoLaco(int camada) {
    if (camada == g_camadaDaDica) {
        D3DDesenhaCamadasAgora();
    }
}

__declspec(naked) void LacoDasCamadasHook() {
    __asm {
        pushad
        pushfd
        mov eax, [ebp - 4]        // a camada da vez
        push eax
        call LojaCamadaDoLaco
        add esp, 4
        popfd
        popad
        cmp dword ptr [ebp - 4], 0x1E   // instrucoes originais
        jge fim
        push kLacoVolta
        ret
    fim:
        push 0x004B95AE
        ret
    }
}

// Sonda da caixa de informacao: com diagnostico=1 e o cursor sobre um item do
// painel, anota o retangulo da janela da dica e o de cada peca desenhada, uma
// vez cada. E o que falta para o painel abrir um vao do tamanho da caixa - as
// duas medidas que eu tentei (o retangulo da janela) sao o espaco maximo dela,
// nao o do desenho.
void DiagCaixaDaDica(float x, float y, float l, float a) {
    if (g_diag == 0 || !g_aberta || g_dicaItem <= 0 || !EmJogo()) {
        return;
    }
    static int anotados = 0;
    if (anotados >= 10) {
        return;
    }
    const DWORD cena = *reinterpret_cast<const DWORD*>(kCena);
    if (cena < 0x10000) {
        return;
    }
    const DWORD janela = *reinterpret_cast<const DWORD*>(cena + 0x58);
    if (janela < 0x10000) {
        return;
    }
    const float* r = reinterpret_cast<const float*>(janela + 0x4C);
    // So pecas grandes: as linhas de texto sao 164x20 e enchiam o registro
    // antes de o fundo da caixa aparecer. O que interessa e o desenho que cobre
    // a caixa, e ele e largo.
    if (l < 150.0f || a < 60.0f) {
        return;
    }
    ++anotados;
    char buf[220];
    sprintf_s(buf,
              "=== diag caixa: no (%.0f,%.0f %.0fx%.0f) | janela da dica (%.0f,%.0f %.0fx%.0f)",
              x, y, l, a, r[0], r[1], r[2], r[3]);
    Log(buf);
}

extern "C" void __cdecl LojaAnotaNo(DWORD no, int camada) {
    if (no < 0x10000) {
        return;
    }
    const float* r = reinterpret_cast<const float*>(no + kNoRetangulo);
    const float x = r[0];
    const float y = r[1];
    const float l = r[2];
    const float a = r[3];
    // A peca da dica tem o tamanho da janela dela: e dela que sai a camada.
    float jx = 0.0f;
    float jy = 0.0f;
    float jl = 0.0f;
    float ja = 0.0f;
    if (JanelaDaDica(&jx, &jy, &jl, &ja) && l >= jl - 2.0f && a >= ja - 2.0f &&
        x >= jx - 2.0f && x <= jx + 2.0f && y >= jy - 2.0f && y <= jy + 2.0f) {
        g_camadaDaDica = camada;
    }
    DiagCaixaDaDica(x, y, l, a);
    if (a < 30.0f || a > 48.0f || l < 120.0f || l > 600.0f) {
        return;
    }
    int cx = 0;
    int cy = 0;
    IconeCanto(CamadaTelaL(), CamadaTelaA(), &cx, &cy);
    const float esq = static_cast<float>(cx);
    const float dir = static_cast<float>(cx + g_iconeL);
    const float topo = static_cast<float>(cy);
    if (x <= esq && x + l >= dir && y <= topo + 2.0f && y + a >= topo + 2.0f) {
        g_faixaVistaEm = GetTickCount();
        g_faixaX = x;
        g_faixaY = y;
        g_faixaL = l;
        g_faixaA = a;
    }
}

// Toda troca da janela ativa do cliente, para descobrir o id de uma janela - a
// da engrenagem, por exemplo.
void DiagJanelaAtiva() {
    if (g_diag == 0 || !EmJogo()) {
        return;
    }
    static WORD ultima = 0;
    const WORD agora = *reinterpret_cast<volatile WORD*>(kIdJanelaAtiva);
    if (agora == ultima) {
        return;
    }
    ultima = agora;
    char buf[80];
    sprintf_s(buf, "=== diag ativa: %04X", agora);
    Log(buf);
}

bool BarraAberta() {
    // Meio segundo, e nao 200 ms: a janela e contada em tempo de relogio, e um
    // quadro lento (o jogo carregando alguma coisa, ou nos escrevendo no disco)
    // fazia a barra "fechar" sozinha - e ai o clique no icone caia no jogo, que
    // abria a lojinha antiga.
    const DWORD agora = GetTickCount();
    return g_faixaVistaEm != 0 && agora - g_faixaVistaEm < 500;
}

// --- cliques ---------------------------------------------------------------
// Guarda o item escolhido numa prateleira, ou atualiza a que ele ja ocupa.
void IncluiNaBarraca() {   // o "Confirmar" da caixa de preco
    const LojaItemCofre* it = LojaRedeCofreItem(g_cofreEscolhido);
    if (it == nullptr || g_precoEdicao <= 0) {
        if (g_diag != 0) {
            char buf[140];
            sprintf_s(buf, "=== diag incluir: recusado (item %s, preco %d)",
                      it == nullptr ? "nao escolhido" : "ok", g_precoEdicao);
            Log(buf);
        }
        return;
    }
    int onde = PrateleiraDoSlot(it->slot);
    if (onde < 0) {
        if (g_prateleirasUsadas >= kMaxPrateleiras) {
            return;
        }
        onde = g_prateleirasUsadas++;
    }
    g_prateleiras[onde].cargoPos = static_cast<signed char>(it->slot);
    g_prateleiras[onde].moeda = static_cast<unsigned char>(g_moedaEdicao);
    g_prateleiras[onde].preco = g_precoEdicao;
    if (g_diag != 0) {
        char buf[140];
        sprintf_s(buf, "=== diag incluir: prateleira %d, item %d, preco %d, moeda %d", onde,
                  it->indice, g_precoEdicao, g_moedaEdicao);
        Log(buf);
    }
}

// Definida adiante, junto com os outros botoes das caixas: o Enter aqui e o
// mesmo que apertar "Prosseguir" ou "Confirmar".
void ConfirmaCaixa();

// O teclado enquanto uma caixa esta aberta: cada caractere vai para o campo
// dela. Devolve 1 quando consumiu - e ai o jogo nao ve a tecla, que senao
// viraria atalho de magia ou linha de chat.
//
// A tela de montagem tambem engole o teclado mesmo sem caixa nenhuma: ela
// ocupa a tela toda do painel, e uma tecla solta abrindo o chat por tras dela
// seria a mesma surpresa.
extern "C" int __cdecl LojaDigita(int c) {
    if (!g_aberta) {
        return 0;
    }
    if (g_caixa == kCaixaPreco) {
        // So numero, digitado da esquerda para a direita como em qualquer campo
        // de valor.
        if (c == 13) {
            ConfirmaCaixa();
            return 1;
        }
        if (c == 8) {
            g_precoEdicao /= 10;
            return 1;
        }
        if (c >= '0' && c <= '9') {
            const long long novo = static_cast<long long>(g_precoEdicao) * 10 + (c - '0');
            if (novo <= kPrecoMax) {
                g_precoEdicao = static_cast<int>(novo);
            }
        }
        return 1;
    }
    if (g_caixa == kCaixaNome) {
        if (c == 13) {
            ConfirmaCaixa();
            return 1;
        }
        if (c == 8) {
            if (g_nomeTam > 0) {
                g_nomeBarraca[--g_nomeTam] = 0;
            }
            return 1;
        }
        if (c < 32 || c > 126) {
            return 1;
        }
        if (g_nomeTam < static_cast<int>(sizeof(g_nomeBarraca)) - 1) {
            g_nomeBarraca[g_nomeTam++] = static_cast<char>(c);
            g_nomeBarraca[g_nomeTam] = 0;
        }
        return 1;
    }
    return Montando() ? 1 : 0;
}

// Chamada depois que a caixa do nome fecha: o nome ja veio de la, e por isso e
// a unica coisa que nao se limpa aqui.
void EntraNaMontagem() {
    g_qualTela = kTelaMontagem;
    g_cofreRolagem = 0;
    g_cofreEscolhido = -1;
    g_cliqueQual = -1;
    g_precoEdicao = 0;
    g_moedaEdicao = kOuro;
    g_prateleirasUsadas = 0;
    for (int i = 0; i < kMaxPrateleiras; ++i) {
        g_prateleiras[i].cargoPos = -1;
        g_prateleiras[i].moeda = 0;
        g_prateleiras[i].preco = 0;
    }
    LojaRedePedeCofre();
}

void SaiDaMontagem() {
    g_qualTela = kTelaMenu;
    g_caixa = kCaixaNenhuma;
    g_escolhido = -1;
    PedeAoServidor();
}

// --- as caixas que perguntam: o que os botoes fazem --------------------------
bool Dentro(const RECT& r, int x, int y) {
    return x >= r.left && x < r.right && y >= r.top && y < r.bottom;
}

void AbreCaixaNome() {
    g_nomeBarraca[0] = 0;
    g_nomeTam = 0;
    g_caixa = kCaixaNome;
}

void AbreCaixaPreco(int qualNoCofre) {
    const LojaItemCofre* it = LojaRedeCofreItem(qualNoCofre);
    if (it == nullptr) {
        return;
    }
    g_cofreEscolhido = qualNoCofre;
    const int onde = PrateleiraDoSlot(it->slot);
    if (onde >= 0) {
        // Item que ja esta na barraca volta com o preco e a moeda dele, para dar
        // para corrigir sem comecar de novo.
        g_precoEdicao = g_prateleiras[onde].preco;
        g_moedaEdicao = g_prateleiras[onde].moeda;
    } else {
        g_precoEdicao = 0;
        g_moedaEdicao = kOuro;
    }
    g_caixa = kCaixaPreco;
}

void ConfirmaCaixa() {
    if (g_caixa == kCaixaNome) {
        if (g_nomeTam <= 0) {
            return;
        }
        g_caixa = kCaixaNenhuma;
        EntraNaMontagem();
        return;
    }
    if (g_caixa != kCaixaPreco || g_precoEdicao <= 0) {
        return;
    }
    IncluiNaBarraca();
    g_caixa = kCaixaNenhuma;
    g_cofreEscolhido = -1;
}

void CliqueCaixa(int x, int y) {
    if (g_caixa == kCaixaPreco) {
        for (int i = 0; i < 3; ++i) {
            if (Dentro(AreaCaixaMoeda(i), x, y)) {
                g_moedaEdicao = i;
                return;
            }
        }
    }
    if (Dentro(AreaCaixaBotao(0), x, y)) {   // Voltar, Cancelar
        g_caixa = kCaixaNenhuma;
        return;
    }
    if (Dentro(AreaCaixaBotao(1), x, y)) {   // Prosseguir, Confirmar
        ConfirmaCaixa();
    }
    // O resto do clique morre aqui: a caixa e modal.
}

// Dois cliques no mesmo quadrado, com meio segundo entre eles. E o gesto que
// abre a caixa do preco - clicar uma vez so escolhe, como em qualquer janela.
bool DoisCliques(int qual) {
    const DWORD agora = GetTickCount();
    const bool sim = qual == g_cliqueQual && agora - g_cliqueEm < 500;
    g_cliqueEm = agora;
    g_cliqueQual = sim ? -1 : qual;
    return sim;
}

void Rola(int linhas) {
    g_cofreRolagem += linhas;
    const int max = RolagemMax();
    if (g_cofreRolagem > max) {
        g_cofreRolagem = max;
    }
    if (g_cofreRolagem < 0) {
        g_cofreRolagem = 0;
    }
}

// Cliques da tela de montagem: o cofre em cima, a previa embaixo, e o "Abrir
// loja" no rodape.
void CliqueMontagem(int x, int y) {
    if (Dentro(AreaVoltar(), x, y)) {
        SaiDaMontagem();
        return;
    }

    for (int i = 0; i < CofreVisiveis(); ++i) {
        if (!Dentro(AreaSlotCofre(i), x, y)) {
            continue;
        }
        const int qual = g_cofreRolagem * kCofreColunas + i;
        if (LojaRedeCofreItem(qual) == nullptr) {
            return;
        }
        g_cofreEscolhido = qual;
        if (DoisCliques(qual)) {
            AbreCaixaPreco(qual);
        }
        return;
    }

    // Na barra de rolagem, clicar acima ou abaixo do polegar anda uma tela.
    const RECT trilho = AreaTrilho();
    if (Dentro(trilho, x, y)) {
        const RECT polegar = AreaPolegar();
        if (y < polegar.top) {
            Rola(-kCofreLinhasVis);
        } else if (y >= polegar.bottom) {
            Rola(kCofreLinhasVis);
        }
        return;
    }

    // Na previa, o clique tira o item da barraca - e a unica forma de desfazer.
    for (int i = 0; i < kMaxPrateleiras; ++i) {
        if (!Dentro(AreaSlotPrateleira(i), x, y)) {
            continue;
        }
        if (i >= g_prateleirasUsadas) {
            return;
        }
        for (int k = i; k + 1 < g_prateleirasUsadas; ++k) {
            g_prateleiras[k] = g_prateleiras[k + 1];
        }
        --g_prateleirasUsadas;
        g_prateleiras[g_prateleirasUsadas].cargoPos = -1;
        g_prateleiras[g_prateleirasUsadas].moeda = 0;
        g_prateleiras[g_prateleirasUsadas].preco = 0;
        return;
    }

    if (Dentro(AreaAbrirLoja(), x, y)) {
        if (g_diag != 0) {
            char buf[140];
            sprintf_s(buf, "=== diag abrir: clique com %d prateleiras, nome \"%s\"",
                      g_prateleirasUsadas, g_nomeBarraca);
            Log(buf);
        }
        if (g_prateleirasUsadas > 0) {
            LojaRedeAbre(g_nomeBarraca, g_prateleiras, g_prateleirasUsadas);
            // A barraca subiu: o painel sai da frente inteiro, e nao volta para
            // o menu inicial. Quem acabou de montar quer ver a barraca no mapa,
            // e cair de novo na escolha "Mercado ou Lojinha" so obrigaria a
            // fechar na mao. Quem avisa o servidor de que o painel fechou e o
            // RenovaSePreciso, que olha esta bandeira a cada quadro.
            g_caixa = kCaixaNenhuma;
            g_qualTela = kTelaMenu;
            g_aberta = false;
        }
    }
}

void CliqueJanela(int x, int y) {
    // A caixa vem primeiro e nao deixa passar nada: nem o X, nem as portas.
    if (g_caixa != kCaixaNenhuma) {
        CliqueCaixa(x, y);
        return;
    }
    const RECT f = AreaFechar();
    if (x >= f.left - 4 && x <= f.right + 4 && y >= f.top - 4 && y <= f.bottom + 4) {
        g_aberta = false;
        g_qualTela = kTelaMenu;
        return;
    }
    if (g_qualTela == kTelaMenu) {
        for (int i = 0; i < 2; ++i) {
            const RECT r = AreaPorta(i);
            if (x < r.left || x >= r.right || y < r.top || y >= r.bottom) {
                continue;
            }
            if (i == 0) {
                g_qualTela = kTelaMercado;
                g_filtro = kMenuTodos;
                g_pagina = 0;
                g_escolhido = -1;
                PedeAoServidor();
            } else {
                if (LojaRedeBarracaAberta() != 0) {
                    return;   // ja ha uma barraca de pe; a porta esta apagada
                }
                // Montar comeca perguntando o nome, como qualquer janela do
                // jogo que precisa de um: so depois de responder e que a tela
                // de montagem aparece.
                AbreCaixaNome();
            }
            return;
        }
        return;
    }
    if (Montando()) {
        CliqueMontagem(x, y);
        return;
    }
    for (int i = 0; i < kMenuTotal; ++i) {
        const RECT r = AreaMenu(i);
        if (x >= r.left && x < r.right && y >= r.top && y < r.bottom) {
            if (i <= kMenuMeus) {
                g_filtro = i;
                g_pagina = 0;
                g_escolhido = -1;
                PedeAoServidor();
            } else if (i == kMenuVoltar) {
                g_qualTela = kTelaMenu;
                g_escolhido = -1;
            } else {
                // EMENDA PARA A HANNA: sacar o dinheiro de vendas em Cash/RMT
                // mexe no saldo da CONTA, que vive no banco do site. Falta o
                // mesmo RPC descrito em tmserver/internal/handler/lojasaldo.go;
                // com ele, aqui entra um pacote novo (0x0F05) pedindo o saque.
                Log("=== loja: 'Realizar saque' depende do saldo de conta (ver lojasaldo.go)");
            }
            return;
        }
    }
    for (int i = 0; i < kPorPagina; ++i) {
        const RECT r = AreaSlot(i);
        if (x >= r.left && x < r.right && y >= r.top && y < r.bottom) {
            g_escolhido = LojaRedeOferta(i) != nullptr ? i : -1;
            return;
        }
    }
    const RECT esq = AreaSeta(false);
    const RECT dir = AreaSeta(true);
    if (y >= esq.top && y < esq.bottom) {
        if (x >= esq.left && x < esq.right && g_pagina > 0) {
            --g_pagina;
            g_escolhido = -1;
            PedeAoServidor();
            return;
        }
        if (x >= dir.left && x < dir.right && g_pagina + 1 < Paginas()) {
            ++g_pagina;
            g_escolhido = -1;
            PedeAoServidor();
            return;
        }
        const RECT c = AreaBotaoComprar();
        if (x >= c.left && x < c.right) {
            const LojaOferta* esc = LojaRedeOferta(g_escolhido);
            if (esc != nullptr && (g_filtro == kMenuMeus || !OfertaMinha(esc))) {
                if (g_filtro == kMenuMeus) {
                    // Na minha barraca o botao troca a moeda do item, girando
                    // entre ouro, cash e RMT.
                    LojaRedeMoeda(esc->slot, (esc->moeda + 1) % 3);
                } else if (esc->perto != 0) {
                    LojaRedeCompra(esc->vendedor, esc->slot, esc->moeda);
                }
                PedeAoServidor();
            }
            return;
        }
        const RECT f2 = AreaBotaoFechar();
        if (x >= f2.left && x < f2.right) {
            g_aberta = false;
        }
    }
}

void JanelaMedida(int telaL, int telaA, int* x, int* y, int* largura, int* altura);
void DiagCliqueNaFaixa();
void BotaoMedida(int telaL, int telaA, int* x, int* y, int* largura, int* altura);

// --- a dica do item --------------------------------------------------------
//
// O mesmo que o jogo faz quando o mouse para sobre um item: uma caixa com o
// nome e a descricao. O texto sai das tabelas que o cliente ja carregou (ver
// dica.h); aqui fica so a caixa, no desenho da loja, ao lado do painel.
// Qual item esta sob o cursor, e onde esta o quadrado dele. Roda todo quadro,
// junto com a camada da janela.
void AtualizaDica() {
    g_dicaItem = 0;
    g_dicaRefino = 0;
    int cx = 0;
    int cy = 0;
    if (g_aberta && CamadaCursor(&cx, &cy)) {
        int px = 0;
        int py = 0;
        int pl = 0;
        int pa = 0;
        JanelaMedida(CamadaTelaL(), CamadaTelaA(), &px, &py, &pl, &pa);
        const int x = cx - px;
        const int y = cy - py;
        // Com uma caixa aberta nao ha dica: o que esta embaixo esta escuro e
        // fora de alcance.
        const int quantos = Montando() ? CofreVisiveis() + kMaxPrateleiras : kPorPagina;
        for (int i = 0; i < quantos && g_dicaItem == 0 && g_caixa == kCaixaNenhuma; ++i) {
            RECT r;
            const LojaItemCofre* doCofre = nullptr;
            const LojaOferta* daVitrine = nullptr;
            if (!Montando()) {
                r = AreaSlot(i);
                daVitrine = LojaRedeOferta(i);
            } else if (i < CofreVisiveis()) {
                r = AreaSlotCofre(i);
                doCofre = LojaRedeCofreItem(g_cofreRolagem * kCofreColunas + i);
            } else {
                const int qual = i - CofreVisiveis();
                r = AreaSlotPrateleira(qual);
                doCofre = qual < g_prateleirasUsadas
                              ? ItemDoSlot(g_prateleiras[qual].cargoPos)
                              : nullptr;
            }
            if (x < r.left || x >= r.right || y < r.top || y >= r.bottom) {
                continue;
            }
            if (doCofre != nullptr) {
                g_dicaItem = doCofre->indice;
                g_dicaRefino = doCofre->refino;
            } else if (daVitrine != nullptr) {
                g_dicaItem = daVitrine->indice;
                g_dicaRefino = daVitrine->refino;
            }
            if (g_dicaItem != 0) {
                g_dicaSlot.left = px + r.left;
                g_dicaSlot.top = py + r.top;
                g_dicaSlot.right = px + r.right;
                g_dicaSlot.bottom = py + r.bottom;
            }
        }
    }
}

// --- a dica do jogo, com o nosso item --------------------------------------
//
// A caixa de informacao do item nao e desenhada por nos: e a do cliente. Ele a
// monta em 0x416A80, a cada movimento do mouse, assim: pergunta a interface
// quem esta sob o ponto (a chamada virtual +0xB8, em 0x416B48), e se o que
// voltou for um controle com item em +0x670, descreve esse item.
//
// A nossa loja nao existe para essa interface - o desenho e nosso, em cima do
// quadro -, entao a pergunta volta vazia e nenhuma dica aparece. O desvio entra
// depois da pergunta: quando o cursor esta sobre um item do painel, a resposta
// passa a ser um controle nosso, que nao tem nada dentro alem do ponteiro do
// item no lugar certo. De 248 usos do controle naquela funcao, 244 sao para
// pegar esse ponteiro - o resto e guardar o proprio controle num global.
//
// Assim a caixa que aparece e a do jogo, com o texto, as cores e a posicao
// dele. Nada de uma janela parecida feita por nos.
constexpr DWORD kPerguntaQuemEsta = 0x00416B48;
constexpr DWORD kPerguntaVolta = 0x00416B4E;
const BYTE kPerguntaBytes[6] = {0xFF, 0x92, 0xB8, 0x00, 0x00, 0x00};

constexpr int kItemNoControle = 0x670;
constexpr int kEfSanc = 43;   // EF_SANC: o refino, o "+N" do nome

#pragma pack(push, 1)
struct ItemDoCliente {
    short indice;
    struct {
        BYTE efeito;
        BYTE valor;
    } efeitos[3];
};
#pragma pack(pop)

ItemDoCliente g_itemDaDica;
BYTE g_controleDaDica[kItemNoControle + 8];

// Devolve o controle que o cliente deve descrever: o nosso quando o cursor esta
// sobre um item do painel, ou o que ele mesmo achou.
extern "C" DWORD __cdecl LojaControleDaDica(DWORD achado) {
    if (!g_aberta || g_dicaItem <= 0) {
        return achado;
    }
    memset(&g_itemDaDica, 0, sizeof(g_itemDaDica));
    g_itemDaDica.indice = static_cast<short>(g_dicaItem);
    if (g_dicaRefino > 0) {
        g_itemDaDica.efeitos[0].efeito = kEfSanc;
        g_itemDaDica.efeitos[0].valor = static_cast<BYTE>(g_dicaRefino);
    }
    memset(g_controleDaDica, 0, sizeof(g_controleDaDica));
    *reinterpret_cast<ItemDoCliente**>(g_controleDaDica + kItemNoControle) = &g_itemDaDica;
    return reinterpret_cast<DWORD>(g_controleDaDica);
}

// A funcao da dica desiste logo na entrada quando o terceiro argumento e zero
// (0x416B00) - e e zero justamente quando o ponteiro nao esta sobre nenhuma
// janela do cliente, que e o nosso caso: a loja nao existe para a interface
// dele. Esse argumento nao e usado em nenhum outro lugar da funcao (conferido:
// uma unica comparacao em 6 KB de codigo), entao forca-lo a um quando o cursor
// esta sobre um item do painel nao muda mais nada - so deixa a funcao seguir
// ate a pergunta de quem esta sob o ponto, onde o nosso item entra.
extern "C" int __cdecl LojaQuerDicaDoJogo() {
    return (g_aberta && g_dicaItem > 0) ? 1 : 0;
}

constexpr DWORD kDicaEntrada = 0x00416A80;
constexpr DWORD kDicaEntradaVolta = 0x00416A88;
const BYTE kDicaEntradaBytes[8] = {0x55, 0x8B, 0xEC, 0xB8, 0x98, 0x14, 0x00, 0x00};

__declspec(naked) void DicaEntradaHook() {
    __asm {
        pushad
        pushfd
        call LojaQuerDicaDoJogo
        test eax, eax
        je segue
        mov dword ptr [esp + 0x30], 1   // o terceiro argumento, na pilha de quem chamou
    segue:
        popfd
        popad
        push ebp                        // instrucoes originais
        mov ebp, esp
        mov eax, 0x1498
        push kDicaEntradaVolta
        ret
    }
}

__declspec(naked) void PerguntaHook() {
    __asm {
        call dword ptr [edx + 0xB8]   // a pergunta original, com os argumentos ja na pilha
        push ecx
        push edx
        push eax
        call LojaControleDaDica
        add esp, 4
        pop edx
        pop ecx
        push kPerguntaVolta
        ret
    }
}

void DesviaDicaDoJogo() {
    if (memcmp(reinterpret_cast<void*>(kPerguntaQuemEsta), kPerguntaBytes,
               sizeof(kPerguntaBytes)) != 0) {
        Log("=== loja: a pergunta da dica mudou de bytes, dica do jogo nao instalada");
        return;
    }
    BYTE salto[sizeof(kPerguntaBytes)];
    memset(salto, 0x90, sizeof(salto));
    salto[0] = 0xE9;
    const DWORD rel = reinterpret_cast<DWORD>(&PerguntaHook) - (kPerguntaQuemEsta + 5);
    memcpy(salto + 1, &rel, sizeof(rel));
    DWORD antes = 0;
    if (!VirtualProtect(reinterpret_cast<void*>(kPerguntaQuemEsta), sizeof(salto),
                        PAGE_EXECUTE_READWRITE, &antes)) {
        return;
    }
    memcpy(reinterpret_cast<void*>(kPerguntaQuemEsta), salto, sizeof(salto));
    VirtualProtect(reinterpret_cast<void*>(kPerguntaQuemEsta), sizeof(salto), antes, &antes);
    FlushInstructionCache(GetCurrentProcess(), reinterpret_cast<void*>(kPerguntaQuemEsta),
                          sizeof(salto));
    if (memcmp(reinterpret_cast<void*>(kDicaEntrada), kDicaEntradaBytes,
               sizeof(kDicaEntradaBytes)) == 0) {
        BYTE salto2[sizeof(kDicaEntradaBytes)];
        memset(salto2, 0x90, sizeof(salto2));
        salto2[0] = 0xE9;
        const DWORD rel2 = reinterpret_cast<DWORD>(&DicaEntradaHook) - (kDicaEntrada + 5);
        memcpy(salto2 + 1, &rel2, sizeof(rel2));
        DWORD antes2 = 0;
        if (VirtualProtect(reinterpret_cast<void*>(kDicaEntrada), sizeof(salto2),
                           PAGE_EXECUTE_READWRITE, &antes2)) {
            memcpy(reinterpret_cast<void*>(kDicaEntrada), salto2, sizeof(salto2));
            VirtualProtect(reinterpret_cast<void*>(kDicaEntrada), sizeof(salto2), antes2,
                           &antes2);
            FlushInstructionCache(GetCurrentProcess(), reinterpret_cast<void*>(kDicaEntrada),
                                  sizeof(salto2));
        }
    } else {
        Log("=== loja: a entrada da dica mudou de bytes");
    }
    Log("=== loja: a dica do jogo passa a descrever o item do painel");
}

// --- as tres camadas -------------------------------------------------------
// A lojinha antiga esta aposentada: se a janela dela aparecer por qualquer
// caminho, e fechada na hora. Ela NAO abre a nossa - abrir a nossa e so pelo
// clique no botao da barra. Enquanto esta funcao tambem abria, a loja aparecia
// sozinha: bastava o cliente marcar aquela janela como ativa por um instante.
void FechaLojinhaAntiga() {
    if (!EmJogo()) {
        return;
    }
    volatile WORD* ativa = reinterpret_cast<volatile WORD*>(kIdJanelaAtiva);
    if (*ativa != kIdLojaPessoal) {
        return;
    }
    *ativa = kIdNenhuma;
    static bool avisou = false;
    if (!avisou) {
        avisou = true;
        Log("=== loja: janela da lojinha antiga fechada (ela esta aposentada)");
    }
}

// --- a tecla Esc -----------------------------------------------------------
//
// Com a loja aberta, Esc fecha a loja e nao chega ao jogo, que abriria o menu da
// engrenagem por cima. Os tres caminhos que a tecla percorre - e por que cortar
// so a descida nao bastava - estao no teclas.h; aqui fica so o que a loja faz.
int LojaEsc() {
    if (!g_aberta) {
        return 0;
    }
    // Com uma caixa aberta o Esc fecha so ela, e o painel continua onde estava.
    if (g_caixa != kCaixaNenhuma) {
        g_caixa = kCaixaNenhuma;
        return 1;
    }
    // Um Esc fecha tudo, inclusive a tela de montagem: o que estava montado e
    // descartado, como acontece ao fechar qualquer janela do jogo pela metade.
    g_qualTela = kTelaMenu;
    g_aberta = false;
    return 1;
}

int JanelaVisivel() {
    DiagJanelaAtiva();
    if (g_diag != 0) {
        // Borda de aperto do botao esquerdo, vista de fora do dono do clique:
        // serve so para a sonda da faixa saber quando olhar.
        static bool antes = false;
        const bool agora = (GetAsyncKeyState(VK_LBUTTON) & 0x8000) != 0;
        if (agora && !antes) {
            DiagCliqueNaFaixa();
        }
        antes = agora;
    }
    // O teclado e decidido a cada quadro, e nao em cada caminho que abre ou
    // fecha o painel: o X, o icone, o Esc e o botao Fechar sao quatro saidas, e
    // esquecer uma delas deixaria o teclado preso conosco.
    //
    // A caixa do nome conta tanto quanto a tela de montagem: ela abre ANTES
    // dela, ainda no menu, e sem isto nao haveria onde digitar o nome.
    const bool querTeclado = g_aberta && (Montando() || g_caixa != kCaixaNenhuma);
    TeclaTexto(querTeclado ? LojaDigita : nullptr);
    // Clicar numa barraca na cidade abre a vitrine: quem avisa e o servidor,
    // respondendo ao mesmo pacote que abria a janela antiga.
    if (LojaRedePedidoMercado() != 0 && !g_aberta) {
        g_aberta = true;
        g_qualTela = kTelaMercado;   // clicou numa barraca: e o mercado que ele quer
        g_qualTela = kTelaMenu;
        g_escolhido = -1;
        g_filtro = kMenuTodos;
        g_pagina = 0;
        PedeAoServidor();
    }
    AtualizaDica();
    FechaLojinhaAntiga();   // roda todo quadro: a lojinha antiga nao volta
    RenovaSePreciso();
    return g_aberta ? 1 : 0;
}

void JanelaMedida(int telaL, int telaA, int* x, int* y, int* largura, int* altura) {
    *largura = kLargura;
    *altura = kAltura;
    // Encostada na janela do Banco pela esquerda, no mesmo topo que ela. Se a
    // tela for estreita demais para isso, a loja encosta na margem.
    *x = kBancoX - kLargura;
    *y = kBancoY;
    if (*x < 0 || kBancoX > telaL) {
        *x = 0;
    }
    if (*y + kAltura > telaA) {
        *y = telaA > kAltura ? telaA - kAltura : 0;
    }
}

const void* JanelaPixels(int* versao) {
    Repinta();
    *versao = g_versao;
    return g_tela.pixels;
}

void JanelaClique(int x, int y) {
    CliqueJanela(x, y);
    Repinta();
}

// --- o clique no botao do jogo ---------------------------------------------
//
// Nada e desenhado aqui: quem desenha o icone da Loja Pessoal e o proprio jogo,
// com a arte dele. Esta camada existe so para ficar por cima daquela celula e
// FICAR com o clique, abrindo a nossa loja em vez da lojinha antiga. Como
// pixels devolve nulo, o modulo de D3D nao desenha nada - mas o teste do mouse
// continua valendo.
//
// A celula so responde com a barra aberta; fora isso o clique e do mundo, e
// seria um estrago devolver o personagem andando para la.
int BotaoVisivel() {
    if (!g_roubaClique || !EmJogo()) {
        return 0;
    }
    return BarraAberta() ? 1 : 0;
}

// A celula inteira, e nao um retangulo do nosso tamanho dentro dela.
//
// O loja.txt guarda um PONTO dentro da celula da Loja Pessoal, nao a medida
// dela. A medida vem da propria faixa que o cliente desenhou: as celulas sao
// quadradas (a altura da faixa e o lado), entao quantas cabem e a largura
// dividida pela altura, e a nossa e a que contem o ponto. Com o retangulo fixo
// de 30 pixels sobrava uma tira da celula de fora - e era por essa tira que o
// clique passava para o jogo e abria a lojinha antiga.
void BotaoMedida(int telaL, int telaA, int* x, int* y, int* largura, int* altura) {
    IconeCanto(telaL, telaA, x, y);
    *largura = g_iconeL;
    *altura = g_iconeA;
    if (!BarraAberta() || g_faixaA <= 0.0f || g_faixaL <= 0.0f) {
        return;
    }
    const int lado = static_cast<int>(g_faixaA + 0.5f);
    const int quantas = static_cast<int>(g_faixaL / g_faixaA + 0.5f);
    if (lado <= 0 || quantas <= 0) {
        return;
    }
    // Em fracao, e nao em divisao inteira: a celula desta faixa tem 38,43 de
    // largura, e arredondar para 38 ia perdendo quase meio pixel por celula -
    // na quarta, a borda direita caia um pixel e meio para dentro, e o clique
    // bem na beirada passava para o jogo. Foi assim que a lojinha antiga
    // continuou abrindo depois de a area ja cobrir "a celula inteira".
    const float largCelula = g_faixaL / static_cast<float>(quantas);
    const int qual = static_cast<int>((static_cast<float>(*x) - g_faixaX) / largCelula);
    if (qual < 0 || qual >= quantas) {
        return;
    }
    const float esq = g_faixaX + qual * largCelula;
    const float dir = g_faixaX + (qual + 1) * largCelula;
    *x = static_cast<int>(esq);
    *y = static_cast<int>(g_faixaY);
    *largura = static_cast<int>(dir + 0.5f) - *x;
    *altura = lado;
}

const void* BotaoPixels(int* versao) {
    *versao = 0;
    return nullptr;   // camada sem desenho
}

// Diagnostico do lugar do icone: com a barra aberta, anota todo clique que caiu
// na faixa e NAO foi nosso. E assim que se descobre onde termina a celula da
// Loja Pessoal sem adivinhar - a nossa area pode estar menor que ela, e a sobra
// e por onde a lojinha antiga ainda abre.
void DiagCliqueNaFaixa() {
    if (g_diag == 0 || !BarraAberta()) {
        return;
    }
    int cx = 0;
    int cy = 0;
    if (!CamadaCursor(&cx, &cy)) {
        return;
    }
    if (static_cast<float>(cx) < g_faixaX || static_cast<float>(cx) > g_faixaX + g_faixaL ||
        static_cast<float>(cy) < g_faixaY || static_cast<float>(cy) > g_faixaY + g_faixaA) {
        return;
    }
    int ix = 0;
    int iy = 0;
    int il = 0;
    int ia = 0;
    BotaoMedida(CamadaTelaL(), CamadaTelaA(), &ix, &iy, &il, &ia);
    if (cx >= ix && cx < ix + il && cy >= iy && cy < iy + ia) {
        return;   // esse foi nosso
    }
    char buf[220];
    sprintf_s(buf,
              "=== diag faixa: clique em (%d,%d) FORA; faixa (%.0f,%.0f %.0fx%.0f); nosso "
              "icone (%d,%d %dx%d)",
              cx, cy, g_faixaX, g_faixaY, g_faixaL, g_faixaA, ix, iy, il, ia);
    Log(buf);
}

void BotaoClique(int, int) {
    g_aberta = !g_aberta;
    if (g_aberta) {
        g_qualTela = kTelaMenu;   // a loja abre perguntando, nao mostrando
        g_escolhido = -1;
        PedeAoServidor();
        Repinta();
        Log("=== loja: clique no botao do jogo, abrindo a nossa loja");
    }
}

const Camada kCamadaBotao = {20, BotaoVisivel, BotaoMedida, BotaoPixels, BotaoClique};

} // namespace

// Devolve o botao ao jogo: com isto desligado, o clique na celula volta a abrir
// a lojinha original. Serve para investigar o cliente sem desinstalar nada.
extern "C" void __cdecl LojaMostraIcone(int roubar) {
    g_roubaClique = roubar != 0;
}

namespace {

const Camada kCamadaJanela = {30, JanelaVisivel, JanelaMedida, JanelaPixels, JanelaClique};

__declspec(naked) void AppendNodeHook() {
    __asm {
        mov eax, [esp + 8]        // o no entregue
        mov edx, [esp + 12]       // e a camada de desenho dele
        pushad
        pushfd
        push edx
        push eax
        call LojaAnotaNo
        add esp, 8
        popfd
        popad
        push ebp                                // instrucoes originais
        mov ebp, esp
        cmp dword ptr [ebp + 0x10], 0x1E
        push 0x0040C444
        ret
    }
}

void DesviaAppendNode() {
    if (memcmp(reinterpret_cast<void*>(kAppendNode), kAppendNodeBytes,
               sizeof(kAppendNodeBytes)) != 0) {
        Log("=== loja: AppendNode com bytes diferentes, barra nao sera detectada");
        return;
    }
    BYTE salto[sizeof(kAppendNodeBytes)];
    memset(salto, 0x90, sizeof(salto));
    salto[0] = 0xE9;
    const DWORD rel = reinterpret_cast<DWORD>(&AppendNodeHook) - (kAppendNode + 5);
    memcpy(salto + 1, &rel, sizeof(rel));
    DWORD antes = 0;
    if (!VirtualProtect(reinterpret_cast<void*>(kAppendNode), sizeof(salto),
                        PAGE_EXECUTE_READWRITE, &antes)) {
        return;
    }
    memcpy(reinterpret_cast<void*>(kAppendNode), salto, sizeof(salto));
    VirtualProtect(reinterpret_cast<void*>(kAppendNode), sizeof(salto), antes, &antes);
    FlushInstructionCache(GetCurrentProcess(), reinterpret_cast<void*>(kAppendNode),
                          sizeof(salto));
}

// O Esc tem DOIS consumidores, e era por isso que a loja fechava mas o menu
// abria assim mesmo. O ramo de WM_KEYDOWN da WndProc (0x54C62F) entrega a tecla
// ao jogo em 0x54C875 - o que o desvio anterior, em 0x4B5803, ja engolia - e
// DEPOIS cai em 0x54D334, que repassa a mesma mensagem para o outro tratador
// (o +0x11C, pela vtable +0x98) antes do DefWindowProc. E esse segundo que abre
// o menu da engrenagem.
//
// Entao o desvio subiu: entra na entrada do ramo de WM_KEYDOWN e, quando a
// tecla e nossa, sai da WndProc devolvendo 0 (0x54D3AB, que e o pop edi/leave/
// ret 0x10 dela). Nenhum dos dois caminhos chega a ver a tecla.
// --- diagnostico dos icones ------------------------------------------------
//
// O cliente monta o icone de um item em 0x40D6D5: ele le itemicon.bin (carregado
// em 0x6EA518) com o indice do item e tira 1. Anotar par a par o que ELE usa e o
// jeito de descobrir por que a nossa tabela desenha outro desenho.
constexpr DWORD kIconeLe = 0x0040D6D5;
constexpr BYTE kIconeLeBytes[7] = {0x8B, 0x04, 0x95, 0x18, 0xA5, 0x6E, 0x00};

extern "C" void __cdecl LojaAnotaIcone(int item, int valor) {
    if (g_diag == 0) {
        return;
    }
    static int vistos[32];
    static int nVistos = 0;
    for (int i = 0; i < nVistos; ++i) {
        if (vistos[i] == item) {
            return;
        }
    }
    if (nVistos < 32) {
        vistos[nVistos++] = item;
    }
    char buf[120];
    sprintf_s(buf, "=== diag icone: item %d -> tabela %d (o cliente usa %d)", item, valor,
              valor - 1);
    Log(buf);
}

__declspec(naked) void IconeLeHook() {
    __asm {
        mov eax, dword ptr [edx * 4 + 0x006EA518]   // instrucao original
        pushad
        pushfd
        push eax
        push edx
        call LojaAnotaIcone
        add esp, 8
        popfd
        popad
        push 0x0040D6DC
        ret
    }
}

// Instala um desvio simples de N bytes: salto e o resto em nop.
void DesviaSimples(DWORD onde, const BYTE* esperado, int quantos, void* destino,
                   const char* nome) {
    if (memcmp(reinterpret_cast<void*>(onde), esperado, quantos) != 0) {
        char buf[120];
        sprintf_s(buf, "=== loja: %s com bytes diferentes, nao instalado", nome);
        Log(buf);
        return;
    }
    BYTE salto[8];
    memset(salto, 0x90, sizeof(salto));
    salto[0] = 0xE9;
    const DWORD rel = reinterpret_cast<DWORD>(destino) - (onde + 5);
    memcpy(salto + 1, &rel, sizeof(rel));
    DWORD antes = 0;
    if (!VirtualProtect(reinterpret_cast<void*>(onde), quantos, PAGE_EXECUTE_READWRITE, &antes)) {
        return;
    }
    memcpy(reinterpret_cast<void*>(onde), salto, quantos);
    VirtualProtect(reinterpret_cast<void*>(onde), quantos, antes, &antes);
    FlushInstructionCache(GetCurrentProcess(), reinterpret_cast<void*>(onde), quantos);
    char buf[120];
    sprintf_s(buf, "=== loja: %s desviado em 0x%06X", nome, onde);
    Log(buf);
}

struct Registro {
    Registro() {
        CarregaConfig();
        DesviaAppendNode();
        TeclaRegistra(VK_ESCAPE, 30, LojaEsc);
        DesviaDicaDoJogo();
        DesviaSimples(kLacoDasCamadas, kLacoBytes, sizeof(kLacoBytes),
                      reinterpret_cast<void*>(&LacoDasCamadasHook), "fila de desenho");
        DesviaSimples(kIconeLe, kIconeLeBytes, sizeof(kIconeLeBytes),
                      reinterpret_cast<void*>(&IconeLeHook), "leitura do icone");
        CamadaRegistra(&kCamadaBotao);
        CamadaRegistra(&kCamadaJanela);
    }
};

Registro g_registro;

} // namespace
