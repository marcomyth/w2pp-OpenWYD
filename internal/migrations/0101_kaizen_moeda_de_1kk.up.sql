-- 0101_kaizen_moeda_de_1kk — a Moeda de 5 milhões fica só nas Hidras e nos
-- Elfos; no Kaizen passa a ser a de 1 milhão.
--
-- Pedido do Marco em 22/09/2026, na sequência da 0099: "Barra 5KK podemos
-- dropar nas Hidras, Elfos, mas nas outras 1KK".
--
-- Onde a Moeda de 5 milhões (4027) cai hoje, depois da 0099 ter tirado a
-- Aparição (que era 2,857% e respondia por quase tudo):
--
--   Cav._Servo      Kaizen   slots 40-44   0,05% cada   <-- esta migração
--   Servo_Elfo      Elfos    slots 40-43   0,05% cada   fica
--   Hidra_Imortal   Hidras   slots 48,49   0,0333%      fica
--                            slot 62       0,0263%      fica
--
-- Sobra só o Cav. Servo. Os cinco slots dele somam 0,25% de moeda por morte, e
-- a regra os substitui por uma linha de 1 milhão com a mesma frequência.
--
-- A chance escrita é 20 e não 25 pelo mesmo motivo da 0099: o sorteio da Mesa é
-- rand()%10000 sobre um rand() que para em 32767, e infla toda chance baixa em
-- 22,07%. 20 x 4 / 32768 = 0,244%, que é o que os cinco slots entregam. O teste
-- mede as duas e recusa uma diferença maior que 0,05 ponto.
--
-- O Jardim dos Deuses não entra: nenhum monstro dele carrega moeda.

INSERT INTO drop_rule (mob, item, chance) VALUES
    ('Cav._Servo', 4027,  0),
    ('Cav._Servo', 4026, 20)
ON CONFLICT (mob, item) DO UPDATE SET chance = EXCLUDED.chance, updated_at = now();

UPDATE drop_rule_meta SET version = version + 1 WHERE id = TRUE;
