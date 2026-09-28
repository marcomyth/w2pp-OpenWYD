-- 0129_terra_mistica_sem_npc — o Cap.Mercenario (bloco 985, 2445,1721) sai. Ele
-- era o NPC da quest das Terras Místicas, que o port nunca terminou: só o primeiro
-- dos três estágios existe (handler.amuMistico) e nada lê a bandeira depois. A
-- quest foi desativada em 12/09/2026, e em 24/09/2026 a equipe pediu o NPC fora —
-- o lugar do mapa fica para outra quest.
--
-- O bloco NÃO sai do NPCGener, porque o índice de um bloco é a posição dele no
-- arquivo; ele desliga pela chave da 0048, a mesma do "/gm npc off", e volta com
-- "/gm npc on 985". TestBlocosDesligadosPorIndice prende o 985 ao Cap.Mercenario.
INSERT INTO npc_generator_off (generator_index, turned_off_by) VALUES (985, 'migração 0129')
ON CONFLICT (generator_index) DO NOTHING;

UPDATE npc_generator_off_meta SET version = version + 1 WHERE id = TRUE;
