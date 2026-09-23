-- 0083_armia_praça_limpa — os 25 comerciantes do aglomerado de Armia saem do
-- mundo (limpeza para o lançamento).
--
-- É a fila e o quadrado de vendedores entre 2085-2109 e 2090-2107: seis de
-- acessórios, três de arma, quatro de set, Runas/Joias, Tintas, Trajes, Trajes2,
-- Gold, Evento, Evolução, Fadas, Entradas, as duas Montarias e o Guarda da
-- Sorte. Nenhum destes templates tem bloco fora dessa praça, então desativar por
-- template_name não respinga em outra cidade.
--
-- Desativa, não apaga — mesma razão da 0082: o slug de um NPC de conteúdo é
-- "<template>-<posição do bloco no NPCGener>", e tirar blocos do arquivo desloca
-- o índice de todos os seguintes. Reativar é um UPDATE ou um clique no painel.
--
-- Vale na hora: o poll de configuração re-materializa o conjunto gerenciado
-- quando a versão sobe.
UPDATE npc_definition SET enabled = FALSE
WHERE template_name IN (
    'Acessorios', 'Acessorios2', 'Acessorios3', 'Acessorios4', 'Acessorios5', 'Acessorios6',
    'Arma_Arch', 'Arma_Mortal', 'Arma_SemiDeus',
    'Set_TK', 'Set_FM', 'Set_BM', 'Set_HT',
    'Runas_Joias', 'Tintas', 'Trajes', 'Trajes2',
    'Gold', 'Evento', 'Evolucao', 'Fadas', 'Entradas',
    'Montarias', 'Montarias_2', 'Guarda_da_Sorte');

UPDATE npc_config_meta SET version = version + 1 WHERE id = TRUE;
