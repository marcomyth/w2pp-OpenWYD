-- 0027_character_presence — quem está em jogo AGORA, para o painel saber quando
-- pode editar um personagem.
--
-- O tmServer é o dono do personagem vivo: o save dele apaga e regrava a lista
-- inteira de itens (store_live.go SaveCharacter). Editar o banco por baixo de
-- alguém jogando é escrita perdida, e um moderador que acredita ter entregue um
-- item que nunca chegou é como um item duplicado nasce — na segunda tentativa.
--
-- O painel já consegue perguntar ao jogo quem está conectado (GameControlService
-- ListOnline) e derrubar uma sessão (Kick). Isso responde "ele está online?",
-- mas NÃO responde a pergunta que a edição precisa: "o último save dele já
-- caiu?". Derrubar retorna assim que a sessão fecha; a gravação sai depois.
--
-- Esta coluna é essa resposta. O tmServer carimba no login e limpa DEPOIS que o
-- save commita (world.LeaveCharacter), então NULL significa exatamente "o banco
-- é a autoridade sobre este personagem". No boot ele limpa todas: um servidor
-- que acabou de subir não tem ninguém em jogo, e é assim que uma queda deixa de
-- prender personagens em "online" para sempre.
ALTER TABLE character
    ADD COLUMN IF NOT EXISTS online_since TIMESTAMPTZ;

-- Parcial: a pergunta é sempre "quem está em jogo", e na esmagadora maioria das
-- linhas a coluna é NULL.
CREATE INDEX IF NOT EXISTS character_online_since_idx
    ON character (online_since) WHERE online_since IS NOT NULL;
