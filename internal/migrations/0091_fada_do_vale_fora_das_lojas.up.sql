-- 0091_fada_do_vale_fora_das_lojas — nenhum comerciante vende a chave do Vale.
--
-- A Fada do Vale (3916) é a ÚNICA porta do Vale Escondido: sem ela no slot 13 o
-- piso de Azran não leva a lugar nenhum, e a varredura tira de lá quem ficar
-- dentro sem ela (handler/vale.go, GetFunc.cpp:924-931). Decisão de 21/09/2026:
-- por enquanto ela não se compra de NPC.
--
-- Um único vendedor no jogo inteiro: o NPC Fadas da praça de Armia, na vaga 19
-- (Carry[55] do template). Ele já está fora do mundo desde a 0083, que desativou
-- os 25 comerciantes da praça para o lançamento — esta migração é o que impede a
-- fada de voltar junto com ele no dia em que for reativado.
--
-- O DELETE é por item e não por NPC de propósito: o slug de um NPC de conteúdo é
-- "<template>-<posição do bloco>", e a posição anda quando alguém edita o
-- NPCGener.txt. O seed da 0006 ainda aponta para "Fadas-6080", que hoje é
-- "Fadas-6076" — apagar por slug apagaria nada.
--
-- Como sempre, as duas metades: Release/TMsrv/run/npc/Fadas perde a vaga no
-- mesmo commit, senão o dbServer a recoloca no boot seguinte.
--
-- Os dois Mapa_Vale_Escondido (3909, 3910) continuam nas vagas 25 e 26: não são
-- a chave e não abrem coisa alguma — nem aqui nem no legado, onde nenhuma linha
-- de Source/Code/TMSrv os lê.
DELETE FROM npc_shop_item WHERE item_index = 3916;

UPDATE npc_config_meta SET version = version + 1 WHERE id = TRUE;
