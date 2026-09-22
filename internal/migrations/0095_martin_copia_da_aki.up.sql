-- 0095_martin_copia_da_aki — o Martin ganha a vitrine da Aki, mais as Ervas de
-- Cura (pedido de 21/09/2026).
--
-- A 0094 tirou os dez itens que o Martin vendia e o deixou de prateleira vazia.
-- A vitrine nova é a mesma da Aki, vaga por vaga: os dois pergaminhos em dez, os
-- quatro consumíveis do Batedor/Coveiro/Jardineiro e os cinco anéis. E as Ervas
-- de Cura (415) entram na vaga 0, que a 0094 deixou livre nas duas lojas — é a
-- erva que o jogador usa contra a lentidão, e foi pedida de volta.
--
-- ATENÇÃO, e está fora do que esta migração resolve: a 415 carrega EF_VOLATILE
-- 243, o volátil da Jóia da Recuperação/Armazenagem, e o handler só limpa afetos
-- quando o índice é 3203 (handler/item.go:1467). Hoje usar a erva CONSOME e não
-- faz nada — no legado também não fazia. Pôr a erva na loja é o que foi pedido;
-- fazer a erva limpar a lentidão é mudança de regra, e fica para uma decisão.
--
-- Os valores são literais, não um SELECT da vitrine da Aki: a partir daqui são
-- duas lojas independentes, e um espelho vivo faria a próxima mexida na Aki
-- respingar no Martin sem ninguém pedir. Endereçado por template_name porque o
-- Martin tem DOIS blocos no NPCGener (Armia 2116,2150 e 1317,346) e os dois
-- devem ficar iguais; o slug carrega a posição do bloco, que anda quando o
-- arquivo é editado.
--
-- As duas metades de sempre: o template Release/TMsrv/run/npc/Martin muda no
-- mesmo commit, senão o dbServer ressemeia a vitrine velha no boot seguinte.
DELETE FROM npc_shop_item
WHERE npc_id IN (SELECT id FROM npc_definition WHERE template_name = 'Martin');

-- A marca de vaga esvaziada pelo painel (0072) sai junto, senão a seed do boot
-- pula a vaga se a linha se perder um dia.
DELETE FROM npc_shop_slot_cleared
WHERE npc_id IN (SELECT id FROM npc_definition WHERE template_name = 'Martin');

INSERT INTO npc_shop_item (npc_id, slot, item_index, quantity)
SELECT d.id, v.slot, v.item, v.qtd
FROM npc_definition d
CROSS JOIN (VALUES
    (0, 415, 10),   -- Ervas de Cura
    (2, 699, 10),   -- Pergaminho do Teleporte
    (3, 410, 10),   -- Pergaminho Retorno
    (10, 4038, 1),  -- Vela do Coveiro
    (11, 4039, 1),  -- Colheita do Jardineiro
    (12, 4040, 1),  -- Cura do Batedor
    (13, 4041, 1),  -- Mana do Batedor
    (18, 501, 1),   -- Anel de Hercules
    (19, 503, 1),   -- Anel de Titã
    (20, 502, 1),   -- Anel de Athena
    (21, 506, 1),   -- Anel de Hecate
    (22, 505, 1)    -- Anel de Zeus
) AS v(slot, item, qtd)
WHERE d.template_name = 'Martin'
ON CONFLICT (npc_id, slot) DO UPDATE
SET item_index = EXCLUDED.item_index, quantity = EXCLUDED.quantity,
    eff1 = 0, effv1 = 0, eff2 = 0, effv2 = 0, eff3 = 0, effv3 = 0;

UPDATE npc_config_meta SET version = version + 1 WHERE id = TRUE;
