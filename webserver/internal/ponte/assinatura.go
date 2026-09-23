// Package ponte é a borda do webServer com a ponte de repasse — o serviço fora
// da Railway que fala com a processadora de pagamento.
//
// DUAS COISAS ATRAVESSAM ESTE PACOTE E NENHUMA DELAS PODE ESCAPAR DELE:
//
//  1. A prova de quem falou é a ASSINATURA, nunca o cabeçalho de rede. A ponte
//     vai ser publicada por túnel, e a borda do túnel põe cabeçalhos que parecem
//     identidade — não são. Se o túnel cair, ou alguém descobrir o endereço e
//     bater direto na porta, é a assinatura que segura, e só ela.
//
//  2. O segredo mora aqui e em nenhum outro serviço. Foi a razão de o webServer
//     ser escolhido como dono dos dois sentidos da conversa com a ponte: dois
//     serviços falando com ela seriam dois lugares guardando o segredo e dois
//     lugares para conciliar dinheiro.
package ponte

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
)

// Erros que a conferência devolve. São respostas previstas e não falhas: quem
// chama traduz em recusa, e nenhum deles deve dizer ao chamador QUAL parte
// falhou — para quem está tentando adivinhar, "assinatura inválida" e "assinatura
// mal formada" são duas dicas diferentes.
var (
	// ErrAssinaturaInvalida é o que sai para qualquer recusa. É um erro só de
	// propósito.
	ErrAssinaturaInvalida = errors.New("ponte: assinatura invalida")
	// ErrSemSegredo é a recusa de quem não foi configurado. Vive separado porque
	// não é tentativa de fraude, é implantação incompleta — e os dois precisam
	// aparecer diferentes no log de quem opera.
	ErrSemSegredo = errors.New("ponte: segredo da ponte nao configurado")
)

// Confere diz se `assinatura` é a assinatura de `mensagem` feita com `segredo`.
//
// HMAC-SHA256, e a assinatura chega em hexadecimal.
//
// A COMPARAÇÃO É EM TEMPO CONSTANTE, e isso não é preciosismo. Comparar com `==`
// para no primeiro byte diferente, e o tempo que a resposta demora conta quantos
// bytes iniciais estavam certos. Quem pode tentar muitas vezes descobre a
// assinatura byte a byte, sem nunca saber o segredo. `hmac.Equal` existe para
// isso e é a única forma usada aqui.
//
// SEGREDO VAZIO RECUSA, em vez de conferir contra a chave vazia. Sem isto, um
// serviço que subiu sem a variável de ambiente aceitaria qualquer requisição que
// soubesse assinar com a chave vazia — que é uma chave conhecida por todo mundo.
// Falhar alto na implantação incompleta é melhor do que abrir a porta em
// silêncio.
func Confere(segredo []byte, mensagem []byte, assinatura string) error {
	if len(segredo) == 0 {
		return ErrSemSegredo
	}
	recebida, err := hex.DecodeString(assinatura)
	if err != nil {
		return ErrAssinaturaInvalida
	}
	mac := hmac.New(sha256.New, segredo)
	mac.Write(mensagem)
	if !hmac.Equal(recebida, mac.Sum(nil)) {
		return ErrAssinaturaInvalida
	}
	return nil
}

// Assina devolve a assinatura de `mensagem`, em hexadecimal.
//
// Ela existe para o lado de SAÍDA — os pedidos que nós mandamos à ponte também
// vão assinados — e para os testes poderem montar uma requisição legítima sem
// repetir o algoritmo, que é o jeito de o teste deixar de provar o que devia.
func Assina(segredo []byte, mensagem []byte) string {
	mac := hmac.New(sha256.New, segredo)
	mac.Write(mensagem)
	return hex.EncodeToString(mac.Sum(nil))
}
