-- Devolve os 25 comerciantes da praça de Armia ao mundo.
UPDATE npc_definition SET enabled = TRUE
WHERE template_name IN (
    'Acessorios', 'Acessorios2', 'Acessorios3', 'Acessorios4', 'Acessorios5', 'Acessorios6',
    'Arma_Arch', 'Arma_Mortal', 'Arma_SemiDeus',
    'Set_TK', 'Set_FM', 'Set_BM', 'Set_HT',
    'Runas_Joias', 'Tintas', 'Trajes', 'Trajes2',
    'Gold', 'Evento', 'Evolucao', 'Fadas', 'Entradas',
    'Montarias', 'Montarias_2', 'Guarda_da_Sorte');

UPDATE npc_config_meta SET version = version + 1 WHERE id = TRUE;
