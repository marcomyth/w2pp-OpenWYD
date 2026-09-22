-- 0097_chat_log_canais — o registro passa a caber os quatro canais.
--
-- A 0034 criou chat_log com dois tipos, 'publico' e 'sussurro', porque eram os
-- dois únicos caminhos de fala que o servidor tinha. Os canais de guilda, grupo,
-- reino e cidadão existiam no legado mas não estavam portados: quem falava neles
-- recebia "O jogador não está conectado." e ninguém ouvia nada, então não havia
-- o que registrar.
--
-- Agora há (handler/canais.go), e o canal Cidadão alcança o servidor INTEIRO.
-- É exatamente o lugar onde a ofensa e o golpe acontecem em público, que é a
-- pergunta que a 0034 diz existir para responder. Deixar os quatro de fora do
-- registro abriria o buraco justamente no canal mais barulhento do jogo.
--
-- Os tipos não se resumem a 'publico' de propósito: o registro guarda ALCANCE,
-- e o alcance de cada canal é diferente. Ver domain.ChatTipoValido, que repete
-- esta lista em Go — as duas andam juntas.

ALTER TABLE chat_log DROP CONSTRAINT IF EXISTS chat_log_tipo_check;

ALTER TABLE chat_log ADD CONSTRAINT chat_log_tipo_check
    CHECK (tipo IN ('publico', 'sussurro', 'guilda', 'grupo', 'reino', 'cidadao'));
