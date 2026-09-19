// Painel de alvos desenhado DENTRO do quadro do jogo.
//
// Antes isto era uma janela em camada por cima do cliente. Funcionava, mas o
// jogo tratava o painel como coisa de fora: outra janela, outra camada, e nada
// aparecia em tela cheia exclusiva. O Marco pediu que ele ficasse integrado do
// mesmo jeito que o inventario.
//
// A saida nao foi voltar ao sistema de nos do cliente - nove tentativas ja
// tinham morrido ali. O desenho continua sendo nosso, em GDI, so que agora ele
// cai num DIB proprio; o modulo d3dpainel.cpp sobe esse DIB como textura e o
// desenha no backbuffer logo antes do EndScene. Mesma janela, mesma camada,
// mesmo Alt+Tab, e vale em tela cheia.
//
// O visual segue o layout que o Marco desenhou (stitch_wyd_enemy_target_ui):
// moldura escura com borda dupla e cantos dourados, cabecalho em degrade marrom
// com losangos, e cada linha no estilo dos botoes do jogo, com tampa metalica
// nas duas pontas. As cores sairam do CSS dele.

#include "camadas.h"
#include "pincel.h"

#include <windows.h>

#include <cstdio>
#include <cstring>

struct AlvoPainel {
    char nome[17];
    int dist;
    bool jogador;
    bool travado;
    int vidaPct;
    int rel;  // 0 monstro, 1 aliado, 2 inimigo
};

extern AlvoPainel g_painelLista[20];
extern int g_painelN;
extern bool g_painelAberto;
void AlvosCliqueNaLinha(int indice);
int AlvosOpcao(int qual);
void AlvosSetOpcao(int qual, int valor);
void AlvosFechaPainel();

namespace {

// --- medidas ---------------------------------------------------------------
// Compacto e no alto: o painel mora no canto superior direito, encostado.
constexpr int kLarguraColuna = 176;
constexpr int kAltLinha = 20;
constexpr int kEspacoLinha = 3;
constexpr int kAltCabecalho = 21;
constexpr int kAltRodape = 14;
constexpr int kPadding = 6;
constexpr int kBordaExterna = 5;     // as três linhas da borda dupla
constexpr int kEspacoColuna = 5;
constexpr int kMargemTopo = 6;
constexpr int kMargemDireita = 6;
constexpr int kMargemBaixo = 110;
// Passando disto, a lista quebra em DUAS COLUNAS em vez de esticar para baixo.
constexpr int kMaxPorColuna = 10;
constexpr int kMaxColunas = 2;
constexpr int kMaxLinhas = kMaxPorColuna * kMaxColunas;

// Cores proprias do painel: a relacao com o alvo. O resto da paleta esta em
// pincel.h, compartilhada com a loja.
const COLORREF kTextoAliado = RGB(126, 217, 139);  // verde: mesma capa que a minha
const COLORREF kTextoInimigo = RGB(255, 118, 108); // vermelho: outra capa, ou sem capa
const COLORREF kTextoMonstro = RGB(255, 232, 179); // o amarelado de sempre

const char* kOpcoes[] = {"Monstros", "Jogadores", "Ocultar aliados", "Alcance", "Ataque"};
constexpr int kNumOpcoes = 5;

Tela g_tela;                   // onde o painel e pintado
int g_telaL = 0;               // tamanho do backbuffer do jogo
int g_telaA = 0;
int g_versao = 0;              // sobe a cada repintura; a textura acompanha
DWORD g_ultimaPintura = 0;
HFONT g_fonte = nullptr;
HFONT g_fonteNegrito = nullptr;
bool g_opcoesAbertas = false;
int g_linhasVisiveis = 0;

// Quantas colunas a lista precisa agora.
int Colunas() {
    if (g_opcoesAbertas) {
        return 1;
    }
    const int n = g_painelN;
    int c = (n + kMaxPorColuna - 1) / kMaxPorColuna;
    if (c < 1) {
        c = 1;
    }
    return c > kMaxColunas ? kMaxColunas : c;
}

// A LARGURA DO PAINEL NAO MUDA. Com duas colunas, o espaço de uma é partido ao
// meio — era esse o pedido: encaixar as duas no mesmo lugar, e não dobrar a caixa.
int LarguraPainel() {
    return kBordaExterna * 2 + kPadding * 2 + kLarguraColuna;
}

int LarguraDaColuna() {
    const int c = Colunas();
    return c <= 1 ? kLarguraColuna : (kLarguraColuna - kEspacoColuna) / 2;
}

int LinhasQueCabem() {
    if (g_telaA <= 0) {
        return 8;
    }
    const int fixo = kBordaExterna * 2 + kPadding * 2 + kAltCabecalho + kAltRodape;
    const int espaco = g_telaA - kMargemTopo - kMargemBaixo - fixo;
    int cabem = espaco / (kAltLinha + kEspacoLinha);
    if (cabem < 1) {
        cabem = 1;
    }
    if (cabem > kMaxPorColuna) {
        cabem = kMaxPorColuna;   // daqui em diante a lista cresce em coluna, não em altura
    }
    return cabem;
}

int AlturaCaixa() {
    int linhas = g_opcoesAbertas ? kNumOpcoes : (g_linhasVisiveis > 0 ? g_linhasVisiveis : 1);
    if (!g_opcoesAbertas) {
        const int n = g_painelN < kMaxLinhas ? g_painelN : kMaxLinhas;
        const int porColuna = (n + Colunas() - 1) / (Colunas() > 0 ? Colunas() : 1);
        if (porColuna > 0 && porColuna < linhas) {
            linhas = porColuna;   // duas colunas: a caixa encolhe de volta
        }
    }
    return kBordaExterna * 2 + kPadding * 2 + kAltCabecalho + kAltRodape +
           (kAltLinha + kEspacoLinha) * linhas;
}

// Canto superior direito, agora em coordenadas do backbuffer.
int PosX() {
    return g_telaL - kMargemDireita - LarguraPainel();
}

int PosY() {
    return kMargemTopo;
}

// A caixa do cabecalho vai de kBordaExterna+kPadding ate +17, com centro em 19.
// As tres barrinhas ocupam dez pixels a partir do topo desta area, entao ela
// comeca em +2 para que barrinhas e X fiquem na mesma altura do titulo.
RECT AreaEngrenagem() {
    RECT r = {LarguraPainel() - kBordaExterna - kPadding - 16, kBordaExterna + kPadding + 2,
              LarguraPainel() - kBordaExterna - kPadding, kBordaExterna + kPadding + 15};
    return r;
}

RECT AreaFechar() {
    RECT g = AreaEngrenagem();
    RECT r = {g.left - 22, g.top, g.left - 6, g.bottom};
    return r;
}

void PintaCabecalho(HDC hdc) {
    const int x = kBordaExterna + kPadding;
    const int y = kBordaExterna + kPadding;
    const int l = LarguraPainel() - (kBordaExterna + kPadding) * 2;

    Degrade(hdc, x, y, l, kAltCabecalho - 4, kCabTopo, kCabBaixo);
    Contorno(hdc, x, y, l, kAltCabecalho - 4, kCabBorda);
    Losango(hdc, x + 9, y + (kAltCabecalho - 4) / 2, 3, kLosango);

    SelectObject(hdc, g_fonteNegrito);
    SetTextColor(hdc, RGB(255, 255, 255));
    char titulo[40];
    sprintf_s(titulo, g_opcoesAbertas ? "Opcoes" : "Alvos (%d)", g_painelN);
    RECT rt = {x + 18, y, x + l - 46, y + kAltCabecalho - 4};
    DrawTextA(hdc, titulo, -1, &rt, DT_LEFT | DT_VCENTER | DT_SINGLELINE | DT_NOPREFIX);

    // engrenagem (três barrinhas) e o X, ambos em tom dourado
    const RECT g = AreaEngrenagem();
    const COLORREF cor = g_opcoesAbertas ? kTextoAtivo : kLosango;
    for (int i = 0; i < 3; ++i) {
        Barra(hdc, g.left + 2, g.top + 1 + i * 4, g.right - g.left - 4, 2, cor);
    }
    const RECT f = AreaFechar();
    SelectObject(hdc, g_fonte);
    SetTextColor(hdc, kLosango);
    RECT rx = {f.left, f.top - 3, f.right, f.bottom + 3};
    DrawTextA(hdc, "X", -1, &rx, DT_CENTER | DT_VCENTER | DT_SINGLELINE | DT_NOPREFIX);
}

int TopoDasLinhas() {
    return kBordaExterna + kPadding + kAltCabecalho + 2;
}

void PintaAlvos(HDC hdc) {
    SelectObject(hdc, g_fonteNegrito);
    const int x0 = kBordaExterna + kPadding;
    const int n = g_painelN < kMaxLinhas ? g_painelN : kMaxLinhas;
    const int cols = Colunas();
    const int porColuna = (n + cols - 1) / (cols > 0 ? cols : 1);

    for (int i = 0; i < n; ++i) {
        const AlvoPainel& a = g_painelLista[i];
        const int col = porColuna > 0 ? i / porColuna : 0;
        const int lin = porColuna > 0 ? i % porColuna : i;
        const int lc = LarguraDaColuna();
        const int x = x0 + col * (lc + kEspacoColuna);
        const int y = TopoDasLinhas() + (kAltLinha + kEspacoLinha) * lin;
        LinhaBotao(hdc, x, y, lc, kAltLinha, a.travado);

        SelectObject(hdc, g_fonteNegrito);
        // A cor diz a relação, mesmo com o alvo travado: perder isso na hora da
        // luta seria perder justamente quando importa.
        const COLORREF corDaLinha = a.rel == 1 ? kTextoAliado
                                  : (a.rel == 2 ? kTextoInimigo : kTextoMonstro);
        SetTextColor(hdc, corDaLinha);
        char esquerda[40];
        // Com a coluna partida ao meio sobra menos espaço: o nome encurta, e o
        // DT_END_ELLIPSIS corta com reticências em vez de invadir a distância.
        sprintf_s(esquerda, "%d %.*s", (i + 1) % 10, Colunas() > 1 ? 9 : 12, a.nome);
        RECT re = {x + kTampa + 5, y, x + lc - kTampa - 22, y + kAltLinha};
        DrawTextA(hdc, esquerda, -1, &re, DT_LEFT | DT_VCENTER | DT_SINGLELINE | DT_NOPREFIX | DT_END_ELLIPSIS);

        char direita[12];
        sprintf_s(direita, "%d", a.dist);
        SetTextColor(hdc, a.travado ? kTextoAtivo : kTextoFraco);
        RECT rd = {x + lc - kTampa - 22, y, x + lc - kTampa - 3, y + kAltLinha};
        DrawTextA(hdc, direita, -1, &rd, DT_RIGHT | DT_VCENTER | DT_SINGLELINE | DT_NOPREFIX);
    }
    if (n == 0) {
        SelectObject(hdc, g_fonte);
        SetTextColor(hdc, kTextoFraco);
        RECT rv = {x0, TopoDasLinhas(), x0 + LarguraDaColuna(), TopoDasLinhas() + kAltLinha};
        DrawTextA(hdc, "nada por perto", -1, &rv, DT_CENTER | DT_VCENTER | DT_SINGLELINE | DT_NOPREFIX);
    }
}

void PintaOpcoes(HDC hdc) {
    const int x = kBordaExterna + kPadding;
    const int l = LarguraPainel() - (kBordaExterna + kPadding) * 2;
    for (int i = 0; i < kNumOpcoes; ++i) {
        const int y = TopoDasLinhas() + (kAltLinha + kEspacoLinha) * i;
        const int valor = AlvosOpcao(i);
        LinhaBotao(hdc, x, y, l, kAltLinha, false);

        if (i < 3) {
            Barra(hdc, x + kTampa + 8, y + kAltLinha / 2 - 4, 9, 9, valor ? kMarcaOn : kMarcaOff);
            Contorno(hdc, x + kTampa + 8, y + kAltLinha / 2 - 4, 9, 9, kTampaBorda);
        }
        SelectObject(hdc, g_fonteNegrito);
        SetTextColor(hdc, kTexto);
        RECT rt = {x + kTampa + (i < 3 ? 23 : 8), y, x + l - kTampa - 60, y + kAltLinha};
        DrawTextA(hdc, kOpcoes[i], -1, &rt, DT_LEFT | DT_VCENTER | DT_SINGLELINE | DT_NOPREFIX);

        if (i == 3 || i == 4) {
            char v[24];
            if (i == 3) {
                sprintf_s(v, "-  %d  +", valor);
            } else {
                // Físico ou mágico é escolha SUA: em PvP o macro fica no modo de
                // poções, e ali o tipo dele não diz nada.
                sprintf_s(v, "%s", valor == 2 ? "Magico" : (valor == 1 ? "Fisico" : "Auto"));
            }
            SetTextColor(hdc, kTextoAtivo);
            RECT rd = {x + l - kTampa - 62, y, x + l - kTampa - 6, y + kAltLinha};
            DrawTextA(hdc, v, -1, &rd, DT_RIGHT | DT_VCENTER | DT_SINGLELINE | DT_NOPREFIX);
        }
    }
}

void PintaRodape(HDC hdc) {
    const int x = kBordaExterna + kPadding;
    const int l = LarguraPainel() - (kBordaExterna + kPadding) * 2;
    const int linhas = g_opcoesAbertas ? kNumOpcoes : (g_linhasVisiveis > 0 ? g_linhasVisiveis : 1);
    const int y = TopoDasLinhas() + (kAltLinha + kEspacoLinha) * linhas;

    Barra(hdc, x, y, l, 1, RGB(58, 48, 36));
    SelectObject(hdc, g_fonte);
    SetTextColor(hdc, RGB(120, 108, 90));
    RECT rt = {x, y + 2, x + l, y + kAltRodape};
    DrawTextA(hdc, "'  escanear      CapsLock  soltar", -1, &rt,
              DT_CENTER | DT_VCENTER | DT_SINGLELINE | DT_NOPREFIX);
}

void Pinta(HDC hdc) {
    SetBkMode(hdc, TRANSPARENT);
    Moldura(hdc, g_tela.l, g_tela.a);
    PintaCabecalho(hdc);
    if (g_opcoesAbertas) {
        PintaOpcoes(hdc);
    } else {
        PintaAlvos(hdc);
    }
    PintaRodape(hdc);
}

void Clique(int x, int y) {
    const RECT g = AreaEngrenagem();
    if (y < TopoDasLinhas() - 2) {
        const RECT f = AreaFechar();
        if (x >= g.left - 4) {
            g_opcoesAbertas = !g_opcoesAbertas;
        } else if (x >= f.left - 3 && x <= f.right + 3) {
            AlvosFechaPainel();
            g_opcoesAbertas = false;
        }
        return;
    }
    const int i = (y - TopoDasLinhas()) / (kAltLinha + kEspacoLinha);
    if (i < 0) {
        return;
    }
    if (g_opcoesAbertas) {
        if (i >= kNumOpcoes) {
            return;
        }
        if (i == 3) {
            const int atual = AlvosOpcao(3);
            AlvosSetOpcao(3, x > LarguraPainel() - 40 ? atual + 1 : atual - 1);
        } else if (i == 4) {
            AlvosSetOpcao(4, AlvosOpcao(4) == 2 ? 1 : 2);
        } else {
            AlvosSetOpcao(i, AlvosOpcao(i) ? 0 : 1);
        }
        return;
    }
    const int n = g_painelN < kMaxLinhas ? g_painelN : kMaxLinhas;
    const int cols = Colunas();
    const int porColuna = (n + cols - 1) / (cols > 0 ? cols : 1);
    const int col = (x - kBordaExterna - kPadding) / (LarguraDaColuna() + kEspacoColuna);
    const int escolhido = col * porColuna + i;
    if (col >= 0 && col < cols && i < porColuna && escolhido < n) {
        AlvosCliqueNaLinha(escolhido);
    }
}

// --- o painel virou pixels --------------------------------------------------

void CriaFontes() {
    if (g_fonte != nullptr) {
        return;
    }
    g_fonte = CreateFontA(13, 0, 0, 0, FW_NORMAL, FALSE, FALSE, FALSE, DEFAULT_CHARSET,
                          OUT_DEFAULT_PRECIS, CLIP_DEFAULT_PRECIS, ANTIALIASED_QUALITY,
                          DEFAULT_PITCH | FF_DONTCARE, "Segoe UI");
    g_fonteNegrito = CreateFontA(13, 0, 0, 0, FW_BOLD, FALSE, FALSE, FALSE, DEFAULT_CHARSET,
                                 OUT_DEFAULT_PRECIS, CLIP_DEFAULT_PRECIS, ANTIALIASED_QUALITY,
                                 DEFAULT_PITCH | FF_DONTCARE, "Segoe UI");
}

// Repinta no maximo a cada 80 ms: o conteudo muda no ritmo da varredura, nao no
// ritmo do quadro, e pintar em GDI 60 vezes por segundo seria desperdicio.
void RepintaSeNecessario(bool forcado) {
    CriaFontes();
    g_linhasVisiveis = LinhasQueCabem();
    const int l = LarguraPainel();
    const int a = AlturaCaixa();
    const DWORD agora = GetTickCount();
    const bool mesmoTamanho = (g_tela.pixels != nullptr && l == g_tela.l && a == g_tela.a);
    if (!forcado && mesmoTamanho && agora - g_ultimaPintura < 80) {
        return;
    }
    if (!TelaGarante(&g_tela, l, a)) {
        return;
    }
    Pinta(g_tela.dc);
    TelaFecha(&g_tela, 179);   // ~70%, como o Marco pediu
    g_ultimaPintura = agora;
    ++g_versao;
}

} // namespace

// --- a camada ---------------------------------------------------------------

// Ditas para o alvos.cpp, que precisa delas no Esc: o painel so some de vez
// quando a gaveta de opcoes fecha junto.
int OverlayOpcoesAbertas() {
    return g_opcoesAbertas ? 1 : 0;
}

void OverlayFechaOpcoes() {
    g_opcoesAbertas = false;
}

namespace {

int PainelVisivel() {
    return (g_painelAberto || g_opcoesAbertas) ? 1 : 0;
}

void PainelMedida(int telaL, int telaA, int* x, int* y, int* largura, int* altura) {
    g_telaL = telaL;
    g_telaA = telaA;
    CriaFontes();
    g_linhasVisiveis = LinhasQueCabem();
    *largura = LarguraPainel();
    *altura = AlturaCaixa();
    *x = PosX();          // canto superior direito, encostado
    *y = PosY();
}

const void* PainelPixels(int* versao) {
    RepintaSeNecessario(false);
    *versao = g_versao;
    return g_tela.pixels;
}

void PainelClique(int x, int y) {
    Clique(x, y);
    RepintaSeNecessario(true);   // a resposta ao clique entra no quadro seguinte
}

const Camada kCamadaPainel = {10, PainelVisivel, PainelMedida, PainelPixels, PainelClique};

struct Registro {
    Registro() { CamadaRegistra(&kCamadaPainel); }
};

Registro g_registro;

} // namespace

// O painel desenhado pelo sistema de nos do cliente saiu de cena; estes dois
// existem so para o tique do macro continuar chamando sem mudar o outro modulo.
void PainelInstala() {}
void PainelNovoQuadro() {}
