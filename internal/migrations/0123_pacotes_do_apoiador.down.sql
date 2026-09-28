DELETE FROM donate_pacote_item WHERE pacote_id IN (
    'apoiador-iniciante','apoiador-bronze','apoiador-prata','apoiador-ouro',
    'apoiador-platina','apoiador-diamante','apoiador-mestre','apoiador-lenda',
    'apoiador-supremo','teste-real');
DELETE FROM donate_pacote WHERE id IN (
    'apoiador-iniciante','apoiador-bronze','apoiador-prata','apoiador-ouro',
    'apoiador-platina','apoiador-diamante','apoiador-mestre','apoiador-lenda',
    'apoiador-supremo','teste-real');
