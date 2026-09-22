-- 0086_lojas_de_cidade_limpas — retirada de itens das lojas de NPC, pergaminho
-- em dez e ração em pacote de sessenta (limpeza para o lançamento).
--
-- Sai de TODA loja de NPC:
--   4111-4113  Entrada do Território (N/M/A)
--   668-671    os livros Sephira (Muro de Espinhos, Ressurreição, Concentração,
--              Força Espectral)
--   4043       Emblema da Proteção
--   3140       Pedra da Luz
--   1774       Pedra do Sábio
--   3338       Refinação Abençoada
--   3200-3209  as dez Jóias
--   2441/2442/2444, 3386/3387/3389, 697, 4131  as gemas, a Safira e o pacote
--
-- Quantidade: todo pergaminho passa a sair de dez em dez (vinha de 255 no Aki), e
-- toda ração de montaria em pacote de sessenta (vinha de 255, 120, 100, 20, 5 e
-- 1, cada loja com a sua).
--
-- A Refinação Abençoada sai também do drop: a regra com mob '*' e chance 0 é a
-- forma que a Mesa de Drops tem de tirar um item de todo monstro (0085 fez o
-- mesmo com a Bolsa da Sorte). Hoje ela cai do Rei Carbuncle e do Rei Lich
-- Batama; com a regra, de nenhum.
--
-- As duas metades de sempre: as 40 lojas dos templates de Release/TMsrv/run/npc/
-- mudam no mesmo commit, senão o dbServer ressemeia o que foi tirado no boot
-- seguinte. Nos templates a mudança é só nas 27 vagas da VITRINE de quem é
-- mercador (Merchant 1 ou 19) — o Carry de monstro, que é tabela de drop, fica
-- intacto, e por isso o drop da Refinação Abençoada precisa da regra acima.
DELETE FROM npc_shop_item WHERE item_index IN (
    4111, 4112, 4113,
    668, 669, 670, 671,
    4043, 3140, 1774, 3338,
    3200, 3201, 3202, 3203, 3204, 3205, 3206, 3207, 3208, 3209,
    2441, 2442, 2444, 3386, 3387, 3389, 697, 4131);

UPDATE npc_shop_item SET quantity = 10
WHERE quantity > 10 AND item_index IN (
    410, 411, 446, 699, 776, 777, 778, 779, 780, 781, 782, 783, 784,
    1777, 1778, 1779,
    3173, 3174, 3175, 3176, 3177, 3178, 3179, 3180,
    3182, 3183, 3184, 3185, 3186, 3187, 3188, 3189,
    3220, 3221, 3335, 3343, 3429, 3430, 3463, 3473, 3474, 4127, 4149, 5700);

UPDATE npc_shop_item SET quantity = 60
WHERE quantity <> 60 AND item_index IN (
    2420, 2421, 2422, 2423, 2424, 2425, 2426, 2427, 2428, 2429, 2430, 2431, 2432,
    2436, 2437, 2438, 2439,
    3368, 3369, 3370, 3371, 3372, 3373, 3374, 3375, 3376, 3377,
    3383, 3384, 3465, 3466);

UPDATE npc_config_meta SET version = version + 1 WHERE id = TRUE;

INSERT INTO drop_rule (mob, item, chance) VALUES ('*', 3338, 0)
ON CONFLICT (mob, item) DO UPDATE SET chance = 0, updated_at = now();

DELETE FROM drop_rule WHERE item = 3338 AND mob <> '*';

UPDATE drop_rule_meta SET version = version + 1 WHERE id = TRUE;
