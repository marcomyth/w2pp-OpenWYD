// Command acessoriocliente gera os arquivos do cliente para a reforma dos
// acessórios: o WYD.exe com as linhas "Dano físico (%)" e "Dano mágico (%)" no
// tooltip, o UI\strdef.bin com os dois rótulos e o ItemList.bin com os
// acessórios de Hércules, Hecate e Zeus e os planetas tirados do catálogo do servidor.
//
//	acessoriocliente -cliente "C:\...\WYD-Cliente-Pronto" \
//	                 -catalogo Release/Common/ItemList.csv
//
// A pasta do cliente é só LIDA: os arquivos vão para -saida (por padrão
// <cliente>\gerado-acessorios), com a mesma disposição. Do ItemList.bin só esses
// registros mudam, para não desfazer o que outros geradores gravaram nele.
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/jeanluca/w2pp-openwyd/webserver/internal/clientacessorio"
	"github.com/jeanluca/w2pp-openwyd/webserver/internal/clientitemlist"
)

func main() {
	cliente := flag.String("cliente", "", "pasta do cliente, com WYD.exe, ItemList.bin e UI\\strdef.bin (só é lida)")
	catalogo := flag.String("catalogo", "", "ItemList.csv do servidor")
	saida := flag.String("saida", "", `pasta onde gravar os arquivos gerados (padrão: <cliente>\gerado-acessorios)`)
	flag.Parse()
	if *cliente == "" || *catalogo == "" {
		flag.Usage()
		os.Exit(2)
	}
	if *saida == "" {
		*saida = filepath.Join(*cliente, "gerado-acessorios")
	}
	if err := run(*cliente, *catalogo, *saida); err != nil {
		log.Fatal(err)
	}
}

func run(cliente, catalogo, saida string) error {
	exe, err := os.ReadFile(filepath.Join(cliente, "WYD.exe"))
	if err != nil {
		return fmt.Errorf("acessoriocliente: ler o WYD.exe: %w", err)
	}
	novoExe, err := clientacessorio.PatchExe(exe)
	if err != nil {
		return err
	}
	sd, err := os.ReadFile(filepath.Join(cliente, "UI", "strdef.bin"))
	if err != nil {
		return fmt.Errorf("acessoriocliente: ler o strdef.bin: %w", err)
	}
	novoSd, err := clientacessorio.PatchStrdef(sd)
	if err != nil {
		return err
	}
	base, err := os.ReadFile(filepath.Join(cliente, "ItemList.bin"))
	if err != nil {
		return fmt.Errorf("acessoriocliente: ler o ItemList.bin: %w", err)
	}
	f, err := os.Open(catalogo)
	if err != nil {
		return fmt.Errorf("acessoriocliente: abrir o catálogo: %w", err)
	}
	linhas, err := clientitemlist.ParseCSV(f)
	_ = f.Close() // lido por inteiro acima; fechar um arquivo só de leitura não muda nada
	if err != nil {
		return err
	}
	inteiro, err := clientitemlist.Build(linhas, base)
	if err != nil {
		return err
	}
	novoIL, err := clientacessorio.CopiarRegistros(base, inteiro, clientacessorio.ItensDaReforma)
	if err != nil {
		return err
	}
	for nome, dados := range map[string][]byte{
		"WYD.exe":                         novoExe,
		filepath.Join("UI", "strdef.bin"): novoSd,
		"ItemList.bin":                    novoIL,
	} {
		destino := filepath.Join(saida, nome)
		if err := os.MkdirAll(filepath.Dir(destino), 0o755); err != nil {
			return fmt.Errorf("acessoriocliente: criar %s: %w", filepath.Dir(destino), err)
		}
		if err := os.WriteFile(destino, dados, 0o644); err != nil {
			return fmt.Errorf("acessoriocliente: gravar %s: %w", destino, err)
		}
		fmt.Println("gerado", destino)
	}
	return nil
}
