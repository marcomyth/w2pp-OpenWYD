-- 0078_honor_store — o God_of_War passa a se chamar "Honor Store" no jogo.
--
-- Ele é o NPC da Loja de Honra (handler/loja_de_honra.go): estava de pé em Armia
-- sem fazer nada, porque o Merchant 104 dele não cai em nenhum tratador, e virou a
-- porta onde os pontos de lojinha se gastam. O nome pedido em 21/09/2026.
--
-- Só o nome muda. O Merchant fica como está, e de propósito: 104 é compartilhado
-- com o Treinador2 e o Uxmal, então ele não identifica a loja e o servidor não o
-- usa para isso — quem identifica é o template_name, traduzido para um Merchant
-- nosso no nascimento do NPC (marcaLojaDeHonra). Mexer no 104 aqui não mudaria
-- nada no jogo, porque o Merchant que chega ao mundo vem dos bytes do template, e
-- não desta coluna.
--
-- A condição é o template, não o slug: o slug carrega o número do bloco do
-- NPCGener (God_of_War-6059) e mudaria se o arquivo mudasse. Se a equipe já tiver
-- renomeado o NPC pelo painel, este UPDATE passa por cima uma vez — é o nome que
-- foi pedido, e depois disso o painel manda de novo.
UPDATE npc_definition
   SET display_name = 'Honor Store', updated_at = now()
 WHERE template_name = 'God_of_War'
   AND display_name <> 'Honor Store';

-- A versão é o que faz o tmServer reler a configuração sem reiniciar.
UPDATE npc_config_meta SET version = version + 1 WHERE id = TRUE;
