-- Tira as regras do Submundo. Os monstros voltam a soltar o que o template diz,
-- inclusive o Cavalo Equipado no Morlock e no Demon Gorgon; o respawn do chefe
-- volta pelo NPCGener.txt revertido no git.
DELETE FROM drop_rule WHERE mob IN (
    'Argos_Errante', 'Troll_Ghoul', 'Aqua_Golem', 'Morlock', 'Demon_Gorgon', 'CH_Troll_Ghoul',
    'Cav._Elfo_Negro', 'Elfo_Negro_Abj', 'FrenzyDemonLord')
  AND item IN (2396, 2397, 2398, 2399, 2401, 2402, 2403, 2404, 2309, 2314, 3173, 4018, 4019, 3224, 1759);

UPDATE drop_rule_meta SET version = version + 1 WHERE id = TRUE;
