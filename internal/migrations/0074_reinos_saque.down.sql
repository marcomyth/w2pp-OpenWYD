DELETE FROM drop_rule WHERE mob = '*' AND item IN (1740, 1741, 1742);
DELETE FROM drop_rule WHERE mob IN (
    'Rei_Harabard', 'Rei_Glantuar', 'Escolta_Real', 'Escolta_Real_',
    'Cav._Real', 'Cav._Real_', 'Averest', 'Averest_', 'Feiticeira', 'Feiticeira_',
    'Guarda_do_Rei', 'Guarda_do_Rei_', 'Combatente', 'Combatente_', 'Lanceiro', 'Lanceiro_',
    'Virago', 'Virago_', 'Bruxa', 'Bruxa_'
) AND item IN (1740, 1741, 3224, 2400, 2405, 2406, 2408, 4026, 4027, 412, 413, 4016, 4017, 4018);
UPDATE drop_rule_meta SET version = version + 1 WHERE id = TRUE;
