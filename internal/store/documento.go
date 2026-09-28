package store

import (
	"errors"
	"strings"
)

// ErrDocumentoInvalido é o CPF que não passa na conferência.
var ErrDocumentoInvalido = errors.New("store: documento invalido")

// soNumeros tira ponto, traço e espaço, que é como as pessoas digitam CPF.
//
// Aceitar as duas formas e guardar UMA: "111.444.777-35" e "11144477735" são a mesma
// pessoa, e recusar a primeira faria o formulário brigar com quem copiou do próprio
// documento.
func soNumeros(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// NormalizaDocumento confere um CPF e devolve só os onze dígitos.
//
// AQUI O DÍGITO VERIFICADOR É CONFERIDO, e na chave Pix do tipo CPF ele NÃO é. A
// diferença é deliberada e vale explicar, porque à primeira vista é incoerente.
//
// O comentário da chave diz que uma implementação errada da regra recusaria CPF
// válido, o que é pior do que deixar passar um inválido — já que a processadora
// recusa de qualquer jeito. Esse raciocínio vale para a CHAVE e não vale para o
// DOCUMENTO, por causa de QUANDO cada erro aparece:
//
//   - Chave errada: a transferência falha na hora do repasse, com mensagem da
//     processadora dizendo que a chave não existe. Barulhento e rastreável.
//   - Documento errado mas bem formado: ele é aceito no cadastro, fica guardado, e
//     só quebra no dia do repasse — como "o documento não confere com a chave", que
//     é um erro que não aponta para o campo errado. O dinheiro do vendedor fica
//     parado enquanto alguém descobre que faltava um dígito.
//
// E o medo de "recusar CPF válido" é mensurável: o algoritmo é fechado, tem teste de
// tabela abaixo, e um número que falha nele está errado com certeza — não há falso
// positivo possível. O que ele pega são os erros comuns de digitação, que são
// justamente os que produzem número bem formado e pessoa inexistente.
func NormalizaDocumento(entrada string) (string, error) {
	doc := soNumeros(entrada)
	if len(doc) != 11 {
		return "", ErrDocumentoInvalido
	}

	// ONZE DÍGITOS IGUAIS PASSAM NA CONTA e não são CPF de ninguém: 111.111.111-11
	// fecha os dois dígitos verificadores certinho. É a armadilha clássica de quem
	// implementa só o algoritmo, e é também o que alguém digita para "pular" o campo.
	if todosIguais(doc) {
		return "", ErrDocumentoInvalido
	}

	if doc[9] != digitoVerificador(doc[:9], 10) || doc[10] != digitoVerificador(doc[:10], 11) {
		return "", ErrDocumentoInvalido
	}
	return doc, nil
}

func todosIguais(s string) bool {
	for i := 1; i < len(s); i++ {
		if s[i] != s[0] {
			return false
		}
	}
	return true
}

// digitoVerificador calcula um dígito do CPF: soma cada algarismo multiplicado por um
// peso que decresce, e o resto por 11 decide. Resto 0 ou 1 dá dígito zero.
func digitoVerificador(base string, pesoInicial int) byte {
	soma := 0
	for i, c := range base {
		soma += int(c-'0') * (pesoInicial - i)
	}
	resto := soma * 10 % 11
	if resto == 10 {
		resto = 0
	}
	return byte('0' + resto)
}

// MascaraDocumento devolve o bastante para a pessoa reconhecer o próprio CPF, e nada
// além disso.
//
// MOSTRA O FIM E NÃO O MEIO, e isso foi pedido pelo par do site com uma razão que eu
// não teria achado sozinha: a tela escreve "CPF terminado em NN" pegando os dois
// últimos dígitos do que vier. Uma máscara que deixasse o meio aparecer —
// "***.456.789-**" — faria a tela mostrar dígitos do MEIO como se fossem o final, e a
// pessoa conferiria contra o número errado sem ter como perceber.
//
// É o mesmo padrão da chave: asteriscos e o final.
func MascaraDocumento(doc string) string {
	if len(doc) != 11 {
		// Inclui o vazio, que é o caso de todo vendedor cadastrado antes de a coluna
		// existir. Vazio é honesto: a tela mostra "não cadastrado" em vez de um
		// desenho de máscara sobre coisa nenhuma.
		return ""
	}
	return "***.***.***-" + doc[9:]
}
