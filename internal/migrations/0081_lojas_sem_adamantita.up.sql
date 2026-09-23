-- 0081_lojas_sem_adamantita — a Pedra de Adamantita sai das lojas dos doze
-- vendedores de armadura.
--
-- Complemento da 0079: os quatro de Armia e os quatro de Azran vendiam a pedra
-- ao lado do set (a 0079 apenas a desceu para a vaga 15, abaixo dos três sets).
-- Agora a prateleira deles é só armadura.
--
-- As duas metades de novo: os mesmos doze arquivos de Release/TMsrv/run/npc/
-- perdem a pedra no mesmo commit, senão o dbServer a recoloca na vaga livre no
-- boot seguinte (INSERT ... ON CONFLICT DO NOTHING). Com a vaga vazia dos dois
-- lados não é preciso marcá-la em npc_shop_slot_cleared (0072): não há nada no
-- template para semear ali.
--
-- Só estes doze. A pedra continua à venda onde já estava fora deles.
DELETE FROM npc_shop_item
WHERE item_index = 578
  AND npc_id IN (
        SELECT id FROM npc_definition
        WHERE template_name IN (
            'Ferreiro', 'Ferreiro_', 'Ferreiro_Azran',
            'Rapein',   'Rapein_',   'Rapein_Azran',
            'Arnod',    'Arnod_',    'Arnod_Azran',
            'Rainy',    'Rainy_',    'Rainy_Azran'));

UPDATE npc_config_meta SET version = version + 1 WHERE id = TRUE;
