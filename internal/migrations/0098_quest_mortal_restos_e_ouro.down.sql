-- Devolve os Restos das três arenas às chances da 0065/0075 e apaga as linhas de
-- recompensa, que é o que faz o ouro do troféu voltar a sair de QuestsRate.txt.
--
-- Não é um "x2" simétrico de propósito: a volta é para os números que as
-- migrações escreveram, porque é isso que alguém quer ao desfazer. Uma linha que
-- o painel tenha ajustado depois é perdida nos dois sentidos — o mesmo que
-- acontece com toda migração de Mesa aqui.
UPDATE drop_rule SET chance = 5000, updated_at = now()
 WHERE item = 419 AND mob IN ('Cav._Kaizen', 'Hidra_Dourada', 'Mestre_Elfo');
UPDATE drop_rule SET chance = 3000, updated_at = now()
 WHERE item = 420 AND mob IN ('Cav._Kaizen', 'Hidra_Dourada', 'Mestre_Elfo');
UPDATE drop_rule SET chance = 1500, updated_at = now() WHERE item = 419 AND mob = 'Cav._Servo';
UPDATE drop_rule SET chance =  800, updated_at = now() WHERE item = 420 AND mob = 'Cav._Servo';
UPDATE drop_rule SET chance = 1000, updated_at = now()
 WHERE item = 419 AND mob IN ('Hidra_Imortal', 'Servo_Elfo');
UPDATE drop_rule SET chance =  500, updated_at = now()
 WHERE item = 420 AND mob IN ('Hidra_Imortal', 'Servo_Elfo');

UPDATE drop_rule_meta SET version = version + 1 WHERE id = TRUE;

DELETE FROM quest_reward WHERE tier BETWEEN 0 AND 4;

UPDATE quest_reward_meta SET version = version + 1 WHERE id = TRUE;
