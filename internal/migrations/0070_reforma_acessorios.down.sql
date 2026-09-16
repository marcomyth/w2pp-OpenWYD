-- Volta só as regras '*' do Arcano. As vagas de loja do Imp_Inferno (Vênus e
-- Marte) voltam sozinhas se os templates forem revertidos, porque o dbServer
-- ressemeia a loja em todo boot. A loja de doação e a recompensa diária NÃO
-- voltam: não há como saber quais linhas estavam ligadas.
DELETE FROM drop_rule WHERE mob = '*' AND item IN (567, 568, 569, 570);
UPDATE drop_rule_meta SET version = version + 1 WHERE id = TRUE;
