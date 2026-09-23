-- 0082_azran_sem_vendedores — a fila de vendedores de Azran sai do mundo.
--
-- Sete NPCs enfileirados em 2496-2514, 1709: os quatro de armadura
-- (Ferreiro/Rainy/Rapein/Arnod de Azran), os dois de arma e o DonatesBars.
-- Os de arma vendiam Anciãs +14 com add — Balmung, Caliburn, Basileus, Asa
-- Draconiana, Éden, Karikas — pelo mesmo motivo que a 0079 tirou os sets top das
-- lojas de armadura: era o melhor equipamento do jogo saindo de NPC por ouro.
--
-- Desativa, não apaga. A definição fica na tabela (e o bloco no NPCGener.txt),
-- porque o slug de um NPC de conteúdo é "<template>-<posição do bloco no
-- arquivo>": remover blocos desloca o índice de todos os que vêm depois e
-- renomeia os slugs deles, que é como 44 NPCs já apareceram duplicados uma vez.
-- Com enabled = FALSE o tmServer simplesmente não materializa a definição, e
-- reativar é um UPDATE (ou um clique no painel).
--
-- Por template_name: qualquer definição com um destes templates sai, inclusive a
-- cópia que o painel tenha criado — no print havia dois DonatesBars no mesmo
-- ponto, e o pedido foi retirar todos.
--
-- Vale na hora: o poll de configuração do tmServer re-materializa o conjunto
-- gerenciado quando a versão sobe, sem esperar reinício.
UPDATE npc_definition SET enabled = FALSE
WHERE template_name IN (
    'Ferreiro_Azran', 'Rainy_Azran', 'Rapein_Azran', 'Arnod_Azran',
    'ArmaMortalAzran', 'ArmaArchAzran', 'DonatesBars');

UPDATE npc_config_meta SET version = version + 1 WHERE id = TRUE;
