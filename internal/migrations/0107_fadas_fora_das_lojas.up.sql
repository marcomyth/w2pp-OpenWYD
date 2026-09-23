-- 0107_fadas_fora_das_lojas — nenhum comerciante vende fada.
--
-- Decisão do Marco, 23/09/2026: fada não se compra mais de NPC. Vale para toda
-- fada que vai no slot 13 — Verde, Azul, Vermelha (3900-3908), Verde e Suprema
-- (3911-3913), Prateada, Dourada e do Vale (3914-3916; a do Vale a 0091 já
-- tinha tirado). Os dois Mapa_Vale_Escondido (3909, 3910) não são fada e ficam.
-- Poeira de Fada, Água das Fadas e Som das Fadas também não são, e ficam.
--
-- O DELETE é por item, como na 0091: pega a vaga do template e a que o painel
-- tenha posto, sem depender do slug do NPC, que anda quando o NPCGener muda.
--
-- As duas metades, como sempre: os templates Fadas, Nordic_Store__, Utilidades
-- e XTS_Store perdem as 20 vagas no mesmo commit, senão o dbServer as recoloca
-- no boot seguinte (store.seedNPCShopRows).
DELETE FROM npc_shop_item
WHERE item_index BETWEEN 3900 AND 3908
   OR item_index BETWEEN 3911 AND 3916;

UPDATE npc_config_meta SET version = version + 1 WHERE id = TRUE;
