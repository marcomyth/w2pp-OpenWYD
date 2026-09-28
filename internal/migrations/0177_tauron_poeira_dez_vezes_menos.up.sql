-- 0177_tauron_poeira_dez_vezes_menos — a Poeira de Oriharucon e a de Lactolerium
-- dos Tauron caem 10 vezes menos (pedido do Marco em 28/09/2026).
--
-- O Tauron do Pilar virou fazenda: uma conta deixada 8 horas seguidas soltou 92
-- Poeiras de Oriharucon e 31 de Lactolerium, e eram cinco contas — num lugar que
-- é de XP. A 0109 dava aos onze monstros comuns do deserto Poeira de Oriharucon a
-- 1% e de Lactolerium a 0,5% (100 e 50 na Mesa). Aqui ficam 10 e 5.
--
-- Os cinco Tauron, todos com a mesma mesa: o Tauron, o Ladrão, o Arqueiro, o Aeon
-- e o Adamant. Os outros seis comuns do deserto não mudam — o pedido foi o Tauron,
-- e baixar só um deles faria o farm mudar para o vizinho.
--
-- A Mesa sorteia rand() % 10000 sobre o rand() de 15 bits do MSVC, e abaixo de
-- 27,68% paga 4/3,2768 do escrito: 100 saía 1,22% e passa a 0,122%; 50 saía 0,61%
-- e passa a 0,061%. O desvio é o mesmo nos dois números, então a razão é
-- exatamente 10.
--
-- O renascimento do Pilar sobe no mesmo PR (no máximo 10 s), e mais abate por
-- hora come parte desses 10×: por hora o drop cai menos que isso.
INSERT INTO drop_rule (mob, item, chance) VALUES
    ('Tauron',          412, 10), ('Tauron',          413, 5),
    ('Ladrao_Tauron',   412, 10), ('Ladrao_Tauron',   413, 5),
    ('Arqueiro_Tauron', 412, 10), ('Arqueiro_Tauron', 413, 5),
    ('Aeon_Tauron',     412, 10), ('Aeon_Tauron',     413, 5),
    ('Adamant_Tauron',  412, 10), ('Adamant_Tauron',  413, 5)
ON CONFLICT (mob, item) DO UPDATE SET chance = EXCLUDED.chance, updated_at = now();

UPDATE drop_rule_meta SET version = version + 1 WHERE id = TRUE;
