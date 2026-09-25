-- Devolve a Mesa ao que era antes da 0149: saem as regras do chefe e da escolta.
-- Os templates Boss_Hidra_Dourada e Guer_Caveira_Escolta e os blocos 2099 e 6161
-- voltam pelo git.
DELETE FROM drop_rule WHERE mob IN ('Boss_Hidra_Dourada', 'Guer_Caveira_Escolta');

UPDATE drop_rule_meta SET version = version + 1 WHERE id = TRUE;
