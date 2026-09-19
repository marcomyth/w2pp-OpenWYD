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
enum {
    kMontaVoltar = 0,
    kMontaMil,
    kMontaDezMil,
    kMontaCemMil,
    kMontaMilhao,
    kMontaZerar,
    kMontaMoeda,
};
const char* const kMenuMonta[kMenuTotal] = {"Voltar", "+1 mil", "+10 mil", "+100 mil",
                                            "+1 milhao", "Zerar", "Moeda"};
const int kDegrau[kMenuTotal] = {0, 1000, 10000, 100000, 1000000, 0, 0};

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
int g_cofrePagina = 0;
int g_cofreEscolhido = -1;    // indice na lista do cofre
int g_precoEdicao = 0;
int g_moedaEdicao = kOuro;
LojaPrateleira g_prateleiras[kMaxPrateleiras];
int g_prateleirasUsadas = 0;

bool g_roubaClique = true;   // desligado durante investigacoes no botao original

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
void Curto(long long v, char* saida, size_t n) {
    if (v >= 1000000000LL) {
        sprintf_s(saida, n, "%lldB", v / 1000000000LL);
    } else if (v >= 1000000LL) {
        sprintf_s(saida, n, "%lldM", v / 1000000LL);
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
    // Embaixo so aparece preco, e so depois de o item entrar numa prateleira. O
    // numero do item era coisa de depuracao: quem olha a barraca quer ver o
    // desenho e a quantidade, como em qualquer janela do jogo.
    if (naBarraca) {
        char rodape[16];
        Curto(g_prateleiras[prateleira].preco, rodape, sizeof(rodape));
        SelectObject(hdc, g_miudo);
        SetTextColor(hdc, kCorMoeda[g_prateleiras[prateleira].moeda % 3]);
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

void PintaGrade(HDC hdc) {
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

void PintaDetalhe(HDC hdc) {
    const int x = kMargem;
    const int y = kTopoDetalhe;
    const int l = kUtil;
    Barra(hdc, x, y, l, 1, RGB(58, 48, 36));

    char texto[160];
    if (g_montando) {
        const LojaItemCofre* it = LojaRedeCofreItem(g_cofreEscolhido);
        char valor[40];
        Pontuado(g_precoEdicao, valor, sizeof(valor));
        if (it == nullptr) {
            sprintf_s(texto, "escolha um item do cofre   %d/%d prateleiras",
                      g_prateleirasUsadas, kMaxPrateleiras);
            SelectObject(hdc, g_fonte);
            SetTextColor(hdc, kTextoFraco);
        } else {
            char item[48];
            if (it->refino > 0) {
                sprintf_s(item, "item %d +%d", it->indice, it->refino);
            } else {
                sprintf_s(item, "item %d", it->indice);
            }
            sprintf_s(texto, "%s   %s %s   %d/%d prateleiras", item, valor,
                      kNomeMoeda[g_moedaEdicao % 3], g_prateleirasUsadas, kMaxPrateleiras);
            SelectObject(hdc, g_negrito);
            SetTextColor(hdc, kCorMoeda[g_moedaEdicao % 3]);
        }
        RECT rm = {x + 4, y + 2, x + l - 4, y + kAltDetalhe};
        DrawTextA(hdc, texto, -1, &rm, DT_LEFT | DT_VCENTER | DT_SINGLELINE | DT_NOPREFIX);
        return;
    }
    const LojaOferta* esc = LojaRedeOferta(g_escolhido);
    if (esc != nullptr) {
        char valor[40];
        Pontuado(esc->preco, valor, sizeof(valor));
        char item[48];
        if (esc->refino > 0) {
            sprintf_s(item, "item %d +%d", esc->indice, esc->refino);
        } else {
            sprintf_s(item, "item %d", esc->indice);
        }
        sprintf_s(texto, "%s   %s %s   %s%s", item, valor, kNomeMoeda[esc->moeda % 3], esc->nome,
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

void DiagNo(float x, float y, float l, float a);

constexpr DWORD kAppendNode = 0x0040C43D;
const BYTE kAppendNodeBytes[7] = {0x55, 0x8B, 0xEC, 0x83, 0x7D, 0x10, 0x1E};
constexpr DWORD kNoRetangulo = 0x04;

DWORD g_faixaVistaEm = 0;

extern "C" void __cdecl LojaAnotaNo(DWORD no) {
    if (no < 0x10000) {
        return;
    }
    const float* r = reinterpret_cast<const float*>(no + kNoRetangulo);
    const float x = r[0];
    const float y = r[1];
    const float l = r[2];
    const float a = r[3];
    DiagNo(x, y, l, a);
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
    }
}

// Anota, uma vez cada, o retangulo das janelas grandes que passam pelo desenho.
void DiagNo(float x, float y, float l, float a) {
    if (g_diag == 0 || l < 200.0f || a < 200.0f) {
        return;
    }
    struct Visto {
        int x;
        int y;
        int l;
        int a;
    };
    static Visto vistos[24];
    static int nVistos = 0;
    const int ix = static_cast<int>(x);
    const int iy = static_cast<int>(y);
    const int il = static_cast<int>(l);
    const int ia = static_cast<int>(a);
    for (int i = 0; i < nVistos; ++i) {
        if (vistos[i].x == ix && vistos[i].y == iy && vistos[i].l == il && vistos[i].a == ia) {
            return;
        }
    }
    if (nVistos < 24) {
        vistos[nVistos].x = ix;
        vistos[nVistos].y = iy;
        vistos[nVistos].l = il;
        vistos[nVistos].a = ia;
        ++nVistos;
    }
    char buf[140];
    sprintf_s(buf, "=== diag janela: x=%d y=%d larg=%d alt=%d  (ativa=%04X, tela %dx%d)", ix, iy,
              il, ia, *reinterpret_cast<volatile WORD*>(kIdJanelaAtiva), CamadaTelaL(),
              CamadaTelaA());
    Log(buf);
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
    const DWORD agora = GetTickCount();
    return g_faixaVistaEm != 0 && agora - g_faixaVistaEm < 200;
}

// --- cliques ---------------------------------------------------------------
// Guarda o item escolhido numa prateleira, ou atualiza a que ele ja ocupa.
void IncluiNaBarraca() {
    const LojaItemCofre* it = LojaRedeCofreItem(g_cofreEscolhido);
    if (it == nullptr || g_precoEdicao <= 0) {
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
}

void EntraNaMontagem() {
    g_montando = true;
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
            } else if (i == kMontaZerar) {
                g_precoEdicao = 0;
            } else if (i == kMontaMoeda) {
                g_moedaEdicao = (g_moedaEdicao + 1) % 3;
            } else {
                g_precoEdicao += kDegrau[i];
            }
            return;
        }
    }
    for (int i = 0; i < kPorPagina; ++i) {
        const RECT r = AreaSlot(i);
        if (x >= r.left && x < r.right && y >= r.top && y < r.bottom) {
            const int qual = g_cofrePagina * kPorPagina + i;
            const LojaItemCofre* it = LojaRedeCofreItem(qual);
            if (it == nullptr) {
                return;
            }
            g_cofreEscolhido = qual;
            // Item que ja esta na barraca volta com o preco e a moeda dele, para
            // dar para corrigir sem comecar de novo.
            const int onde = PrateleiraDoSlot(it->slot);
            if (onde >= 0) {
                g_precoEdicao = g_prateleiras[onde].preco;
                g_moedaEdicao = g_prateleiras[onde].moeda;
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
        if (x >= abrir.left && x < abrir.right && g_prateleirasUsadas > 0) {
            LojaRedeAbre(g_prateleiras, g_prateleirasUsadas);
            SaiDaMontagem();
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

void BotaoMedida(int telaL, int telaA, int* x, int* y, int* largura, int* altura) {
    IconeCanto(telaL, telaA, x, y);
    *largura = g_iconeL;
    *altura = g_iconeA;
}

const void* BotaoPixels(int* versao) {
    *versao = 0;
    return nullptr;   // camada sem desenho
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
        pushad
        pushfd
        push eax
        call LojaAnotaNo
        add esp, 4
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
        DesviaSimples(kIconeLe, kIconeLeBytes, sizeof(kIconeLeBytes),
                      reinterpret_cast<void*>(&IconeLeHook), "leitura do icone");
        CamadaRegistra(&kCamadaBotao);
        CamadaRegistra(&kCamadaJanela);
    }
};

Registro g_registro;

} // namespace
