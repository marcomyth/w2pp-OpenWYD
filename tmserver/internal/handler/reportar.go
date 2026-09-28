package handler

// A DENÚNCIA SAIU DO JOGO, 25/09/2026, por decisão da Hanna: denúncia e suporte passam a
// ser pelo Discord do servidor.
//
// O que existia aqui era o /reportar: ele gravava o que o jogador escreveu MAIS a
// fotografia do servidor naquele instante — posição, nível e quem estava por perto. Era
// bom, e o motivo de sair não é técnico: é que a equipe atende no Discord e uma fila que
// ninguém abre é pior do que não ter fila, porque promete resposta.
//
// O QUE NÃO FOI MEXIDO, de propósito:
//
// A TABELA FICA, com as denúncias que já existem. Elas são histórico, e apagar histórico
// junto com a tela é perder o que ele serve para responder depois.
//
// A LIMPEZA AUTOMÁTICA das antigas FICA também, e o motivo é de privacidade e não de
// espaço: denúncia tem nome de gente e conversa dentro, e guardar para sempre algo que
// ninguém mais vai abrir só aumenta o que vaza num incidente. O prazo continua sendo o
// mesmo de antes; passado ele, a tabela fica vazia e inerte.
//
// E O CAMINHO DE GRAVAÇÃO CONTINUA DE PÉ sem ninguém chamar: RecordReport no dbclient, na
// interface de persistência e no store. Tirá-lo significaria mexer no .proto, o que quebra
// o build do par do site até o handshake — custo alto para apagar código que não roda. Se
// um dia a tabela for embora num PR próprio, ele sai junto.

// msgSuportePeloDiscord é o que quem digita /reportar lê agora.
const msgSuportePeloDiscord = "Denúncias e suporte agora são pelo Discord do servidor."
