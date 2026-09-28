#!/usr/bin/env bash
# Recusa gerar código de proto com uma versão de ferramenta diferente da do repo.
#
# POR QUE ISTO EXISTE, e não é preciosismo: o cabeçalho de cada .pb.go carrega a
# versão que o gerou. Rodar `make proto` com outra versão produz diff em arquivo que
# NINGUÉM TOCOU — e isso aconteceu de verdade aqui. O bin.pb.go ficou por meses com
# protoc v7.36.0 enquanto os outros três estavam em v5.29.3, então quem mexesse num
# .proto qualquer levava o selo do bin junto, no meio de um PR de outro assunto.
#
# E RECUSA ANTES DE GERAR, em vez de avisar depois. Um aviso depois chega quando o
# diff já está no disco, e quem estava com pressa já commitou sem olhar.
set -euo pipefail

quero_protoc="${PROTOC_VERSION:-29.3}"
quero_gen_go="${PROTOC_GEN_GO_VERSION:-v1.36.11}"
quero_grpc="${PROTOC_GRPC_VERSION:-1.6.2}"

erro=0

confere() {
	local ferramenta="$1" quero="$2" tenho
	if ! command -v "$ferramenta" >/dev/null 2>&1; then
		echo "proto: $ferramenta não está no PATH (preciso da $quero)" >&2
		erro=1
		return
	fi
	# O segundo campo do --version, que é onde as três põem o número:
	#   libprotoc 29.3 / protoc-gen-go v1.36.11 / protoc-gen-go-grpc 1.6.2
	tenho="$("$ferramenta" --version | awk '{print $2}')"
	if [ "$tenho" != "$quero" ]; then
		echo "proto: $ferramenta é $tenho e eu preciso da $quero." >&2
		echo "       Gerar com essa versão muda o selo de arquivo que você não tocou." >&2
		erro=1
	fi
}

confere protoc "$quero_protoc"
confere protoc-gen-go "$quero_gen_go"
confere protoc-gen-go-grpc "$quero_grpc"

if [ "$erro" != 0 ]; then
	cat >&2 <<'FIM'

Como instalar exatamente estas:
  protoc             https://github.com/protocolbuffers/protobuf/releases/tag/v29.3
                     (binário solto num .zip; não precisa instalador nem PATH do sistema)
  protoc-gen-go      go install google.golang.org/protobuf/cmd/protoc-gen-go@v1.36.11
  protoc-gen-go-grpc go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@v1.6.2

O `protoc --version` diz "libprotoc 29.3" e o selo do .pb.go diz "protoc v5.29.3".
São a mesma coisa: o protoc 29.3 sela com o major da libprotobuf.

Para mudar de versão de propósito, é decisão do repo e não da máquina de quem gera:
mude os números no topo deste script, regenere os QUATRO .proto no mesmo commit, e
diga no commit por que a versão subiu.
FIM
	exit 1
fi
