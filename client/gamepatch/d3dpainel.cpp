// Desenha o painel de alvos DENTRO do quadro do jogo, em Direct3D 9.
//
// O cliente importa Direct3DCreate9 e desenha tudo por um IDirect3DDevice9. No
// Win32 a tabela virtual de um objeto COM mora na d3d9.dll e e a MESMA para
// todos os dispositivos: basta criar um dispositivo de mentira uma unica vez,
// ler a tabela dele e trocar duas entradas para que as chamadas do jogo passem
// por aqui.
//
//   slot 16 -> Reset       (a tela mudou de tamanho; soltar o que e nosso)
//   slot 42 -> EndScene    (o quadro esta pronto; o painel entra por ultimo)
//
// Metodos COM em x86 sao __stdcall com o ponteiro do objeto como primeiro
// argumento na pilha, que e exatamente a assinatura usada abaixo.
//
// O painel chega pronto do overlay.cpp: um retangulo BGRA pintado em GDI. Aqui
// ele vira uma textura A8R8G8B8 e um retangulo de dois triangulos em coordenadas
// de tela. Por isso o painel se comporta como o inventario - e parte do quadro,
// nao uma janela por cima.

#include <windows.h>
#include <d3d9.h>

#include <cstdio>
#include <cstring>

#include "camadas.h"

namespace {
bool g_okDoDesenho = true;
}

namespace {

typedef HRESULT(WINAPI* EndSceneFn)(IDirect3DDevice9*);

constexpr int kSlotEndScene = 42;

IDirect3DDevice9* g_dev = nullptr;

// Uma textura por camada: elas mudam em ritmos diferentes e cada uma so e
// reenviada quando a sua versao muda.
struct Tex {
    IDirect3DTexture9* tex;
    int l;
    int a;
    int versao;
};

Tex g_tex[8];

// Para ler pixels do quadro: um alvo de 1x1 na placa e uma copia na memoria.
IDirect3DSurface9* g_pontoGpu = nullptr;
IDirect3DSurface9* g_pontoCpu = nullptr;
DWORD g_ultimaAmostra = 0;

struct Vertice {
    float x, y, z, rhw;
    float u, v;
};

constexpr DWORD kFvf = D3DFVF_XYZRHW | D3DFVF_TEX1;

void Log(const char* texto) {
    char caminho[MAX_PATH];
    GetModuleFileNameA(nullptr, caminho, MAX_PATH);
    char* barra = strrchr(caminho, 92);
    if (barra == nullptr) {
        return;
    }
    strcpy_s(barra + 1, MAX_PATH - (barra + 1 - caminho), "alvos.log");
    FILE* f = nullptr;
    if (fopen_s(&f, caminho, "a") == 0 && f != nullptr) {
        fputs(texto, f);
        fputc(10, f);
        fclose(f);
    }
}

// De qual modulo veio um endereco: serve para conferir, no log, que a funcao
// desviada e mesmo a da d3d9.dll.
void ModuloDe(void* p, char* saida, size_t n) {
    HMODULE mod = nullptr;
    if (GetModuleHandleExA(GET_MODULE_HANDLE_EX_FLAG_FROM_ADDRESS |
                               GET_MODULE_HANDLE_EX_FLAG_UNCHANGED_REFCOUNT,
                           reinterpret_cast<LPCSTR>(p), &mod) &&
        mod != nullptr) {
        char caminho[MAX_PATH];
        if (GetModuleFileNameA(mod, caminho, MAX_PATH) != 0) {
            const char* barra = strrchr(caminho, 92);
            strcpy_s(saida, n, barra != nullptr ? barra + 1 : caminho);
            return;
        }
    }
    strcpy_s(saida, n, "(sem modulo)");
}

bool GaranteTextura(IDirect3DDevice9* dev, Tex* t, int l, int a) {
    if (t->tex != nullptr && l == t->l && a == t->a) {
        return true;
    }
    if (t->tex != nullptr) {
        t->tex->Release();
        t->tex = nullptr;
    }
    // MANAGED: sobrevive a um Reset e ainda assim pode ser travada para escrita.
    if (FAILED(dev->CreateTexture(l, a, 1, 0, D3DFMT_A8R8G8B8, D3DPOOL_MANAGED, &t->tex, nullptr))) {
        t->tex = nullptr;
        return false;
    }
    t->l = l;
    t->a = a;
    t->versao = -1;
    return true;
}

void SobeTextura(IDirect3DTexture9* tex, const void* pixels, int l, int a) {
    D3DLOCKED_RECT lr;
    if (FAILED(tex->LockRect(0, &lr, nullptr, 0))) {
        return;
    }
    const BYTE* orig = static_cast<const BYTE*>(pixels);
    BYTE* dest = static_cast<BYTE*>(lr.pBits);
    for (int y = 0; y < a; ++y) {
        memcpy(dest + static_cast<size_t>(y) * lr.Pitch, orig + static_cast<size_t>(y) * l * 4,
               static_cast<size_t>(l) * 4);
    }
    tex->UnlockRect(0);
}

// --- guardar e devolver o estado do dispositivo ----------------------------
//
// Um IDirect3DStateBlock9 seria o caminho curto, mas ele nao sobrevive a um
// Reset do dispositivo, e este cliente troca de dispositivo e de modo mais de
// uma vez. Entao os poucos estados que mexemos sao lidos e devolvidos na mao,
// que nao depende de objeto nenhum.

struct Estado {
    DWORD rs[14];
    DWORD tss0[4];
    DWORD tss1[2];
    DWORD samp[6];
    DWORD fvf;
    IDirect3DVertexShader9* vs;
    IDirect3DPixelShader9* ps;
    IDirect3DBaseTexture9* tex;
    IDirect3DVertexBuffer9* fluxo;
    UINT fluxoOff;
    UINT fluxoPasso;
    IDirect3DIndexBuffer9* indices;
};

const D3DRENDERSTATETYPE kRs[14] = {
    D3DRS_ALPHABLENDENABLE, D3DRS_SRCBLEND,   D3DRS_DESTBLEND,     D3DRS_ALPHATESTENABLE,
    D3DRS_ZENABLE,          D3DRS_ZWRITEENABLE, D3DRS_STENCILENABLE, D3DRS_SCISSORTESTENABLE,
    D3DRS_CULLMODE,         D3DRS_LIGHTING,   D3DRS_FOGENABLE,     D3DRS_CLIPPING,
    D3DRS_COLORWRITEENABLE, D3DRS_SRGBWRITEENABLE};

const D3DTEXTURESTAGESTATETYPE kTss0[4] = {D3DTSS_COLOROP, D3DTSS_COLORARG1, D3DTSS_ALPHAOP,
                                           D3DTSS_ALPHAARG1};
const D3DTEXTURESTAGESTATETYPE kTss1[2] = {D3DTSS_COLOROP, D3DTSS_ALPHAOP};
const D3DSAMPLERSTATETYPE kSamp[6] = {D3DSAMP_MINFILTER, D3DSAMP_MAGFILTER, D3DSAMP_MIPFILTER,
                                      D3DSAMP_ADDRESSU,  D3DSAMP_ADDRESSV,  D3DSAMP_SRGBTEXTURE};

void GuardaEstado(IDirect3DDevice9* dev, Estado* e) {
    memset(e, 0, sizeof(*e));
    for (int i = 0; i < 14; ++i) {
        dev->GetRenderState(kRs[i], &e->rs[i]);
    }
    for (int i = 0; i < 4; ++i) {
        dev->GetTextureStageState(0, kTss0[i], &e->tss0[i]);
    }
    for (int i = 0; i < 2; ++i) {
        dev->GetTextureStageState(1, kTss1[i], &e->tss1[i]);
    }
    for (int i = 0; i < 6; ++i) {
        dev->GetSamplerState(0, kSamp[i], &e->samp[i]);
    }
    dev->GetFVF(&e->fvf);
    dev->GetVertexShader(&e->vs);
    dev->GetPixelShader(&e->ps);
    dev->GetTexture(0, &e->tex);
    dev->GetStreamSource(0, &e->fluxo, &e->fluxoOff, &e->fluxoPasso);
    dev->GetIndices(&e->indices);
}

void DevolveEstado(IDirect3DDevice9* dev, Estado* e) {
    for (int i = 0; i < 14; ++i) {
        dev->SetRenderState(kRs[i], e->rs[i]);
    }
    for (int i = 0; i < 4; ++i) {
        dev->SetTextureStageState(0, kTss0[i], e->tss0[i]);
    }
    for (int i = 0; i < 2; ++i) {
        dev->SetTextureStageState(1, kTss1[i], e->tss1[i]);
    }
    for (int i = 0; i < 6; ++i) {
        dev->SetSamplerState(0, kSamp[i], e->samp[i]);
    }
    dev->SetFVF(e->fvf);
    dev->SetVertexShader(e->vs);
    dev->SetPixelShader(e->ps);
    dev->SetTexture(0, e->tex);
    // DrawPrimitiveUP desfaz a ligacao do fluxo 0; devolvemos a que estava.
    dev->SetStreamSource(0, e->fluxo, e->fluxoOff, e->fluxoPasso);
    dev->SetIndices(e->indices);
    if (e->vs != nullptr) {
        e->vs->Release();
    }
    if (e->ps != nullptr) {
        e->ps->Release();
    }
    if (e->tex != nullptr) {
        e->tex->Release();
    }
    if (e->fluxo != nullptr) {
        e->fluxo->Release();
    }
    if (e->indices != nullptr) {
        e->indices->Release();
    }
}

void AjustaEstado(IDirect3DDevice9* dev) {
    dev->SetVertexShader(nullptr);
    dev->SetPixelShader(nullptr);
    dev->SetFVF(kFvf);
    dev->SetRenderState(D3DRS_ALPHABLENDENABLE, TRUE);
    dev->SetRenderState(D3DRS_SRCBLEND, D3DBLEND_SRCALPHA);
    dev->SetRenderState(D3DRS_DESTBLEND, D3DBLEND_INVSRCALPHA);
    dev->SetRenderState(D3DRS_ALPHATESTENABLE, FALSE);
    dev->SetRenderState(D3DRS_ZENABLE, FALSE);
    dev->SetRenderState(D3DRS_ZWRITEENABLE, FALSE);
    dev->SetRenderState(D3DRS_STENCILENABLE, FALSE);
    dev->SetRenderState(D3DRS_SCISSORTESTENABLE, FALSE);
    dev->SetRenderState(D3DRS_CULLMODE, D3DCULL_NONE);
    dev->SetRenderState(D3DRS_LIGHTING, FALSE);
    dev->SetRenderState(D3DRS_FOGENABLE, FALSE);
    dev->SetRenderState(D3DRS_CLIPPING, TRUE);
    dev->SetRenderState(D3DRS_COLORWRITEENABLE, 0x0F);
    dev->SetRenderState(D3DRS_SRGBWRITEENABLE, FALSE);

    dev->SetTextureStageState(0, D3DTSS_COLOROP, D3DTOP_SELECTARG1);
    dev->SetTextureStageState(0, D3DTSS_COLORARG1, D3DTA_TEXTURE);
    dev->SetTextureStageState(0, D3DTSS_ALPHAOP, D3DTOP_SELECTARG1);
    dev->SetTextureStageState(0, D3DTSS_ALPHAARG1, D3DTA_TEXTURE);
    dev->SetTextureStageState(1, D3DTSS_COLOROP, D3DTOP_DISABLE);
    dev->SetTextureStageState(1, D3DTSS_ALPHAOP, D3DTOP_DISABLE);

    // Ponto, e nao linear: o painel e desenhado no tamanho exato, e assim o
    // texto sai com a mesma nitidez que tinha na janela.
    dev->SetSamplerState(0, D3DSAMP_MINFILTER, D3DTEXF_POINT);
    dev->SetSamplerState(0, D3DSAMP_MAGFILTER, D3DTEXF_POINT);
    dev->SetSamplerState(0, D3DSAMP_MIPFILTER, D3DTEXF_NONE);
    dev->SetSamplerState(0, D3DSAMP_ADDRESSU, D3DTADDRESS_CLAMP);
    dev->SetSamplerState(0, D3DSAMP_ADDRESSV, D3DTADDRESS_CLAMP);
    dev->SetSamplerState(0, D3DSAMP_SRGBTEXTURE, FALSE);
}

// Um pedaco da camada, com as coordenadas de textura tiradas de onde ele esta
// dentro dela.
void DesenhaPedaco(IDirect3DDevice9* dev, int cx, int cy, int cl, int ca, int px, int py,
                   int pl, int pa) {
    if (pl <= 0 || pa <= 0) {
        return;
    }
    // -0.5 em cada eixo: e a regra do D3D9 para casar texel com pixel.
    const float x0 = static_cast<float>(px) - 0.5f;
    const float y0 = static_cast<float>(py) - 0.5f;
    const float x1 = x0 + static_cast<float>(pl);
    const float y1 = y0 + static_cast<float>(pa);
    const float u0 = static_cast<float>(px - cx) / static_cast<float>(cl);
    const float v0 = static_cast<float>(py - cy) / static_cast<float>(ca);
    const float u1 = static_cast<float>(px - cx + pl) / static_cast<float>(cl);
    const float v1 = static_cast<float>(py - cy + pa) / static_cast<float>(ca);
    const Vertice v[4] = {
        {x0, y0, 0.0f, 1.0f, u0, v0},
        {x1, y0, 0.0f, 1.0f, u1, v0},
        {x0, y1, 0.0f, 1.0f, u0, v1},
        {x1, y1, 0.0f, 1.0f, u1, v1},
    };
    if (FAILED(dev->DrawPrimitiveUP(D3DPT_TRIANGLESTRIP, 2, v, sizeof(Vertice)))) {
        g_okDoDesenho = false;
    }
}

void DesenhaCamada(IDirect3DDevice9* dev, const Camada* c, Tex* t, int telaL, int telaA) {
    int x = 0;
    int y = 0;
    int l = 0;
    int a = 0;
    c->medida(telaL, telaA, &x, &y, &l, &a);
    if (l <= 0 || a <= 0) {
        return;
    }
    int versao = 0;
    const void* pixels = c->pixels(&versao);
    if (pixels == nullptr) {
        return;
    }
    if (!GaranteTextura(dev, t, l, a)) {
        return;
    }
    if (versao != t->versao) {
        SobeTextura(t->tex, pixels, l, a);
        t->versao = versao;
    }
    dev->SetTexture(0, t->tex);
    DesenhaPedaco(dev, x, y, l, a, x, y, l, a);
}

void Desenha(IDirect3DDevice9* dev) {
    D3DVIEWPORT9 vp;
    if (FAILED(dev->GetViewport(&vp)) || vp.Width == 0 || vp.Height == 0) {
        return;
    }
    if (dev != g_dev) {
        // Texturas do dispositivo anterior nao valem aqui, e solta-las agora
        // seria mexer em algo que talvez ja tenha morrido: soltamos a mao.
        g_dev = dev;
        memset(g_tex, 0, sizeof(g_tex));
        g_pontoGpu = nullptr;
        g_pontoCpu = nullptr;
    }
    const int telaL = static_cast<int>(vp.Width);
    const int telaA = static_cast<int>(vp.Height);
    CamadaTela(telaL, telaA);

    // Pergunta a TODAS, sem parar na primeira que responder que sim: visivel()
    // nao e so uma pergunta - e nela que as camadas acertam o estado do quadro
    // (a loja fecha a janela da lojinha antiga, decide de quem e o teclado e
    // descobre o item sob o cursor). Parar no meio deixava as de cima sem rodar
    // justamente quando uma de baixo estava aberta.
    const int total = CamadaTotal();
    bool algumaVisivel = false;
    for (int i = 0; i < total; ++i) {
        if (CamadaEm(i)->visivel() != 0) {
            algumaVisivel = true;
        }
    }
    if (!algumaVisivel) {
        return;
    }

    // O jogo deixa o dispositivo do jeito dele; guardamos tudo e devolvemos
    // igual, senao o quadro seguinte sai com as nossas regras de mistura.
    Estado antes;
    GuardaEstado(dev, &antes);
    AjustaEstado(dev);

    for (int i = 0; i < total && i < 8; ++i) {
        const Camada* c = CamadaEm(i);
        if (c->visivel() != 0) {
            DesenhaCamada(dev, c, &g_tex[i], telaL, telaA);
        }
    }

    DevolveEstado(dev, &antes);
}

// --- o desvio, agora no codigo da d3d9.dll ---------------------------------
//
// Trocar o ponteiro na tabela virtual do dispositivo funcionou por dois quadros
// e foi desfeito: a tabela deste dispositivo mora no heap e o proprio D3D a
// reescreve. O desvio passou entao para o inicio da funcao EndScene, dentro da
// d3d9.dll. Nao importa quantas tabelas existam nem quem as restaure: todas
// apontam para esse mesmo codigo.
//
// O inicio da funcao e "push 0x14; mov eax, imm32" (7 bytes, nenhum relativo),
// entao esses 7 bytes sao copiados para um trecho nosso, seguidos de um salto de
// volta - e no lugar deles entra o salto para ca.

EndSceneFn g_endSceneTramp = nullptr;

// Le as cores pedidas pelas camadas. Chamada DEPOIS do EndScene porque o D3D
// recusa StretchRect e GetRenderTargetData dentro da cena. A cada 250 ms: os
// pontos sao poucos e de 1x1, mas cada leitura obriga a CPU a esperar a placa.
void LeAmostras(IDirect3DDevice9* dev) {
    const DWORD agora = GetTickCount();
    if (AmostraConsomeUrgencia() == 0 && agora - g_ultimaAmostra < 120) {
        return;
    }
    g_ultimaAmostra = agora;

    IDirect3DSurface9* quadro = nullptr;
    if (FAILED(dev->GetBackBuffer(0, 0, D3DBACKBUFFER_TYPE_MONO, &quadro)) || quadro == nullptr) {
        return;
    }
    D3DSURFACE_DESC desc;
    quadro->GetDesc(&desc);
    // O alvo de 1x1 tem de nascer no MESMO formato do quadro, senao o D3D recusa
    // a copia; e com multiamostragem o StretchRect nem e permitido.
    if (g_pontoGpu == nullptr) {
        const HRESULT hr = dev->CreateRenderTarget(1, 1, desc.Format, D3DMULTISAMPLE_NONE, 0,
                                                   FALSE, &g_pontoGpu, nullptr);
        if (FAILED(hr)) {
            g_pontoGpu = nullptr;
            quadro->Release();
            return;
        }
    }
    if (g_pontoCpu == nullptr) {
        const HRESULT hr = dev->CreateOffscreenPlainSurface(1, 1, desc.Format, D3DPOOL_SYSTEMMEM,
                                                            &g_pontoCpu, nullptr);
        if (FAILED(hr)) {
            g_pontoCpu = nullptr;
            quadro->Release();
            return;
        }
    }
    for (int i = 0; i < kMaxAmostras; ++i) {
        int x = 0;
        int y = 0;
        if (!AmostraPonto(i, &x, &y) || x < 0 || y < 0 || x + 1 >= static_cast<int>(desc.Width) ||
            y + 1 >= static_cast<int>(desc.Height)) {
            continue;
        }
        RECT r = {x, y, x + 1, y + 1};
        HRESULT hr = dev->StretchRect(quadro, &r, g_pontoGpu, nullptr, D3DTEXF_NONE);
        if (FAILED(hr)) {
            continue;
        }
        hr = dev->GetRenderTargetData(g_pontoGpu, g_pontoCpu);
        if (FAILED(hr)) {
            continue;
        }
        D3DLOCKED_RECT lr;
        if (SUCCEEDED(g_pontoCpu->LockRect(&lr, nullptr, D3DLOCK_READONLY))) {
            const DWORD cor = *static_cast<DWORD*>(lr.pBits) & 0x00FFFFFF;
            AmostraGuarda(i, cor);
            g_pontoCpu->UnlockRect();
        }
    }
    quadro->Release();
}

// Quem manda desenhar agora e o laco de camadas do cliente: ver loja.cpp.
IDirect3DDevice9* g_devDoQuadro = nullptr;
bool g_desenhadoNoQuadro = false;

extern "C" void __cdecl D3DDesenhaCamadasAgora() {
    if (g_desenhadoNoQuadro || g_devDoQuadro == nullptr) {
        return;
    }
    g_desenhadoNoQuadro = true;
    Desenha(g_devDoQuadro);
}

// O desenho sai no fim do quadro, no EndScene.
//
// Ja tentei sair antes, na primeira peca da interface (o AppendNode), para
// ficar sob as janelas do cliente e deixar a caixa de informacao do item por
// cima. Nao funciona: naquele ponto o mundo ainda nao foi desenhado, e ele
// passa por cima do painel - a loja simplesmente sumia. A ordem do cliente e
// montar a interface primeiro e desenhar tudo depois.
HRESULT WINAPI MeuEndScene(IDirect3DDevice9* dev) {
    g_devDoQuadro = dev;
    if (!g_desenhadoNoQuadro) {
        Desenha(dev);
    }
    g_desenhadoNoQuadro = false;
    const HRESULT hr = g_endSceneTramp(dev);
    LeAmostras(dev);
    return hr;
}

bool DesviaCodigo(void* alvo) {
    BYTE* f = static_cast<BYTE*>(alvo);
    if (!(f[0] == 0x6A && f[2] == 0xB8)) {
        char buf[120];
        sprintf_s(buf, "=== d3d prologo inesperado: %02X %02X %02X", f[0], f[1], f[2]);
        Log(buf);
        return false;
    }
    constexpr int kRoubados = 7;   // push imm8 (2) + mov eax, imm32 (5)

    BYTE* ponte = static_cast<BYTE*>(
        VirtualAlloc(nullptr, 32, MEM_COMMIT | MEM_RESERVE, PAGE_EXECUTE_READWRITE));
    if (ponte == nullptr) {
        return false;
    }
    memcpy(ponte, f, kRoubados);
    ponte[kRoubados] = 0xE9;
    const DWORD voltaRel = static_cast<DWORD>((f + kRoubados) - (ponte + kRoubados + 5));
    memcpy(ponte + kRoubados + 1, &voltaRel, sizeof(voltaRel));

    BYTE salto[kRoubados];
    memset(salto, 0x90, sizeof(salto));
    salto[0] = 0xE9;
    const DWORD idaRel = static_cast<DWORD>(reinterpret_cast<BYTE*>(&MeuEndScene) - (f + 5));
    memcpy(salto + 1, &idaRel, sizeof(idaRel));

    DWORD antes = 0;
    if (!VirtualProtect(f, kRoubados, PAGE_EXECUTE_READWRITE, &antes)) {
        return false;
    }
    memcpy(f, salto, kRoubados);
    VirtualProtect(f, kRoubados, antes, &antes);
    FlushInstructionCache(GetCurrentProcess(), f, kRoubados);

    g_endSceneTramp = reinterpret_cast<EndSceneFn>(ponte);
    return true;
}

// --- achando a funcao: pelo dispositivo que o jogo cria ---------------------

typedef IDirect3D9*(WINAPI* Criar9Fn)(UINT);
typedef HRESULT(WINAPI* CriarDevFn)(IDirect3D9*, UINT, D3DDEVTYPE, HWND, DWORD,
                                    D3DPRESENT_PARAMETERS*, IDirect3DDevice9**);

constexpr int kSlotCreateDevice = 16;   // na tabela do IDirect3D9

Criar9Fn g_criar9Orig = nullptr;
CriarDevFn g_criarDevOrig = nullptr;

bool TrocaSlot(void** tabela, int slot, void* meu, void** guardaOriginal) {
    DWORD antes = 0;
    if (!VirtualProtect(&tabela[slot], sizeof(void*), PAGE_EXECUTE_READWRITE, &antes)) {
        return false;
    }
    *guardaOriginal = tabela[slot];
    tabela[slot] = meu;
    VirtualProtect(&tabela[slot], sizeof(void*), antes, &antes);
    return true;
}

void DesviaDispositivo(IDirect3DDevice9* dev) {
    if (dev == nullptr || g_endSceneTramp != nullptr) {
        return;   // o desvio no codigo ja vale para qualquer dispositivo
    }
    void** tabela = *reinterpret_cast<void***>(dev);
    void* endScene = tabela[kSlotEndScene];
    char mod[64];
    ModuloDe(endScene, mod, sizeof(mod));
    const bool ok = DesviaCodigo(endScene);
    char buf[200];
    sprintf_s(buf, "=== d3d EndScene de %s em %p, desvio no codigo %s", mod, endScene,
              ok ? "ok" : "falhou");
    Log(buf);
}

HRESULT WINAPI MeuCreateDevice(IDirect3D9* self, UINT adaptador, D3DDEVTYPE tipo, HWND janela,
                               DWORD flags, D3DPRESENT_PARAMETERS* pp, IDirect3DDevice9** saida) {
    const HRESULT hr = g_criarDevOrig(self, adaptador, tipo, janela, flags, pp, saida);
    if (SUCCEEDED(hr) && saida != nullptr && *saida != nullptr) {
        DesviaDispositivo(*saida);
    }
    return hr;
}

IDirect3D9* WINAPI MeuDirect3DCreate9(UINT sdk) {
    IDirect3D9* d3d = g_criar9Orig(sdk);
    if (d3d != nullptr && g_criarDevOrig == nullptr) {
        void** tabela = *reinterpret_cast<void***>(d3d);
        if (!TrocaSlot(tabela, kSlotCreateDevice, reinterpret_cast<void*>(&MeuCreateDevice),
                       reinterpret_cast<void**>(&g_criarDevOrig))) {
            Log("=== d3d nao consegui desviar CreateDevice");
        }
    }
    return d3d;
}

// Troca a entrada de Direct3DCreate9 na tabela de importacao do executavel.
bool DesviaImportacao() {
    BYTE* base = reinterpret_cast<BYTE*>(GetModuleHandleA(nullptr));
    const IMAGE_DOS_HEADER* dos = reinterpret_cast<IMAGE_DOS_HEADER*>(base);
    const IMAGE_NT_HEADERS* nt = reinterpret_cast<IMAGE_NT_HEADERS*>(base + dos->e_lfanew);
    const IMAGE_DATA_DIRECTORY& dir = nt->OptionalHeader.DataDirectory[IMAGE_DIRECTORY_ENTRY_IMPORT];
    if (dir.VirtualAddress == 0) {
        return false;
    }
    const IMAGE_IMPORT_DESCRIPTOR* imp =
        reinterpret_cast<IMAGE_IMPORT_DESCRIPTOR*>(base + dir.VirtualAddress);
    for (; imp->Name != 0; ++imp) {
        const char* dll = reinterpret_cast<const char*>(base + imp->Name);
        if (_stricmp(dll, "d3d9.dll") != 0) {
            continue;
        }
        const IMAGE_THUNK_DATA* nomes =
            reinterpret_cast<IMAGE_THUNK_DATA*>(base + (imp->OriginalFirstThunk != 0
                                                            ? imp->OriginalFirstThunk
                                                            : imp->FirstThunk));
        IMAGE_THUNK_DATA* enderecos = reinterpret_cast<IMAGE_THUNK_DATA*>(base + imp->FirstThunk);
        for (; nomes->u1.AddressOfData != 0; ++nomes, ++enderecos) {
            if (IMAGE_SNAP_BY_ORDINAL(nomes->u1.Ordinal)) {
                continue;
            }
            const IMAGE_IMPORT_BY_NAME* n =
                reinterpret_cast<IMAGE_IMPORT_BY_NAME*>(base + nomes->u1.AddressOfData);
            if (strcmp(reinterpret_cast<const char*>(n->Name), "Direct3DCreate9") != 0) {
                continue;
            }
            DWORD antes = 0;
            if (!VirtualProtect(enderecos, sizeof(IMAGE_THUNK_DATA), PAGE_READWRITE, &antes)) {
                return false;
            }
            g_criar9Orig = reinterpret_cast<Criar9Fn>(enderecos->u1.Function);
            enderecos->u1.Function =
                reinterpret_cast<ULONG_PTR>(reinterpret_cast<void*>(&MeuDirect3DCreate9));
            VirtualProtect(enderecos, sizeof(IMAGE_THUNK_DATA), antes, &antes);
            return true;
        }
    }
    return false;
}

struct Installer {
    Installer() {
        Log(DesviaImportacao() ? "=== d3d Direct3DCreate9 desviado na importacao"
                               : "=== d3d nao achei Direct3DCreate9 na importacao");
    }
};

Installer g_instalador;

} // namespace
