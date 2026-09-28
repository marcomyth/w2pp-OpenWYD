-- A volta devolve o estado do teste: os nove reais a R$ 1,00 e os espelhos ligados.
--
-- ELA EXISTE PARA O CASO DE ESTA MIGRAÇÃO NÃO DEVER TER ENTRADO — por exemplo, se o
-- site ainda estivesse vendendo a R$ 1,00 quando ela subiu. NÃO é o caminho de começar
-- uma promoção nova: para isso vai outra migração, com a palavra da Hanna.
UPDATE donate_pacote SET amount_cents = 100, atualizado_em = now() WHERE id IN (
    'apoiador-iniciante', 'apoiador-bronze',   'apoiador-prata',
    'apoiador-ouro',      'apoiador-platina',  'apoiador-diamante',
    'apoiador-mestre',    'apoiador-lenda',    'apoiador-supremo');
UPDATE donate_pacote SET ativo = TRUE, atualizado_em = now() WHERE id LIKE 'teste-apoiador-%';
