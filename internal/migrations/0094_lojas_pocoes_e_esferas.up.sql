-- 0094_lojas_pocoes_e_esferas — os dez itens da vitrine do Martin saem de toda
-- loja de NPC, e a limpeza das 0091 e 0092 é refeita sobre os templates que
-- faltaram.
--
-- Os dez (pedido de 21/09/2026, olhando a loja do Martin em jogo):
--   3322/3323  as Caixas de Poção de Cura e de Mana — as mais espalhadas,
--              em quinze vitrines
--   3431       a Poção Poderosa, que cura 1.000 de HP e de MP, o dobro do teto
--              de uma poção comum
--   415        as Ervas de Cura
--   3381/3363/3366  as Poções Divina, Sephira e de Saúde de TRINTA DIAS: um
--              buff de mês inteiro numa compra
--   4128/4129/4130  as três Esferas da Sorte, que custam 200.000 e são inertes
--              — nenhuma linha de código as lê, aqui ou no legado
-- Depois disto o Martin fica com a vitrine vazia; o NPC continua no mundo.
--
-- A SEGUNDA METADE desta migração conserta um erro meu. A 0091 (Fada do Vale) e
-- a 0092 (Coral, Lactolerium 100) foram escritas a partir de uma varredura que
-- só aceitava templates de 816 bytes. Release/TMsrv/run/npc/ tem TRÊS layouts:
-- 1.792 arquivos de 816, 207 de 756 e 15 de 756 com lixo no fim (920). Os 222
-- do layout legado ficaram sem olhar, e neles sobraram a Fada do Vale (no
-- Utilidades), o Coral (h21horas, Utilidades) e o Lactolerium 100 (h24horas,
-- Outros). Os templates saem limpos neste commit.
--
-- O DELETE dos três é repetido aqui de propósito, e não é redundante: num boot
-- em que as 0091/0092 já rodaram mas os templates ainda estavam sujos, a seed
-- reinseriu as linhas logo depois da migração (store.Migrate roda ANTES de
-- SeedNPCDefinitions). Apagar de novo, agora com os arquivos limpos, é o que
-- fecha o ciclo.
--
-- Nenhuma das dez lojas do layout legado nasce no mundo hoje — nenhum bloco do
-- NPCGener.txt aponta para elas —, então a Fada do Vale não chegou a ficar
-- comprável. O template é a fonte viva da vitrine mesmo assim: basta alguém
-- ligar o NPC pelo painel para o item voltar.
DELETE FROM npc_shop_item WHERE item_index IN (
    3322, 3323,
    3431, 415,
    3381, 3363, 3366,
    4128, 4129, 4130);

DELETE FROM npc_shop_item WHERE item_index IN (3916, 2443, 4141);

UPDATE npc_config_meta SET version = version + 1 WHERE id = TRUE;
