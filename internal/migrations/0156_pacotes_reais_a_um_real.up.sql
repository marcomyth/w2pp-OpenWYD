-- 0156_pacotes_reais_a_um_real — os NOVE PACOTES DE VERDADE passam a custar R$ 1,00.
--
-- ORDEM DA HANNA, 25/09/2026, autorizada por escrito e com o custo na mesa: "os preços
-- voltam quando eu mandar". Os espelhos da 0149 não serviram para o que ela precisava.
--
-- O QUE ISTO CUSTA, escrito aqui porque quem ler depois precisa saber: enquanto esta
-- migração estiver valendo, QUALQUER conta logada compra o Supremo — 20.000 Rcoins e
-- seis brindes — por R$ 1,00. Os pacotes reais NÃO têm trava de staff, porque eles são a
-- loja; a trava `so_staff` existe só nos espelhos e no `teste-real`. Era exatamente isso
-- que os espelhos existiam para evitar, e a decisão de pagar esse preço é dela.
--
-- VOLTAR = MIGRAÇÃO NOVA com os preços da 0123, quando ela mandar. NÃO se volta pelo
-- down desta: o down existe para desfazer uma migração que não devia ter entrado, e
-- desfazer deploy antigo em produção é outra coisa, e mais arriscada, do que mudar um
-- preço de propósito.
--
-- OS CREDITS E OS BRINDES NÃO MUDAM. Só o preço — e é a conferência de preço do
-- `ConferirPacote` que faz o pedido do site casar com esta linha, então o site tem de
-- mandar 100 a partir daqui.
UPDATE donate_pacote
   SET amount_cents = 100, atualizado_em = now()
 WHERE id IN (
        'apoiador-iniciante', 'apoiador-bronze',   'apoiador-prata',
        'apoiador-ouro',      'apoiador-platina',  'apoiador-diamante',
        'apoiador-mestre',    'apoiador-lenda',    'apoiador-supremo'
       );
