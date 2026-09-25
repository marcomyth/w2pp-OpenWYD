-- 0146_emblema_do_dragao_fora — o Emblema do Dragão (751) sai do drop de todo
-- monstro.
--
-- Pedido do Marco em 25/09/2026: "não podemos dropar essa chave". O emblema é a
-- chave do Portão dos 2 Castelos (757, EF_KEYID 10) e caía de dez templates:
-- Anciao_Ciclops_, BolldyCyclops, BolldyCyclops_, CruelCyclops, CruelCyclops_,
-- ElderCyclops, Lanceiro_Zakum, Orc_Medico_, Rei_Taurus_ e Zakum_Picket.
--
-- Mesma forma da 0085: a regra '*' a 0% faz a Mesa de Drops pular a vaga do item
-- em TODO template (droprule.Table.Governs), inclusive num que ganhe o emblema
-- depois; as regras por monstro saem junto, senão continuariam valendo. Os
-- templates ficam como estão, e o painel ainda pode reabrir um monstro.
INSERT INTO drop_rule (mob, item, chance) VALUES ('*', 751, 0)
ON CONFLICT (mob, item) DO UPDATE SET chance = 0, updated_at = now();

DELETE FROM drop_rule WHERE item = 751 AND mob <> '*';

UPDATE drop_rule_meta SET version = version + 1 WHERE id = TRUE;
