-- Tira os âmagos da Dungeon. A XP volta pelos templates revertidos no git (o
-- overlay do painel só existe para quem foi editado à mão, e o valor anterior
-- dele não é conhecido aqui), e o respawn do Demon Lord pelo NPCGener.txt.
DELETE FROM drop_rule
WHERE item IN (2392, 2393, 2394, 2396, 2397, 2398, 2401, 2402, 2403)
  AND mob IN (
    'Caveira', 'Urso_Zumbi', 'Troll_Zumbi', 'Arq_Caveira', 'Caveira_Lanc', 'Conj_Caveira',
    'Hidra', 'Cav.Caveira', 'Hidra_Dourada_', 'Elfo_Negro', 'Guer_Caveira',
    'Anf_Assassino', 'Bruxa_Elfica', 'Gargula_Sabio', 'Gargula', 'Golem_de_Fogo',
    'Anf_Ninja', 'Grim_Lock', 'Gargula_Inf', 'Gargula_Servo', 'Golem_de_Pedra',
    'Hezling', 'Dragao_Lich', 'Cav._Mortal', 'Cavaleiro_Negro', 'Demon_Lord');

UPDATE drop_rule_meta SET version = version + 1 WHERE id = TRUE;
