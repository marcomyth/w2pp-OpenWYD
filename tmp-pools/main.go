// Ferramenta de uma vez: devolver a vida e a mana do personagem ao padrao.
//
// O XpFoema1 estava com 50.000.100 de vida e 5.000.000 de mana, valores postos a
// mao para testar bots. O padrao e o que BASE_GetHpMp calcula a partir da
// classe, do nivel e dos atributos BASE - a mesma funcao que o servidor usa, e
// nao um numero escolhido aqui.
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/jackc/pgx/v5"
	"github.com/jeanluca/w2pp-openwyd/internal/level"
)

const nome = "XpFoema1"

func main() {
	host, porta := os.Getenv("PROXY_HOST"), os.Getenv("PROXY_PORT")
	url := fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=require",
		os.Getenv("PGUSER"), os.Getenv("PGPASSWORD"), host, porta, os.Getenv("PGDATABASE"))
	ctx := context.Background()
	c, err := pgx.Connect(ctx, url)
	if err != nil {
		fmt.Println("conexao falhou:", err)
		os.Exit(1)
	}
	defer c.Close(ctx)

	var cls, classMaster int16
	var lvl, con, intel, maxhp, maxmp int32
	err = c.QueryRow(ctx, `
		SELECT class, coalesce(class_master, 0), level, con, int, max_hp, max_mp
		  FROM character WHERE name = $1`, nome).
		Scan(&cls, &classMaster, &lvl, &con, &intel, &maxhp, &maxmp)
	if err != nil {
		fmt.Println("leitura falhou:", err)
		os.Exit(1)
	}
	fmt.Printf("%s  classe %d  master %d  nivel %d  CON %d  INT %d\n",
		nome, cls, classMaster, lvl, con, intel)
	fmt.Printf("  antes:  vida %d   mana %d\n", maxhp, maxmp)

	hp, mp := level.BasePools(uint8(cls), uint8(classMaster), lvl, con, intel)
	fmt.Printf("  padrao: vida %d   mana %d\n", hp, mp)

	if os.Getenv("GRAVAR") != "1" {
		fmt.Println("=> ensaio: nada gravado. Rode com GRAVAR=1.")
		return
	}
	if _, err := c.Exec(ctx, `
		UPDATE character SET max_hp = $2, max_mp = $3, hp = $2, mp = $3
		 WHERE name = $1`, nome, hp, mp); err != nil {
		fmt.Println("escrita falhou:", err)
		os.Exit(1)
	}
	fmt.Println("=> gravado, com a vida e a mana cheias.")
}
