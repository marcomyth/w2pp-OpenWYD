-- 0100_aki_pocoes_e_batedor — a Aki e o Martin passam a vender as poções de 500,
-- e as duas do Batedor voltam à prateleira (pedido de 22/09/2026).
--
-- DUAS COISAS DIFERENTES AQUI, e a segunda é uma correção.
--
-- 1) AS POÇÕES DE 500. Nenhuma das 2014 lojas do jogo vendia poção nenhuma até
--    agora — as poções só vinham de drop e das caixas, e as caixas saíram na
--    0094. A Ultra Poção de Cura (404) e a Ultra Poção de Mana (409) são as de
--    500, o teto do que existe (EF_HP/EF_MP 500 no ItemList).
--
--    UMA POR COMPRA, sem EF_AMOUNT, e isto é deliberado: o servidor cobra o
--    preço do catálogo por COMPRA, não por unidade (handler/shop.go:120). Uma
--    pilha de dez sairia pelo preço de uma — 200 de ouro a poção em vez de
--    2.000. É o que acontece hoje com o Pergaminho Retorno destas mesmas lojas,
--    que sai em dez por 1.500 enquanto o item "10x" do catálogo custa 15.000.
--    Não corrijo o pergaminho aqui: é outra decisão, e é do Marco.
--
-- 2) A CURA E A MANA DO BATEDOR VOLTAM. A 0006 já semeava as duas nas vagas 12 e
--    13 da Aki, e o template também as traz — mas em jogo as duas vagas estão
--    vazias (foto de 22/09). Alguma coisa as tirou do banco depois da seed, e a
--    marca de "vaga esvaziada no painel" (0072) faz o boot seguinte PULAR a vaga
--    em vez de recolocar o item. Por isso a marca sai antes do INSERT.
--
--    O INSERT é idempotente: se as linhas ainda estiverem lá, nada muda.
--
-- Endereçado por template_name, e não por slug, porque a Aki tem dois blocos no
-- NPCGener e o Martin também — os dois devem ficar iguais, e o slug carrega a
-- posição do bloco, que anda quando o arquivo é editado.
--
-- As duas metades de sempre: os templates Release/TMsrv/run/npc/{Aki,Martin}
-- mudam no mesmo commit, senão o dbServer ressemeia a vitrine velha no boot.

DELETE FROM npc_shop_slot_cleared
WHERE slot IN (0, 1, 4, 12, 13)
  AND npc_id IN (SELECT id FROM npc_definition WHERE template_name IN ('Aki', 'Martin'));

-- Aki: as duas poções fecham a primeira linha da janela, ao lado dos
-- pergaminhos; as duas do Batedor voltam para onde a 0006 as pôs.
INSERT INTO npc_shop_item (npc_id, slot, item_index, quantity)
SELECT d.id, v.slot, v.item, v.qtd
FROM npc_definition d
CROSS JOIN (VALUES
    (0, 404, 1),    -- Ultra Poção de Cura (500 de HP)
    (1, 409, 1),    -- Ultra Poção de Mana (500 de MP)
    (12, 4040, 1),  -- Cura do Batedor
    (13, 4041, 1)   -- Mana do Batedor
) AS v(slot, item, qtd)
WHERE d.template_name = 'Aki'
ON CONFLICT (npc_id, slot) DO UPDATE
SET item_index = EXCLUDED.item_index, quantity = EXCLUDED.quantity,
    eff1 = 0, effv1 = 0, eff2 = 0, effv2 = 0, eff3 = 0, effv3 = 0;

-- Martin: a vaga 0 é das Ervas de Cura, então as poções entram na 1 e na 4 e
-- fecham a mesma primeira linha. O Martin é a cópia da Aki (0095) e segue igual.
INSERT INTO npc_shop_item (npc_id, slot, item_index, quantity)
SELECT d.id, v.slot, v.item, v.qtd
FROM npc_definition d
CROSS JOIN (VALUES
    (1, 404, 1),    -- Ultra Poção de Cura (500 de HP)
    (4, 409, 1),    -- Ultra Poção de Mana (500 de MP)
    (12, 4040, 1),  -- Cura do Batedor
    (13, 4041, 1)   -- Mana do Batedor
) AS v(slot, item, qtd)
WHERE d.template_name = 'Martin'
ON CONFLICT (npc_id, slot) DO UPDATE
SET item_index = EXCLUDED.item_index, quantity = EXCLUDED.quantity,
    eff1 = 0, effv1 = 0, eff2 = 0, effv2 = 0, eff3 = 0, effv3 = 0;

UPDATE npc_config_meta SET version = version + 1 WHERE id = TRUE;
