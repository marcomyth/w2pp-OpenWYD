-- 0085_bolsa_da_sorte_fora — a Bolsa da Sorte (4104) sai do drop de todo
-- monstro e de toda loja (limpeza para o lançamento).
--
-- A regra com mob '*' e chance 0 é a forma que a Mesa de Drops tem de tirar um
-- item de TODO monstro de uma vez (0070 fez o mesmo com o Amuleto Arcano); as
-- regras por monstro daquele item saem junto, senão continuariam valendo para os
-- que as tivessem. Além disso os templates que a carregavam perderam a vaga no
-- mesmo commit: cinco monstros (Aranha Venenosa, Cria de Aranha, Demônio do
-- Vale, Orc Cavaleiro e o Porco) e três lojas (Evento, Acessorios3 e Águia).
--
-- Porco e Águia tinham a bolsa em TODAS as vagas, então ficam sem nada — é o
-- que "retirar de todos os mobs" pede, e o que devolver no lugar é decisão de
-- balanceamento, não desta limpeza.
INSERT INTO drop_rule (mob, item, chance) VALUES ('*', 4104, 0)
ON CONFLICT (mob, item) DO UPDATE SET chance = 0, updated_at = now();

DELETE FROM drop_rule WHERE item = 4104 AND mob <> '*';

UPDATE drop_rule_meta SET version = version + 1 WHERE id = TRUE;

DELETE FROM npc_shop_item WHERE item_index = 4104;

UPDATE donate_shop_item SET enabled = FALSE, updated_at = now()
WHERE item_index = 4104 AND enabled;

UPDATE daily_reward_item SET enabled = FALSE, updated_at = now()
WHERE item_index = 4104 AND enabled;

UPDATE npc_config_meta SET version = version + 1 WHERE id = TRUE;
