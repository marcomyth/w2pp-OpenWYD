-- 0184_reino_bruxa_lanceiro_saque — a Bruxa e o Lanceiro do Reino trocam o
-- Âmago de Andaluz pelo de Cavalo Equipado e ganham Safira e Classe D (pedido do
-- Marco em 29/09/2026).
--
-- Só os dois, nos dois reinos: Combatente, Guarda do Rei e Virago seguem com a
-- mesa da 0074. O âmago continua seguindo a cor do reino, como o Andaluz fazia:
-- Equipado B (2404) em Hekalotia, o azul, e Equipado N (2399) em Akelonia, o
-- vermelho (nome com "_"). Mesma chance do Andaluz (200) e o mesmo pacote de 5
-- (reinosPacotes, handler/reinos.go).
--
-- Safira (697) a 100 e Classe D (4019) a 300, uma unidade cada. A Poeira de
-- Lactolerium fica como está (300), escolha do Marco.
--
-- A Mesa sorteia rand() % 10000 sobre o rand() de 15 bits do MSVC, e abaixo de
-- 27,68% paga 4/3,2768 do escrito: 200 sai 2,44%, 100 sai 1,22% e 300 sai 3,66%.
INSERT INTO drop_rule (mob, item, chance) VALUES
    ('Bruxa',     2405, 0), ('Bruxa',     2404, 200), ('Bruxa',     697, 100), ('Bruxa',     4019, 300),
    ('Lanceiro',  2405, 0), ('Lanceiro',  2404, 200), ('Lanceiro',  697, 100), ('Lanceiro',  4019, 300),
    ('Bruxa_',    2400, 0), ('Bruxa_',    2399, 200), ('Bruxa_',    697, 100), ('Bruxa_',    4019, 300),
    ('Lanceiro_', 2400, 0), ('Lanceiro_', 2399, 200), ('Lanceiro_', 697, 100), ('Lanceiro_', 4019, 300)
ON CONFLICT (mob, item) DO UPDATE SET chance = EXCLUDED.chance, updated_at = now();

UPDATE drop_rule_meta SET version = version + 1 WHERE id = TRUE;
