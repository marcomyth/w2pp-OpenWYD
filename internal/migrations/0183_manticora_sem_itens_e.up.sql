-- 0183_manticora_sem_itens_e — a Mantícora comum do Deserto deixa de soltar
-- armas E e peças do set E (pedido do Marco em 29/09/2026: "Armas E e set E na
-- mantícora não é para dropar").
--
-- A 0109 e a 0180 cuidaram das Poeiras, dos âmagos e das Armas D da Mantícora,
-- mas o resto do template seguia valendo, e nele estavam:
--   vagas 0-2   Vingadora (3571), Asa Draconiana (3551) e Éden (3591), 1 em 891
--               cada (0,113%) — uma arma E a cada ~300 abates somando as três;
--   vagas 40-47 os elmos dos quatro sets E (Mortal 1225, Templário 1360, do
--               Corvo 1510, Legionário 1660), duas vagas de 0,052% cada.
-- A Éden é a espada que leva o TK ao teto de 9.000 de ataque.
--
-- Uma regra a 0% tira o item do template só deste monstro (droprule.Governs).
-- Vale só para o template 'Manticora', o do Deserto: a Manticora@ (bloco 318) e
-- a Manticora_ do Pesadelo são outros arquivos e não mudam aqui. As Armas D
-- (Vorpal, Solaris, Martelo Assassino) continuam, com o add da 0180.
INSERT INTO drop_rule (mob, item, chance) VALUES
    ('Manticora', 3551, 0),   -- Asa Draconiana
    ('Manticora', 3571, 0),   -- Vingadora
    ('Manticora', 3591, 0),   -- Éden
    ('Manticora', 1225, 0),   -- Elmo Mortal
    ('Manticora', 1360, 0),   -- Elmo Templário
    ('Manticora', 1510, 0),   -- Elmo do Corvo
    ('Manticora', 1660, 0)    -- Elmo Legionário
ON CONFLICT (mob, item) DO UPDATE SET chance = EXCLUDED.chance, updated_at = now();

UPDATE drop_rule_meta SET version = version + 1 WHERE id = TRUE;
