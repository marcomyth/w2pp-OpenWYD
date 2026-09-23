-- Tira as regras do Deserto. Os monstros voltam a soltar o que o template diz,
-- inclusive a Pedra do Lugefer em 1 de cada 4 mortes; a Chave do Rei Orc (465,
-- da 0053) fica. Os blocos do Cav. Lugefer voltam a 10 pelo NPCGener.txt
-- revertido no git, e os dois Agmo voltam a nascer no boot.
DELETE FROM drop_rule WHERE mob IN (
    'Tauron', 'Ladrao_Tauron', 'Aranha_Inferno', 'Taron_Assassino', 'Arqueiro_Tauron',
    'Verme_', 'Manticora', 'Aeon_Tauron', 'Treant', 'Lugefer', 'Adamant_Tauron',
    'Cav._Lugefer', 'Tauron_Agmo', 'Verme_Agmo')
  AND item IN (412, 413, 4026, 2396, 2397, 2398, 2399, 2309, 4018, 4019, 4020,
               2406, 2316, 2441, 2442, 2443, 2444, 1756, 1758, 2400, 2405, 3224);

UPDATE drop_rule_meta SET version = version + 1 WHERE id = TRUE;

DELETE FROM npc_generator_off
WHERE generator_index IN (3451, 3452) AND turned_off_by = 'migração 0108';

UPDATE npc_generator_off_meta SET version = version + 1 WHERE id = TRUE;
