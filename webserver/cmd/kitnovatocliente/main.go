// Command kitnovatocliente grava no ItemList.bin do cliente as duas variantes do
// kit de novato (5760 Frango Assado (Novato) e 5761 Baú de Experiência
// (Novato)), copiando nome, malha, textura e efeitos dos itens de origem.
//
// Sem isto o jogador recebe pelo /novato dois itens que o cliente dele não
// conhece: o registro existe no catálogo (é um vetor fixo de 6500) mas está
// zerado, então a bolsa mostra um quadrado sem nome e sem ícone.
//
// Ele NUNCA escreve dentro da pasta do cliente: lê o ItemList.bin de lá e põe o
// resultado na pasta de saída, pronto para o launcher publicar. Trocar o arquivo
// do cliente é decisão de quem está jogando com ele.
//
//	go run ./webserver/cmd/kitnovatocliente \
//	    -cliente "C:\caminho\do\cliente" -saida .\out
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/jeanluca/w2pp-openwyd/webserver/internal/clientkit"
)

func main() {
	cliente := flag.String("cliente", "", "pasta do cliente, com ItemList.bin (só é lida)")
	saida := flag.String("saida", "out", "pasta onde o ItemList.bin novo é escrito")
	flag.Parse()

	if err := executar(*cliente, *saida); err != nil {
		fmt.Fprintln(os.Stderr, "erro:", err)
		os.Exit(1)
	}
}

func executar(cliente, saida string) error {
	if cliente == "" {
		return fmt.Errorf("informe -cliente com a pasta do cliente")
	}
	origem := filepath.Join(cliente, "ItemList.bin")
	il, err := os.ReadFile(origem)
	if err != nil {
		return fmt.Errorf("ler %s: %w", origem, err)
	}
	novo, err := clientkit.Aplicar(il, clientkit.KitDoNovato())
	if err != nil {
		return err
	}
	if err := os.MkdirAll(saida, 0o755); err != nil {
		return fmt.Errorf("criar %s: %w", saida, err)
	}
	destino := filepath.Join(saida, "ItemList.bin")
	if err := os.WriteFile(destino, novo, 0o644); err != nil {
		return fmt.Errorf("escrever %s: %w", destino, err)
	}
	for _, v := range clientkit.KitDoNovato() {
		fmt.Printf("item %d gravado a partir do %d: %s\n", v.Destino, v.Origem, v.Nome)
	}
	fmt.Println("saída:", destino)
	return nil
}
