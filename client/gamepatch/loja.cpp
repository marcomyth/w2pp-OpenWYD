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
constexpr int kAltSaldos = 20;
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
constexpr int kTopoDetalhe = kTopoConteudo + kAltGrade + 6;
constexpr int kTopoSaldos = kTopoDetalhe + kAltDetalhe + 6;

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
    kMenuCriar,
    kMenuSaque,
    kMenuTotal,
};

const char* const kMenu[kMenuTotal] = {"Todos",      "Ouro",         "Cash",
                                       "RMT",        "Meus itens",   "Criar lojinha",
                                       "Realizar saque"};

// Em modo montagem a mesma coluna vira os controles de preco: cada degrau soma,
// "Zerar" recomeca e "Moeda" gira entre ouro, cash e RMT.
// A fileira de cima sao os comandos; a de baixo, atalhos de preco. O preco
// mesmo e digitado: em ouro os numeros sao grandes demais para somar de mil em
// mil, e em cash e RMT sao pequenos demais para o degrau de milhao fazer
// sentido.
enum {
    kMontaVoltar = 0,
    kMontaBarraca,
    kMontaZerar,
    kMontaMil,
    kMontaCemMil,
    kMontaMilhao,
    kMontaMoeda,
};
// A moeda fica na fileira de baixo porque o nome dela e comprido: "Moeda: Ouro"
// nao cabe nos 64 pixels da de cima.
const char* const kMenuMonta[kMenuTotal] = {"Voltar",    "Barraca",   "Zerar",  "+1 mil",
                                            "+100 mil",  "+1 milhao", "Moeda"};
const int kDegrau[kMenuTotal] = {0, 0, 0, 1000, 100000, 1000000, 0};
constexpr int kPrecoMax = 1999999999;

// --- estado ----------------------------------------------------------------
Tela g_tela;
HFONT g_fonte = nullptr;
HFONT g_negrito = nullptr;
HFONT g_miudo = nullptr;
bool g_aberta = false;
int g_filtro = kMenuTodos;
int g_pagina = 0;
int g_escolhido = -1;
int g_versao = 0;

// Montar a barraca: o painel virou a janela de montagem, porque a do cliente so
// sabia de ouro. Nao ha onde digitar - o cliente le o teclado por conta dele e
// cada numero e um atalho do jogo -, entao o preco entra por botoes e o titulo
// e o nome do personagem, posto pelo servidor.
bool g_montando = false;
// O nome da barraca, digitado na tela de montagem. Vazio: o servidor poe o nome
// do personagem, como a lojinha do jogo faz.
char g_nomeBarraca[24] = {0};
int g_nomeTam = 0;
int g_campo = 0;            // 0 = nome, 1 = preco
bool g_vendoBarraca = false;   // a grade mostra as prateleiras, nao o cofre
int g_cofrePagina = 0;
int g_cofreEscolhido = -1;    // indice na lista do cofre
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
void RenovaSePreciso() {
    if (!g_aberta) {
        return;
    }
    const DWORD agora = GetTickCount();
    if (agora - g_ultimoPedido > 3000) {
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
    return kTopoSaldos + kAltSaldos + 6;
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
    DrawTextA(hdc, g_montando ? "Montar a minha barraca" : "Loja do Servidor", -1, &rt,
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

    const int largura = l / 3;
    for (int i = 0; i < 3; ++i) {
        const int cx = x + i * largura;
        Losango(hdc, cx + 9, y + kAltSaldos / 2, 4, kCorMoeda[i]);
        char valor[40];
        Curto(LojaRedeSaldo(i), valor, sizeof(valor));
        char texto[64];
        sprintf_s(texto, "%s %s", kNomeMoeda[i], valor);
        SelectObject(hdc, g_miudo);
        SetTextColor(hdc, kCorMoeda[i]);
        RECT rt = {cx + 17, y, cx + largura - 2, y + kAltSaldos};
        DrawTextA(hdc, texto, -1, &rt, DT_LEFT | DT_VCENTER | DT_SINGLELINE | DT_NOPREFIX);
    }
}

void PintaMenu(HDC hdc) {
    for (int i = 0; i < kMenuTotal; ++i) {
        const RECT r = AreaMenu(i);
        bool ativo = false;
        char texto[32];
        if (g_montando) {
            if (i == kMontaMoeda) {
                sprintf_s(texto, "Moeda: %s", kNomeMoeda[g_moedaEdicao % 3]);
                ativo = true;
            } else if (i == kMontaBarraca) {
                sprintf_s(texto, "Barraca %d", g_prateleirasUsadas);
                ativo = g_vendoBarraca;
            } else {
                sprintf_s(texto, "%s", kMenuMonta[i]);
            }
        } else {
            sprintf_s(texto, "%s", kMenu[i]);
            ativo = (i == g_filtro) && i <= kMenuMeus;
        }
        LinhaBotao(hdc, r.left, r.top, r.right - r.left, kAltBotao, ativo);
        // A fileira de baixo carrega os nomes compridos - "Realizar saque",
        // "Moeda: Ouro" -, que so cabem na fonte miuda.
        const bool comprido = i >= kMenuLinha1 || (g_montando && i == kMontaBarraca);
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
void DesenhaIcone(HDC hdc, const RECT& r, int item, COLORREF corMoeda) {
    GdiFlush();
    if (IconeDesenha(g_tela.pixels, g_tela.l, g_tela.a, r.left, r.top, kSlot, kSlot, item) == 0) {
        Losango(hdc, r.left + kSlot / 2, r.top + kSlot / 2, 9, corMoeda);
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
    DesenhaIcone(hdc, r, o->indice, kCorMoeda[o->moeda % 3]);
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
                    int prateleira) {
    Degrade(hdc, r.left, r.top, kSlot, kSlot, RGB(28, 22, 16), RGB(14, 11, 8));
    Contorno(hdc, r.left, r.top, kSlot, kSlot, escolhido ? kCanto : RGB(64, 53, 39));
    if (it == nullptr) {
        return;
    }
    const bool naBarraca = prateleira >= 0;
    DesenhaIcone(hdc, r, it->indice,
                 naBarraca ? kCorMoeda[g_prateleiras[prateleira].moeda % 3] : RGB(120, 104, 78));
    if (naBarraca) {
        const COLORREF cor = kCorMoeda[g_prateleiras[prateleira].moeda % 3];
        Contorno(hdc, r.left + 1, r.top + 1, kSlot - 2, kSlot - 2, cor);
        Losango(hdc, r.left + 6, r.top + kSlot - 8, 4, cor);
    }
    if (it->refino > 0) {
        char ref[8];
        sprintf_s(ref, "+%d", it->refino);
        SelectObject(hdc, g_miudo);
        SetTextColor(hdc, kCanto);
        RECT rr = {r.left + 2, r.top + 1, r.left + kSlot - 2, r.top + 12};
        DrawTextA(hdc, ref, -1, &rr, DT_LEFT | DT_TOP | DT_SINGLELINE | DT_NOPREFIX);
    }
    if (it->qtd > 1) {
        char qtd[8];
        sprintf_s(qtd, "%d", it->qtd);
        SelectObject(hdc, g_miudo);
        SetTextColor(hdc, RGB(255, 255, 255));
        RECT rq = {r.left + 2, r.top + 1, r.left + kSlot - 3, r.top + 12};
        DrawTextA(hdc, qtd, -1, &rq, DT_RIGHT | DT_TOP | DT_SINGLELINE | DT_NOPREFIX);
    }
    // Embaixo so aparece preco: o da prateleira, se o item ja entrou, ou o que
    // esta sendo digitado nos botoes, no quadrado escolhido. O numero do item
    // era coisa de depuracao - quem olha a barraca quer o desenho e a
    // quantidade, como em qualquer janela do jogo.
    const bool mostraEdicao = escolhido && !naBarraca && g_precoEdicao > 0;
    if (naBarraca || mostraEdicao) {
        char rodape[16];
        Curto(naBarraca ? g_prateleiras[prateleira].preco : g_precoEdicao, rodape,
              sizeof(rodape));
        SelectObject(hdc, g_miudo);
        SetTextColor(hdc, kCorMoeda[(naBarraca ? g_prateleiras[prateleira].moeda
                                               : g_moedaEdicao) % 3]);
        RECT rp = {r.left + 1, r.top + kSlot - 14, r.right - 1, r.bottom - 2};
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

int CofrePaginas() {
    const int n = LojaRedeCofreQtd();
    const int p = (n + kPorPagina - 1) / kPorPagina;
    return p < 1 ? 1 : p;
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

// A previa: as prateleiras ja montadas, com desenho, preco e moeda, antes de a
// barraca subir. Sem isto so dava para conferir o que entrou item a item, e os
// itens de outra pagina do cofre nem apareciam.
void PintaPrateleiras(HDC hdc) {
    for (int i = 0; i < kPorPagina; ++i) {
        const RECT r = AreaSlot(i);
        if (i >= kMaxPrateleiras) {
            Degrade(hdc, r.left, r.top, kSlot, kSlot, RGB(20, 16, 12), RGB(12, 9, 7));
            continue;
        }
        const bool tem = i < g_prateleirasUsadas && g_prateleiras[i].cargoPos >= 0;
        const LojaItemCofre* it = tem ? ItemDoSlot(g_prateleiras[i].cargoPos) : nullptr;
        PintaSlotCofre(hdc, r, it, false, tem ? i : -1);
    }
}

void PintaGrade(HDC hdc) {
    if (g_montando && g_vendoBarraca) {
        PintaPrateleiras(hdc);
        return;
    }
    for (int i = 0; i < kPorPagina; ++i) {
        const RECT r = AreaSlot(i);
        if (g_montando) {
            const int qual = g_cofrePagina * kPorPagina + i;
            const LojaItemCofre* it = LojaRedeCofreItem(qual);
            PintaSlotCofre(hdc, r, it, it != nullptr && qual == g_cofreEscolhido,
                           it != nullptr ? PrateleiraDoSlot(it->slot) : -1);
        } else {
            const LojaOferta* o = LojaRedeOferta(i);
            PintaSlot(hdc, r, o, o != nullptr && i == g_escolhido);
        }
    }
}

// A linha de detalhe so existe na vitrine, onde ela diz o estado da busca
// ("nenhuma barraca aberta na cidade") e o que o comprador escolheu. Na tela de
// montagem ela foi embora: o preco em edicao aparece no proprio quadrado, e o
// resto era numero de depuracao.
void PintaDetalhe(HDC hdc) {
    if (g_montando) {
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
RECT AreaCampo(int qual) {
    const int y = kTopoDetalhe - 2;
    const int meio = kMargem + (kUtil * 3) / 5;
    RECT r = {qual == 0 ? kMargem : meio + 2, y, qual == 0 ? meio - 2 : kMargem + kUtil,
              y + kAltDetalhe + 4};
    return r;
}

void PintaCampo(HDC hdc, int qual) {
    const RECT r = AreaCampo(qual);
    const int l = r.right - r.left;
    const int a = r.bottom - r.top;
    const bool comFoco = g_campo == qual;
    Degrade(hdc, r.left, r.top, l, a, RGB(30, 23, 16), RGB(16, 12, 8));
    Contorno(hdc, r.left, r.top, l, a, comFoco ? kCanto : RGB(58, 48, 36));

    const char* rotulo = qual == 0 ? "Nome" : "Preco";
    SelectObject(hdc, g_miudo);
    SetTextColor(hdc, kTextoFraco);
    RECT rr = {r.left + 4, r.top, r.left + 36, r.bottom};
    DrawTextA(hdc, rotulo, -1, &rr, DT_LEFT | DT_VCENTER | DT_SINGLELINE | DT_NOPREFIX);

    char texto[48];
    bool cheio = false;
    if (qual == 0) {
        cheio = g_nomeTam > 0;
        sprintf_s(texto, "%s%s", cheio ? g_nomeBarraca : "a barraca", comFoco && cheio ? "|" : "");
    } else {
        cheio = g_precoEdicao > 0;
        char valor[32];
        Pontuado(g_precoEdicao, valor, sizeof(valor));
        sprintf_s(texto, "%s%s", cheio ? valor : "0", comFoco && cheio ? "|" : "");
    }
    SelectObject(hdc, cheio ? g_negrito : g_miudo);
    SetTextColor(hdc, cheio ? (qual == 0 ? kTexto : kCorMoeda[g_moedaEdicao % 3]) : kTextoFraco);
    RECT rt = {r.left + 38, r.top, r.right - 4, r.bottom};
    DrawTextA(hdc, texto, -1, &rt, DT_LEFT | DT_VCENTER | DT_SINGLELINE | DT_NOPREFIX);
}

void PintaNome(HDC hdc) {
    if (!g_montando) {
        return;
    }
    PintaCampo(hdc, 0);
    PintaCampo(hdc, 1);
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
    if (g_montando) {
        sprintf_s(pag, "%d/%d", g_cofrePagina + 1, CofrePaginas());
    } else {
        sprintf_s(pag, "%d/%d", g_pagina + 1, Paginas());
    }
    SelectObject(hdc, g_fonte);
    SetTextColor(hdc, kTexto);
    RECT rp = {esq.right, y, dir.left, y + kAltRodape};
    DrawTextA(hdc, pag, -1, &rp, DT_CENTER | DT_VCENTER | DT_SINGLELINE | DT_NOPREFIX);

    // Na montagem os mesmos dois botoes viram "Incluir" e "Abrir loja".
    if (g_montando) {
        const LojaItemCofre* it = LojaRedeCofreItem(g_cofreEscolhido);
        const bool podeIncluir = it != nullptr && g_precoEdicao > 0 &&
                                 (PrateleiraDoSlot(it->slot) >= 0 ||
                                  g_prateleirasUsadas < kMaxPrateleiras);
        const RECT c = AreaBotaoComprar();
        LinhaBotao(hdc, c.left, c.top, c.right - c.left, kAltRodape, podeIncluir);
        SelectObject(hdc, podeIncluir ? g_negrito : g_fonte);
        SetTextColor(hdc, podeIncluir ? kTextoAtivo : kTextoFraco);
        RECT rc2 = {c.left, c.top, c.right, c.bottom};
        DrawTextA(hdc, "Incluir", -1, &rc2, DT_CENTER | DT_VCENTER | DT_SINGLELINE | DT_NOPREFIX);

        const RECT f3 = AreaBotaoFechar();
        const bool podeAbrir = g_prateleirasUsadas > 0;
        LinhaBotao(hdc, f3.left, f3.top, f3.right - f3.left, kAltRodape, podeAbrir);
        SelectObject(hdc, podeAbrir ? g_negrito : g_fonte);
        SetTextColor(hdc, podeAbrir ? kTextoAtivo : kTextoFraco);
        RECT rf3 = {f3.left, f3.top, f3.right, f3.bottom};
        DrawTextA(hdc, "Abrir loja", -1, &rf3,
                  DT_CENTER | DT_VCENTER | DT_SINGLELINE | DT_NOPREFIX);
        return;
    }

    // Comprar so acende com uma oferta escolhida que esteja ao alcance: a vitrine
    // junta a cidade, mas levar o item exige estar perto da barraca.
    const LojaOferta* esc = LojaRedeOferta(g_escolhido);
    const bool podeComprar = esc != nullptr && esc->perto != 0 && g_filtro != kMenuMeus;
    const RECT c = AreaBotaoComprar();
    LinhaBotao(hdc, c.left, c.top, c.right - c.left, kAltRodape, podeComprar);
    SelectObject(hdc, podeComprar ? g_negrito : g_fonte);
    SetTextColor(hdc, podeComprar ? kTextoAtivo : kTextoFraco);
    RECT rc = {c.left, c.top, c.right, c.bottom};
    DrawTextA(hdc, g_filtro == kMenuMeus ? "Trocar moeda" : "Comprar", -1, &rc,
              DT_CENTER | DT_VCENTER | DT_SINGLELINE | DT_NOPREFIX);

    const RECT f = AreaBotaoFechar();
    LinhaBotao(hdc, f.left, f.top, f.right - f.left, kAltRodape, false);
    SelectObject(hdc, g_fonte);
    SetTextColor(hdc, kTexto);
    RECT rf = {f.left, f.top, f.right, f.bottom};
    DrawTextA(hdc, "Fechar", -1, &rf, DT_CENTER | DT_VCENTER | DT_SINGLELINE | DT_NOPREFIX);
}

void Pinta(HDC hdc) {
    SetBkMode(hdc, TRANSPARENT);
    Moldura(hdc, kLargura, kAltura);
    PintaCabecalho(hdc);
    PintaSaldos(hdc);
    PintaMenu(hdc);
    PintaGrade(hdc);
    PintaDetalhe(hdc);
    PintaNome(hdc);
    PintaRodape(hdc);
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
void IncluiNaBarraca() {   // "Incluir" no rodape
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

// O teclado enquanto a tela de montagem esta aberta: cada caractere vai para o
// nome da barraca. Devolve 1 quando consumiu - e ai o jogo nao ve a tecla, que
// senao viraria atalho de magia ou linha de chat.
extern "C" int __cdecl LojaDigita(int c) {
    if (!g_aberta || !g_montando) {
        return 0;
    }
    if (g_campo == 1) {
        // Preco: so numero, digitado da esquerda para a direita como em
        // qualquer campo de valor.
        if (c == 13) {
            IncluiNaBarraca();   // o Enter e o "Confirmar" da caixa antiga
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
    if (c == 8) {   // apagar
        if (g_nomeTam > 0) {
            g_nomeBarraca[--g_nomeTam] = 0;
        }
        return 1;
    }
    if (c < 32 || c > 126) {
        return 1;   // Enter e companhia: engolidos, mas nao entram no nome
    }
    if (g_nomeTam < static_cast<int>(sizeof(g_nomeBarraca)) - 1) {
        g_nomeBarraca[g_nomeTam++] = static_cast<char>(c);
        g_nomeBarraca[g_nomeTam] = 0;
    }
    return 1;
}

void EntraNaMontagem() {
    g_montando = true;
    g_nomeBarraca[0] = 0;
    g_nomeTam = 0;
    g_campo = 0;
    g_vendoBarraca = false;
    g_cofrePagina = 0;
    g_cofreEscolhido = -1;
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
    g_montando = false;
    g_escolhido = -1;
    PedeAoServidor();
}

// Cliques da tela de montagem. A coluna da esquerda vira os degraus de preco,
// a grade e o cofre, e os dois botoes do rodape incluem e abrem.
void CliqueMontagem(int x, int y) {
    for (int i = 0; i < kMenuTotal; ++i) {
        const RECT r = AreaMenu(i);
        if (x >= r.left && x < r.right && y >= r.top && y < r.bottom) {
            if (i == kMontaVoltar) {
                SaiDaMontagem();
            } else if (i == kMontaBarraca) {
                g_vendoBarraca = !g_vendoBarraca;
                g_cofreEscolhido = -1;
            } else if (i == kMontaZerar) {
                g_precoEdicao = 0;
            } else if (i == kMontaMoeda) {
                g_moedaEdicao = (g_moedaEdicao + 1) % 3;
            } else if (kDegrau[i] > 0) {
                const long long novo =
                    static_cast<long long>(g_precoEdicao) + kDegrau[i];
                g_precoEdicao = novo > kPrecoMax ? kPrecoMax : static_cast<int>(novo);
            }
            return;
        }
    }
    for (int i = 0; i < 2; ++i) {
        const RECT r = AreaCampo(i);
        if (x >= r.left && x < r.right && y >= r.top && y < r.bottom) {
            g_campo = i;
            return;
        }
    }
    // A grade: na previa o clique tira o item da prateleira - e a unica forma de
    // desfazer um "Incluir" -, e no cofre ele escolhe o item. Fora da grade o
    // clique segue adiante, para o rodape: engolir tudo aqui deixava "Abrir
    // loja" sem resposta enquanto a previa estivesse aberta.
    if (g_vendoBarraca) {
        for (int i = 0; i < kPorPagina && i < kMaxPrateleiras; ++i) {
            const RECT r = AreaSlot(i);
            if (x < r.left || x >= r.right || y < r.top || y >= r.bottom) {
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
    }
    for (int i = 0; i < kPorPagina && !g_vendoBarraca; ++i) {
        const RECT r = AreaSlot(i);
        if (x >= r.left && x < r.right && y >= r.top && y < r.bottom) {
            const int qual = g_cofrePagina * kPorPagina + i;
            const LojaItemCofre* it = LojaRedeCofreItem(qual);
            if (it == nullptr) {
                return;
            }
            g_cofreEscolhido = qual;
            // Escolher o item ja pede o valor, como a janela antiga fazia: la o
            // item entrava na barraca e a caixa "Insira o valor dos seus
            // produtos" aparecia na hora. Aqui o teclado vai para o campo do
            // preco no mesmo clique, e o Enter conclui.
            g_campo = 1;
            const int onde = PrateleiraDoSlot(it->slot);
            if (onde >= 0) {
                // Item que ja esta na barraca volta com o preco e a moeda dele,
                // para dar para corrigir sem comecar de novo.
                g_precoEdicao = g_prateleiras[onde].preco;
                g_moedaEdicao = g_prateleiras[onde].moeda;
            } else {
                g_precoEdicao = 0;
            }
            return;
        }
    }
    const RECT esq = AreaSeta(false);
    const RECT dir = AreaSeta(true);
    if (y >= esq.top && y < esq.bottom) {
        if (x >= esq.left && x < esq.right && g_cofrePagina > 0) {
            --g_cofrePagina;
            return;
        }
        if (x >= dir.left && x < dir.right && g_cofrePagina + 1 < CofrePaginas()) {
            ++g_cofrePagina;
            return;
        }
        const RECT inc = AreaBotaoComprar();
        if (x >= inc.left && x < inc.right) {
            IncluiNaBarraca();
            return;
        }
        const RECT abrir = AreaBotaoFechar();
        if (x >= abrir.left && x < abrir.right) {
            if (g_diag != 0) {
                char buf[140];
                sprintf_s(buf, "=== diag abrir: clique com %d prateleiras, nome \"%s\"",
                          g_prateleirasUsadas, g_nomeBarraca);
                Log(buf);
            }
            if (g_prateleirasUsadas > 0) {
                LojaRedeAbre(g_nomeBarraca, g_prateleiras, g_prateleirasUsadas);
                SaiDaMontagem();
            }
        }
    }
}

void CliqueJanela(int x, int y) {
    const RECT f = AreaFechar();
    if (x >= f.left - 4 && x <= f.right + 4 && y >= f.top - 4 && y <= f.bottom + 4) {
        g_aberta = false;
        g_montando = false;
        return;
    }
    if (g_montando) {
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
            } else if (i == kMenuCriar) {
                EntraNaMontagem();
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
            if (esc != nullptr) {
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
        for (int i = 0; i < kPorPagina && g_dicaItem == 0; ++i) {
            const RECT r = AreaSlot(i);
            if (x < r.left || x >= r.right || y < r.top || y >= r.bottom) {
                continue;
            }
            if (g_montando) {
                const LojaItemCofre* it =
                    g_vendoBarraca
                        ? (i < g_prateleirasUsadas ? ItemDoSlot(g_prateleiras[i].cargoPos)
                                                   : nullptr)
                        : LojaRedeCofreItem(g_cofrePagina * kPorPagina + i);
                if (it != nullptr) {
                    g_dicaItem = it->indice;
                    g_dicaRefino = it->refino;
                }
            } else {
                const LojaOferta* o = LojaRedeOferta(i);
                if (o != nullptr) {
                    g_dicaItem = o->indice;
                    g_dicaRefino = o->refino;
                }
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
    // Um Esc fecha tudo, inclusive a tela de montagem: o que estava montado e
    // descartado, como acontece ao fechar qualquer janela do jogo pela metade.
    g_montando = false;
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
    TeclaTexto((g_aberta && g_montando) ? LojaDigita : nullptr);
    // Clicar numa barraca na cidade abre a vitrine: quem avisa e o servidor,
    // respondendo ao mesmo pacote que abria a janela antiga.
    if (LojaRedePedidoMercado() != 0 && !g_aberta) {
        g_aberta = true;
        g_montando = false;
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
