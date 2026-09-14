// Command kitnovatocliente grava no cliente as duas variantes do kit de novato
// (5760 Frango Assado (Novato) e 5761 Baú de Experiência (Novato)), copiando dos
// itens de origem o nome, a malha e os efeitos (ItemList.bin), o ícone
// (itemicon.bin) e a descrição (itemhelp.dat).
//
// Sem isto o jogador recebe pelo /novato dois itens que o cliente dele não
// conhece: a bolsa mostra um quadrado sem nome, sem ícone e sem descrição. Os
// três arquivos têm de ir juntos — só o ItemList.bin, como na primeira versão,
// ainda deixa o quadrado vazio, porque o ícone não mora nele.
//
// Ele NUNCA escreve dentro da pasta do cliente: lê os arquivos de lá e põe o
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
	cliente := flag.String("cliente", "", "pasta do cliente, com ItemList.bin, itemicon.bin e itemhelp.dat (só é lida)")
	saida := flag.String("saida", "out", "pasta onde os arquivos novos são escritos")
	flag.Parse()

	if err := executar(*cliente, *saida); err != nil {
		fmt.Fprintln(os.Stderr, "erro:", err)
		os.Exit(1)
	}
}

// arquivo é um dos três arquivos do cliente e a transformação que ele recebe.
type arquivo struct {
	nome     string
	aplicar  func([]byte, []clientkit.Variante) ([]byte, error)
	conteudo []byte
}

func executar(cliente, saida string) error {
	if cliente == "" {
		return fmt.Errorf("informe -cliente com a pasta do cliente")
	}
	arquivos := []*arquivo{
		{nome: "ItemList.bin", aplicar: clientkit.Aplicar},
		{nome: "itemicon.bin", aplicar: clientkit.AplicarIcones},
		{nome: "itemhelp.dat", aplicar: clientkit.AplicarDescricoes},
	}
	// Tudo é calculado antes de qualquer gravação: uma saída com o ItemList.bin
	// novo e o itemicon.bin velho é exatamente o kit sem ícone.
	for _, a := range arquivos {
		origem := filepath.Join(cliente, a.nome)
		dados, err := os.ReadFile(origem)
		if err != nil {
			return fmt.Errorf("ler %s: %w", origem, err)
		}
		if a.conteudo, err = a.aplicar(dados, clientkit.KitDoNovato()); err != nil {
			return err
		}
	}
	if err := os.MkdirAll(saida, 0o755); err != nil {
		return fmt.Errorf("criar %s: %w", saida, err)
	}
	for _, a := range arquivos {
		destino := filepath.Join(saida, a.nome)
		if err := os.WriteFile(destino, a.conteudo, 0o644); err != nil {
			return fmt.Errorf("escrever %s: %w", destino, err)
		}
	}
	for _, v := range clientkit.KitDoNovato() {
		fmt.Printf("item %d gravado a partir do %d: %s\n", v.Destino, v.Origem, v.Nome)
	}
	fmt.Println("saída:", saida)
	return nil
}
