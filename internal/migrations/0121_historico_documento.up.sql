-- 0121_historico_documento — a troca do CPF entra no rastro, mascarada.
--
-- A 0106 criou o rastro da chave com uma razão que vale igual para o documento: numa
-- disputa, a pergunta é "o que aconteceu com o destino do dinheiro DESTA conta". Uma
-- troca de CPF é a mesma espécie de evento que uma troca de chave — alguém mudou para
-- onde o dinheiro vai —, e sem registro ela é invisível.
--
-- MASCARADO, como a chave, e pelo mesmo motivo que a 0106 escreveu: o rastro é o lugar
-- que ninguém lembra de proteger porque "é só histórico". O que se precisa saber numa
-- disputa é QUE mudou e QUANDO, e para isso o final basta.
--
-- Nulos no primeiro cadastro, que não tem "de onde", e nulos também em toda linha
-- escrita antes desta migração — onde nulo quer dizer "não registrado", e não "não
-- havia documento".
ALTER TABLE rmt_recebedor_historico
    ADD COLUMN IF NOT EXISTS documento_antigo_mascarado TEXT,
    ADD COLUMN IF NOT EXISTS documento_novo_mascarado   TEXT;
