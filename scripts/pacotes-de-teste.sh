#!/usr/bin/env bash
# Separa os pacotes de teste em dois grupos: os que precisam do ambiente de
# integracao (PostgreSQL) e os que nao precisam.
#
# POR QUE ISTO NAO E UMA LISTA ESCRITA A MAO. O job de integracao passou a rodar SO
# os pacotes que precisam de banco, porque o job "Build & Test" ja roda todos os
# outros com -race — eram os mesmos pacotes duas vezes no mesmo commit. Uma lista a
# mao erraria do jeito mais silencioso possivel: um pacote novo que usa banco ficaria
# fora dos dois jobs, os testes dele nao rodariam, e nada ficaria vermelho.
#
# COMO DECIDE, por DOIS criterios independentes, e a uniao deles:
#
#   1. O COMPILADOR. Um pacote que GANHA arquivos de teste quando a etiqueta
#      `integration` esta ligada tem teste que so existe nesse ambiente. Quem responde
#      isso e o proprio `go list`, com e sem a etiqueta — nao e busca por texto, e a
#      resposta e a mesma que o `go test` vai usar.
#
#   2. A VARIAVEL DO BANCO. Um pacote cujos arquivos de teste citam W2PP_TEST_DSN fala
#      com o Postgres. E a unica porta de entrada do banco nos testes: todos os
#      helpers a leem com os.Getenv, nao ha outro nome de variavel e nao ha DSN
#      escrito no codigo.
#
# Os dois cobrem buracos diferentes. O criterio 1 sozinho perderia um pacote que usa
# banco sem etiqueta nenhuma (ele pularia calado nos dois jobs). O criterio 2 sozinho
# perderia um teste de integracao que nao toca o banco (ele nao rodaria em job nenhum).
#
# O ERRO SEGURO E PARA QUAL LADO: um pacote que so MENCIONA a variavel num comentario
# cai no grupo com banco. Ele roda certo, so nao ganha o paralelismo do outro job. O
# erro caro e o contrario, e o modo `conferir` existe para ele nunca passar calado.
#
# Uso: pacotes-de-teste.sh {com-banco|sem-banco|conferir}
set -euo pipefail

qual="${1:-}"
case "$qual" in
com-banco | sem-banco | conferir) ;;
*)
    echo "uso: $0 {com-banco|sem-banco|conferir}" >&2
    exit 2
    ;;
esac

# assinatura imprime, por pacote, o caminho e a lista de arquivos de teste que a
# etiqueta pedida faz o compilador enxergar.
assinatura() {
    go list "$@" -f '{{.ImportPath}}{{"\t"}}{{join .TestGoFiles ","}},{{join .XTestGoFiles ","}}' ./...
}

# criterio 1: quem ganha arquivo de teste com a etiqueta ligada.
por_etiqueta() {
    join -t$'\t' -j 1 -o 0,1.2,2.2 \
        <(assinatura | sort -t$'\t' -k1,1) \
        <(assinatura -tags=integration | sort -t$'\t' -k1,1) |
        awk -F'\t' '$2 != $3 {print $1}'
}

# criterio 2: quem cita a variavel do banco em algum arquivo de teste.
por_variavel() {
    go list -tags=integration -f \
        '{{.ImportPath}}{{"\t"}}{{.Dir}}{{"\t"}}{{join .TestGoFiles " "}} {{join .XTestGoFiles " "}}' ./... |
        while IFS=$'\t' read -r caminho dir arquivos; do
            [ -z "${arquivos// /}" ] && continue
            for arquivo in $arquivos; do
                if grep -q 'W2PP_TEST_DSN' "$dir/$arquivo" 2>/dev/null; then
                    echo "$caminho"
                    break
                fi
            done
        done
}

com_banco() {
    { por_etiqueta; por_variavel; } | sort -u
}

case "$qual" in
com-banco)
    com_banco
    ;;
sem-banco)
    # Pacote sem teste nenhum fica de fora dos dois grupos: `go test` nele so
    # imprimiria "no test files", e somar dezenas disso a uma linha de comando
    # atrapalha quem for depurar a CI.
    comem=$(com_banco)
    go list -f '{{.ImportPath}}{{"\t"}}{{join .TestGoFiles ","}},{{join .XTestGoFiles ","}}' ./... |
        awk -F'\t' '$2 != "," {print $1}' |
        grep -vxF -f <(printf '%s\n' "$comem") || true
    ;;
conferir)
    # A CONFERENCIA QUE FECHA O BURACO: um pacote com teste de integracao que ficasse
    # fora da lista nao seria rodado por ninguem — nem pelo Build & Test, que roda sem
    # a etiqueta, nem pelo job de integracao, que so chama os escolhidos. O teste
    # sumiria e nada ficaria vermelho.
    escolhidos=$(com_banco)
    if [ -z "$escolhidos" ]; then
        echo "::error::a lista de pacotes com banco saiu VAZIA; a suite de integracao nao rodaria nada" >&2
        exit 1
    fi
    faltando=0
    while IFS= read -r caminho; do
        if ! printf '%s\n' "$escolhidos" | grep -qxF "$caminho"; then
            echo "::error::$caminho tem teste que so existe com a etiqueta integration, mas ficou fora do job; ele nao seria rodado por ninguem" >&2
            faltando=1
        fi
    done < <(por_etiqueta)
    [ "$faltando" -eq 0 ] || exit 1

    quantos=$(printf '%s\n' "$escolhidos" | grep -c . || true)
    echo "conferencia ok: $quantos pacote(s) no job de integracao, e nenhum teste de integracao ficou fora"
    ;;
esac
