-- 0149_boss_hidra_dourada — a Hidra Dourada do 2º andar da Dungeon vira chefe e
-- passa a ser a dona da Pedra do Esqueleto; a escolta de Guer Caveira fica 6x mais
-- forte.
--
-- Pedido do Marco em 25/09/2026, com print em (740,3776): o bloco 2099, uma Hidra
-- Dourada liderando cinco Guer Caveira.
--
-- O chefe (template Boss_Hidra_Dourada, bloco 6161) é o molde do Boss Dragão Lich
-- da 0140: os números do Boss Mantícora, 4 horas entre as mortes, e o saque dela
-- trocando a pedra. A Pedra do Esqueleto (1753), que a 0140 tirou do jogo, volta
-- só aqui, a 81 = 0,99% por morte, a régua das Pedras Arch (0136). Os quatro
-- prêmios sorteados são código (handler/dungeon.go).
--
-- A escolta é um template novo, Guer_Caveira_Escolta, para os outros 99 Guer
-- Caveira da Dungeon não mudarem: 52.200 de vida e dano 2.988 (6x). Como a Mesa é
-- por arquivo de template, ela herda aqui tudo o que o Guer_Caveira tem hoje na
-- Mesa, inclusive o que o painel gravou, e a pedra fica a 0% nela também.
INSERT INTO drop_rule (mob, item, chance)
SELECT 'Guer_Caveira_Escolta', item, chance FROM drop_rule WHERE mob = 'Guer_Caveira'
ON CONFLICT (mob, item) DO UPDATE SET chance = EXCLUDED.chance, updated_at = now();

INSERT INTO drop_rule (mob, item, chance) VALUES
    ('Guer_Caveira_Escolta', 1753,  0),   -- Pedra do Esqueleto
    ('Boss_Hidra_Dourada',   1753, 81)    -- Pedra do Esqueleto
ON CONFLICT (mob, item) DO UPDATE SET chance = EXCLUDED.chance, updated_at = now();

UPDATE drop_rule_meta SET version = version + 1 WHERE id = TRUE;
