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
#include "lojarede.h"
#include "pincel.h"

#include <cstdio>
#include <cstring>

namespace {

// --- medidas da janela -----------------------------------------------------
constexpr int kBorda = 5;            // as tres linhas da borda dupla
constexpr int kPad = 6;
constexpr int kAltCabecalho = 17;
constexpr int kAltSaldos = 20;
constexpr int kLargMenu = 104;       // coluna de botoes da esquerda
constexpr int kAltBotao = 22;
constexpr int kEspacoBotao = 4;
constexpr int kSlot = 44;            // quadrado de um item
constexpr int kEspacoSlot = 4;
constexpr int kColunas = 8;
constexpr int kLinhas = 4;
constexpr int kPorPagina = kColunas * kLinhas;
constexpr int kAltDetalhe = 16;
constexpr int kAltRodape = 22;

constexpr int kLargura = kBorda * 2 + kPad * 2 + kLargMenu + 8 + kColunas * kSlot +
                         (kColunas - 1) * kEspacoSlot;
constexpr int kTopoConteudo = kBorda + kPad + kAltCabecalho + 6 + kAltSaldos + 6;
constexpr int kAltGrade = kLinhas * kSlot + (kLinhas - 1) * kEspacoSlot;
constexpr int kAltura = kTopoConteudo + kAltGrade + 6 + kAltDetalhe + 4 + kAltRodape + kPad + kBorda;

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
bool g_roubaClique = true;   // desligado durante investigacoes no botao original
DWORD g_devolveBotaoAte = 0; // ate este instante o botao volta a ser do jogo

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
    RECT r = {kLargura - kBorda - kPad - 16, kBorda + kPad + 2, kLargura - kBorda - kPad,
              kBorda + kPad + 15};
    return r;
}

RECT AreaMenu(int i) {
    const int x = kBorda + kPad;
    const int y = kTopoConteudo + i * (kAltBotao + kEspacoBotao);
    RECT r = {x, y, x + kLargMenu, y + kAltBotao};
    return r;
}

RECT AreaSlot(int i) {
    const int col = i % kColunas;
    const int lin = i / kColunas;
    const int x = kBorda + kPad + kLargMenu + 8 + col * (kSlot + kEspacoSlot);
    const int y = kTopoConteudo + lin * (kSlot + kEspacoSlot);
    RECT r = {x, y, x + kSlot, y + kSlot};
    return r;
}

int TopoRodape() {
    return kTopoConteudo + kAltGrade + 6 + kAltDetalhe + 4;
}

RECT AreaSeta(bool direita) {
    const int meio = kLargura / 2;
    const int y = TopoRodape();
    RECT r = {direita ? meio + 26 : meio - 44, y, direita ? meio + 44 : meio - 26, y + kAltRodape};
    return r;
}

RECT AreaBotaoFechar() {
    RECT r = {kLargura - kBorda - kPad - 76, TopoRodape(), kLargura - kBorda - kPad,
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
    const int x = kBorda + kPad;
    const int y = kBorda + kPad;
    const int l = kLargura - (kBorda + kPad) * 2;

    Degrade(hdc, x, y, l, kAltCabecalho, kCabTopo, kCabBaixo);
    Contorno(hdc, x, y, l, kAltCabecalho, kCabBorda);
    Losango(hdc, x + 9, y + kAltCabecalho / 2, 3, kLosango);

    SelectObject(hdc, g_negrito);
    SetTextColor(hdc, RGB(255, 255, 255));
    RECT rt = {x + 18, y, x + l - 30, y + kAltCabecalho};
    DrawTextA(hdc, "Loja do Servidor", -1, &rt, DT_LEFT | DT_VCENTER | DT_SINGLELINE | DT_NOPREFIX);

    const RECT f = AreaFechar();
    SelectObject(hdc, g_fonte);
    SetTextColor(hdc, kLosango);
    RECT rx = {f.left, f.top - 3, f.right, f.bottom + 3};
    DrawTextA(hdc, "X", -1, &rx, DT_CENTER | DT_VCENTER | DT_SINGLELINE | DT_NOPREFIX);
}

void PintaSaldos(HDC hdc) {
    const int x = kBorda + kPad;
    const int y = kBorda + kPad + kAltCabecalho + 6;
    const int l = kLargura - (kBorda + kPad) * 2;

    Degrade(hdc, x, y, l, kAltSaldos, RGB(30, 23, 16), RGB(16, 12, 8));
    Contorno(hdc, x, y, l, kAltSaldos, RGB(58, 48, 36));

    const int largura = l / 3;
    for (int i = 0; i < 3; ++i) {
        const int cx = x + i * largura;
        Losango(hdc, cx + 14, y + kAltSaldos / 2, 4, kCorMoeda[i]);
        char valor[40];
        Pontuado(LojaRedeSaldo(i), valor, sizeof(valor));
        char texto[64];
        sprintf_s(texto, "%s  %s", kNomeMoeda[i], valor);
        SelectObject(hdc, g_fonte);
        SetTextColor(hdc, kCorMoeda[i]);
        RECT rt = {cx + 24, y, cx + largura - 6, y + kAltSaldos};
        DrawTextA(hdc, texto, -1, &rt, DT_LEFT | DT_VCENTER | DT_SINGLELINE | DT_NOPREFIX);
    }
}

void PintaMenu(HDC hdc) {
    for (int i = 0; i < kMenuTotal; ++i) {
        const RECT r = AreaMenu(i);
        const bool ativo = (i == g_filtro) && i <= kMenuMeus;
        LinhaBotao(hdc, r.left, r.top, kLargMenu, kAltBotao, ativo);
        SelectObject(hdc, ativo ? g_negrito : g_fonte);
        SetTextColor(hdc, ativo ? kTextoAtivo : kTexto);
        RECT rt = {r.left + 10, r.top, r.right - 10, r.bottom};
        DrawTextA(hdc, kMenu[i], -1, &rt, DT_CENTER | DT_VCENTER | DT_SINGLELINE | DT_NOPREFIX);
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
    Losango(hdc, r.left + kSlot / 2, r.top + 15, 9, kCorMoeda[o->moeda % 3]);
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

void PintaGrade(HDC hdc) {
    for (int i = 0; i < kPorPagina; ++i) {
        const RECT r = AreaSlot(i);
        const LojaOferta* o = LojaRedeOferta(i);
        PintaSlot(hdc, r, o, o != nullptr && i == g_escolhido);
    }
}

void PintaDetalhe(HDC hdc) {
    const int x = kBorda + kPad;
    const int y = kTopoConteudo + kAltGrade + 6;
    const int l = kLargura - (kBorda + kPad) * 2;
    Barra(hdc, x, y, l, 1, RGB(58, 48, 36));

    char texto[160];
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
    sprintf_s(pag, "%d/%d", g_pagina + 1, Paginas());
    SelectObject(hdc, g_fonte);
    SetTextColor(hdc, kTexto);
    RECT rp = {esq.right, y, dir.left, y + kAltRodape};
    DrawTextA(hdc, pag, -1, &rp, DT_CENTER | DT_VCENTER | DT_SINGLELINE | DT_NOPREFIX);

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
    TelaFecha(&g_tela, 179);   // os mesmos ~70% do painel de alvos
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

extern "C" void __cdecl LojaAnotaNo(DWORD no) {
    if (no < 0x10000) {
        return;
    }
    const float* r = reinterpret_cast<const float*>(no + kNoRetangulo);
    const float x = r[0];
    const float y = r[1];
    const float l = r[2];
    const float a = r[3];
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

bool BarraAberta() {
    const DWORD agora = GetTickCount();
    return g_faixaVistaEm != 0 && agora - g_faixaVistaEm < 200;
}

// --- cliques ---------------------------------------------------------------
void CliqueJanela(int x, int y) {
    const RECT f = AreaFechar();
    if (x >= f.left - 4 && x <= f.right + 4 && y >= f.top - 4 && y <= f.bottom + 4) {
        g_aberta = false;
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
                // A barraca continua sendo montada pela janela do proprio jogo:
                // e la que o jogador escolhe itens do cofre e digita precos. Nos
                // devolvemos o botao da barra por alguns segundos para que o
                // clique seguinte abra aquele fluxo; a moeda de cada item ele
                // escolhe depois, aqui, em "Meus itens".
                g_aberta = false;
                g_devolveBotaoAte = GetTickCount() + 6000;
                Log("=== loja: devolvendo o botao ao jogo para montar a barraca");
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
void VigiaBotaoDoJogo() {
    if (!EmJogo()) {
        return;
    }
    volatile WORD* ativa = reinterpret_cast<volatile WORD*>(kIdJanelaAtiva);

    if (*ativa != kIdLojaPessoal) {
        return;
    }
    *ativa = kIdNenhuma;
    if (!g_aberta) {
        g_aberta = true;
        g_escolhido = -1;
        PedeAoServidor();
        Repinta();
        Log("=== loja: botao da Loja Pessoal tomado, abrindo a nossa");
    }
}

int JanelaVisivel() {
    VigiaBotaoDoJogo();   // roda todo quadro: e aqui que o botao do jogo e tomado
    RenovaSePreciso();
    return g_aberta ? 1 : 0;
}

void JanelaMedida(int telaL, int telaA, int* x, int* y, int* largura, int* altura) {
    *largura = kLargura;
    *altura = kAltura;
    *x = (telaL - kLargura) / 2;
    *y = (telaA - kAltura) / 2 - 20;   // um pouco acima do centro, como no jogo
    if (*y < 0) {
        *y = 0;
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
    if (g_devolveBotaoAte != 0) {
        if (GetTickCount() < g_devolveBotaoAte) {
            return 0;   // o clique e do jogo: e assim que a barraca e montada
        }
        g_devolveBotaoAte = 0;
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

struct Registro {
    Registro() {
        CarregaConfig();
        DesviaAppendNode();
        CamadaRegistra(&kCamadaBotao);
        CamadaRegistra(&kCamadaJanela);
    }
};

Registro g_registro;

} // namespace
