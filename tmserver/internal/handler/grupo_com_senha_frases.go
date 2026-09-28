package handler

// AS FRASES DO GRUPO COM SENHA, num arquivo só.
//
// JUNTAS PORQUE A REGRA É SOBRE O CONJUNTO: cada recusa tem de dizer o MOTIVO, e o
// jeito de garantir isso é ler todas de uma vez. Espalhadas pelo handler, a décima
// recusa acaba sendo um "não foi possível" que não ajuda ninguém.
//
// CADA UMA CABE EM 94 BYTES em cp1252 (protocol.messagePanelTextMax), e isso está
// medido em teste e não conferido no olho — o painel do cliente corta o resto sem
// avisar, e uma frase cortada no meio é pior do que uma frase curta.
//
// POR QUE NENHUMA DIZ "SENHA": nem a errada nem a certa repetem o que a pessoa
// digitou. O painel do cliente fica na tela por alguns segundos e é visível para quem
// estiver olhando a máquina dela.
const (
	msgGrupoComoUsarCriar      = "Use: /criargrupo senha    (4 a 12 letras ou números)"
	msgGrupoComoUsarEntrar     = "Use: /entrar NomeDoPersonagem senha"
	msgGrupoComoUsarTransLider = "Use: /translider NomeDoPersonagem"

	msgGrupoSenhaInvalida = "A senha do grupo tem de 4 a 12 caracteres, só letras e números."
	msgGrupoCriado        = "Grupo criado com senha. Você é o líder."
	msgGrupoSenhaTrocada  = "A senha do seu grupo foi trocada."

	msgGrupoEntrou       = "Você entrou no grupo."
	msgGrupoAlguemEntrou = "Alguém entrou no seu grupo pela senha."

	msgGrupoSenhaErrada       = "Senha do grupo errada."
	msgGrupoCheio             = "O grupo está cheio."
	msgGrupoVoceJaEstaEmGrupo = "Você já está num grupo. Saia dele antes de entrar em outro."
	msgGrupoMuitasTentativas  = "Muitas tentativas. Espere 1 minuto."
	msgGrupoNomeNaoEncontrado = "Não achei esse personagem online."
	msgGrupoNaoTemSenha       = "O grupo desse personagem não tem senha."
	msgGrupoVoceMesmo         = "Esse é você mesmo."
	msgGrupoLimiteDeNivel     = "A diferença de nível não permite entrar nesse grupo."

	msgGrupoVoceNaoEOLider   = "Você não é o líder do grupo."
	msgGrupoSemMembros       = "Seu grupo não tem outros membros."
	msgGrupoNaoEMembro       = "Esse personagem não está no seu grupo."
	msgGrupoLiderancaPassada = "Você passou a liderança do grupo."
	msgGrupoVoceEOLiderAgora = "Você agora é o líder do grupo, e a senha continua a mesma."
)

// frasesDoGrupoComSenha é a lista que o teste de tamanho percorre.
//
// EXISTE PARA O TESTE NÃO DEPENDER DE ALGUÉM LEMBRAR DE ADICIONAR. Uma frase nova que
// não entre aqui não é medida, então o teste também confere que a lista tem o mesmo
// tamanho do bloco de constantes — ver grupo_com_senha_test.go.
var frasesDoGrupoComSenha = []string{
	msgGrupoComoUsarCriar,
	msgGrupoComoUsarEntrar,
	msgGrupoComoUsarTransLider,
	msgGrupoSenhaInvalida,
	msgGrupoCriado,
	msgGrupoSenhaTrocada,
	msgGrupoEntrou,
	msgGrupoAlguemEntrou,
	msgGrupoSenhaErrada,
	msgGrupoCheio,
	msgGrupoVoceJaEstaEmGrupo,
	msgGrupoMuitasTentativas,
	msgGrupoNomeNaoEncontrado,
	msgGrupoNaoTemSenha,
	msgGrupoVoceMesmo,
	msgGrupoLimiteDeNivel,
	msgGrupoVoceNaoEOLider,
	msgGrupoSemMembros,
	msgGrupoNaoEMembro,
	msgGrupoLiderancaPassada,
	msgGrupoVoceEOLiderAgora,
}
