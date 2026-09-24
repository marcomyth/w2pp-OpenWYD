package itemcatalog_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/jeanluca/w2pp-openwyd/internal/buildinfo"
	"github.com/jeanluca/w2pp-openwyd/webserver/internal/itemcatalog"
)

// AS DUAS IMPRESSÕES DO ItemList TÊM DE SER O MESMO NÚMERO.
//
// O webServer calcula a sua enquanto varre o arquivo (TeeReader sobre o Scanner); o
// tmServer usa buildinfo.ImpressaoDoConteudo, que lê o arquivo direto. São dois
// caminhos diferentes de propósito — o webServer não paga uma segunda leitura —, e é
// exatamente por isso que este teste existe: se os dois números divergirem, cada
// serviço passa a dizer uma coisa sobre o MESMO arquivo, e a comparação entre eles,
// que é a única razão de a impressão existir, deixa de valer calada.
func TestAsDuasImpressoesDoItemListSaoAMesma(t *testing.T) {
	conteudo := filepath.Join("..", "..", "..", "Release")
	caminho := filepath.Join(conteudo, "Common", "ItemList.csv")
	if _, err := os.Stat(caminho); err != nil {
		t.Skipf("Release/Common/ItemList.csv ausente (%v); a comparação só vale com o conteúdo presente", err)
	}

	catalogo, err := itemcatalog.Scan(conteudo)
	if err != nil {
		t.Fatalf("varredura do catálogo: %v", err)
	}
	impressao, err := buildinfo.ImpressaoDoConteudo(caminho)
	if err != nil {
		t.Fatalf("impressão do conteúdo: %v", err)
	}
	if catalogo.Version != impressao {
		t.Errorf("webServer diz %q e tmServer diria %q; os dois lados precisam do MESMO numero",
			catalogo.Version, impressao)
	}
	if len(impressao) != buildinfo.TamanhoDaImpressao {
		t.Errorf("a impressao tem %d caracteres, quero %d", len(impressao), buildinfo.TamanhoDaImpressao)
	}
}
