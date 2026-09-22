-- Tira os três sets do degrau C das vagas 0-14 dos doze vendedores de armadura.
--
-- Não recria o estoque anterior: ele vinha do template, e reverter esta migração
-- sem reverter os doze arquivos de Release/TMsrv/run/npc/ deixaria as duas
-- metades em desacordo. Com as vagas livres e os templates revertidos pelo git,
-- o dbServer ressemeia a prateleira antiga no boot seguinte.
DELETE FROM npc_shop_item
WHERE slot BETWEEN 0 AND 14
  AND npc_id IN (
        SELECT id FROM npc_definition
        WHERE template_name IN (
            'Ferreiro', 'Ferreiro_', 'Ferreiro_Azran',
            'Rapein',   'Rapein_',   'Rapein_Azran',
            'Arnod',    'Arnod_',    'Arnod_Azran',
            'Rainy',    'Rainy_',    'Rainy_Azran'));

UPDATE npc_config_meta SET version = version + 1 WHERE id = TRUE;
