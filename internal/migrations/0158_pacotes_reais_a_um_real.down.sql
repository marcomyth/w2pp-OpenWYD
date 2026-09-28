-- A volta restaura os preços que a 0123 leu do site (src/config/pacotes.ts).
--
-- ELA EXISTE PARA O CASO DE ESTA MIGRAÇÃO NÃO DEVER TER ENTRADO, e NÃO é o caminho de
-- acabar a promoção. Para acabar vai uma migração NOVA, quando a Hanna mandar: desfazer
-- uma migração já aplicada em produção é operação de conserto, e mudar preço é operação
-- de rotina. Confundir as duas é como um "voltar tudo" vira um susto.
--
-- Uma linha por pacote, e não um CASE: assim o diff de uma revisão mostra preço por
-- preço, e é exatamente isso que alguém confere quando os valores voltam.
UPDATE donate_pacote SET amount_cents = 2990,  atualizado_em = now() WHERE id = 'apoiador-iniciante';
UPDATE donate_pacote SET amount_cents = 4990,  atualizado_em = now() WHERE id = 'apoiador-bronze';
UPDATE donate_pacote SET amount_cents = 9990,  atualizado_em = now() WHERE id = 'apoiador-prata';
UPDATE donate_pacote SET amount_cents = 14990, atualizado_em = now() WHERE id = 'apoiador-ouro';
UPDATE donate_pacote SET amount_cents = 19990, atualizado_em = now() WHERE id = 'apoiador-platina';
UPDATE donate_pacote SET amount_cents = 29990, atualizado_em = now() WHERE id = 'apoiador-diamante';
UPDATE donate_pacote SET amount_cents = 39990, atualizado_em = now() WHERE id = 'apoiador-mestre';
UPDATE donate_pacote SET amount_cents = 49990, atualizado_em = now() WHERE id = 'apoiador-lenda';
UPDATE donate_pacote SET amount_cents = 79990, atualizado_em = now() WHERE id = 'apoiador-supremo';
