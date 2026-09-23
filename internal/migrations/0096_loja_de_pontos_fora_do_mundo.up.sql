-- 0096_loja_de_pontos_fora_do_mundo — a Loja de Pontos sai do mundo (pedido de
-- 21/09/2026), como os 25 comerciantes da praça de Armia saíram na 0083.
--
-- É o NPC Loja_de_Pontos, em Armia (2139,2104), que vendia as onze armas
-- Seladas, a Jóia da Escuridão e a Pedra Lunar. A 0094 já tinha tirado dali o
-- Lactolerium 100.
--
-- Desativa, não apaga — mesma razão da 0083: o slug de um NPC de conteúdo é
-- "<template>-<posição do bloco no NPCGener>", e tirar o bloco do arquivo
-- desloca o índice de todos os seguintes. Reativar é um UPDATE ou um clique no
-- painel. O template de Release/ fica como está: NPC desativado não abre loja, e
-- mexer na vitrine de quem não aparece só faria ruído.
--
-- AS DUAS INSTRUÇÕES, e a segunda não é enfeite. A Loja de Pontos NÃO está no
-- seed da 0006: a linha dela nasce da reconciliação do catálogo de conteúdo, que
-- o dbServer roda no boot (store.SeedNPCDefinitions) DEPOIS de store.Migrate.
-- Numa base nova, portanto, o UPDATE abaixo não encontra linha nenhuma e a seed
-- criaria o NPC logo em seguida com enabled no padrão TRUE — a limpeza sumiria
-- sem aviso. Por isso o INSERT: ele deixa a linha já desativada, e o upsert da
-- seed não toca em `enabled` (seedNPCRows atualiza origin, generator_index e os
-- campos do gerador, e mais nada), então a desativação sobrevive ao boot.
--
-- O UPDATE por template_name continua na frente porque é ele que pega a linha
-- que já existe hoje, qualquer que seja o slug dela: o índice do bloco anda
-- quando o NPCGener.txt é editado, e o 6144 abaixo vale para a base nova, não
-- para a que está no ar.
UPDATE npc_definition SET enabled = FALSE WHERE template_name = 'Loja_de_Pontos';

INSERT INTO npc_definition
    (slug, template_name, display_name, enabled, map_id, pos_x, pos_y, route_type, merchant)
VALUES
    ('Loja_de_Pontos-6144', 'Loja_de_Pontos', 'Loja_de_Pontos', FALSE, 0, 2139, 2104, 2, 1)
ON CONFLICT (slug) DO UPDATE SET enabled = FALSE;

UPDATE npc_config_meta SET version = version + 1 WHERE id = TRUE;
