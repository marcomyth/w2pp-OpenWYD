-- 0070_reforma_acessorios — planetas, Amuleto dos Amantes e Amuleto Arcano saem
-- de toda venda.
--
-- Reforma dos acessórios, decidida em 16/09/2026. Os sete planetas (762-768) e o
-- Amuleto dos Amantes (1738) passam a ser raros e só caem de monstro; o Amuleto
-- Arcano (567-570) passa a vir só da evolução do Místico +15 no Odin, e por isso
-- sai também do drop. Os que já estão com jogadores continuam valendo, e a equipe
-- ainda pode dar pelo painel e pelo /gm.
--
-- Esta migração é metade do conserto. A outra metade são as casas zeradas nos
-- templates Imp_Inferno, Imp_Inferno_ e Lich_Mercador: o dbServer ressemeia a
-- loja a partir dos templates em todo boot (INSERT ... ON CONFLICT DO NOTHING),
-- então só apagar aqui faria as vagas voltarem no boot seguinte.

-- 1. Venda em loja de NPC. A versão só sobe se alguma linha saiu.
WITH removidas AS (
    DELETE FROM npc_shop_item
    WHERE item_index IN (762, 763, 764, 765, 766, 767, 768, 1738, 567, 568, 569, 570)
    RETURNING 1
)
UPDATE npc_config_meta SET version = version + 1
WHERE id = TRUE AND EXISTS (SELECT 1 FROM removidas);

-- 1b. O Aki de Armia passa a vender os anéis. Saem das vagas 18-22 dele o Remédio
--     e o Elixir da Coragem e os três Círculos Divinos; o template Aki já traz
--     Hércules, Titã, Athena, Hecate e Zeus nessas vagas, e o dbServer os semeia
--     no boot porque as vagas ficam livres aqui. Só as vagas do Aki com esses
--     cinco itens: a CustomShop continua vendendo os Círculos.
WITH removidas AS (
    DELETE FROM npc_shop_item
    WHERE npc_id IN (SELECT id FROM npc_definition WHERE template_name = 'Aki')
      AND slot IN (18, 19, 20, 21, 22)
      AND item_index IN (646, 647, 693, 694, 695)
    RETURNING 1
)
UPDATE npc_config_meta SET version = version + 1
WHERE id = TRUE AND EXISTS (SELECT 1 FROM removidas);

-- 2. Drop do Arcano: a regra '*' a 0% tira o item de todo monstro (o Dionys o
--    carrega). Planetas e Amantes NÃO entram aqui: continuam caindo, e a
--    raridade deles se ajusta na Mesa de Drops.
INSERT INTO drop_rule (mob, item, chance) VALUES
    ('*', 567, 0),
    ('*', 568, 0),
    ('*', 569, 0),
    ('*', 570, 0)
ON CONFLICT (mob, item) DO UPDATE SET chance = 0, updated_at = now();

DELETE FROM drop_rule WHERE item IN (567, 568, 569, 570) AND mob <> '*';

UPDATE drop_rule_meta SET version = version + 1 WHERE id = TRUE;

-- 3. Loja de doação e recompensa diária: tira da vitrine, sem apagar o histórico.
UPDATE donate_shop_item SET enabled = FALSE, updated_at = now()
WHERE item_index IN (762, 763, 764, 765, 766, 767, 768, 1738, 567, 568, 569, 570) AND enabled;

UPDATE daily_reward_item SET enabled = FALSE, updated_at = now()
WHERE item_index IN (762, 763, 764, 765, 766, 767, 768, 1738, 567, 568, 569, 570) AND enabled;
