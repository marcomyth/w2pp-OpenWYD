-- 0123_pacotes_do_apoiador — os nove pacotes e o de teste, com o que cada um entrega.
--
-- DE ONDE VIERAM OS NÚMEROS, para ninguém ter de confiar na minha memória: os ids, os
-- preços e os Rcoins foram LIDOS de `src/config/pacotes.ts:158-195` no repo do site
-- (marcomyth/w2pp-site). Os Rcoins são o TOTAL, o quinto argumento do `apoiador(...)`,
-- porque o bônus já está dentro dele — creditar a base cobraria o preço cheio e
-- entregaria menos.
--
-- Os brindes estão em TEXTO na lista de recompensas do site ("Montaria Shire (3
-- dias)"), e o mapa para índice foi medido no `Release/Common/ItemList.csv`.
--
-- A CONFERÊNCIA DE PREÇO ACONTECE CONTRA ESTA TABELA. O pedido diz qual pacote; o
-- preço e os créditos que valem são os daqui, e os do pedido são recusados se
-- divergirem. A requisição vem do BFF do site, que é nosso, e ainda assim: quem
-- entrega confere, porque conferir é barato e destrocar item entregue não é.

INSERT INTO donate_pacote (id, credits, amount_cents, so_staff) VALUES
    ('apoiador-iniciante',   300,  2990, FALSE),
    ('apoiador-bronze',      575,  4990, FALSE),
    ('apoiador-prata',      1250,  9990, FALSE),
    ('apoiador-ouro',       2100, 14990, FALSE),
    ('apoiador-platina',    3000, 19990, FALSE),
    ('apoiador-diamante',   4950, 29990, FALSE),
    ('apoiador-mestre',     7000, 39990, FALSE),
    ('apoiador-lenda',     10000, 49990, FALSE),
    ('apoiador-supremo',   20000, 79990, FALSE),
    -- O pacote de R$ 1,00 que existe para a dona do servidor testar pagamento de
    -- verdade sem cobrar R$ 500 de si mesma. Sem brinde, e SÓ STAFF: o site o esconde,
    -- e o servidor também o recusa para quem não é staff. Esconder na tela não é
    -- trava, porque a tela é uma das portas.
    --
    -- O id é literal e combinado com o site: `teste-real`, minúsculas e hífen. O
    -- `teste-100` que existe lá é do gateway falso e não chega aqui.
    ('teste-real',            10,   100, TRUE);

-- OS BRINDES.
--
-- Três regras decidem o que vai em cada linha, e a diferença entre elas é onde a
-- duração mora:
--
--   MONTARIAS: a duração vai em EF_WDAY (106), na forma NÃO INICIADA. O catálogo tem
--   índice próprio só para 15 e 30 dias (3983-3988), e os pacotes vendem 3, 5, 7 e 15
--   — então para 3, 5 e 7 não há índice, e o efeito é o único jeito. Tigre de Fogo e
--   Dragão Vermelho não têm duração padrão no catálogo, então dependem dele sempre.
--
--   FADAS E POÇÕES: a duração é o PRÓPRIO ÍNDICE (Fada Vermelha 3d/5d/7d = 3902/3905/
--   3908; Divina 7/15 = 3379/3380; Sephira 7/15 = 3361/3362). Aqui NÃO se escreve
--   EF_WDAY: o `itemLifetime` faz os efeitos do item ganharem do catálogo, então um
--   EF_WDAY por cima passaria a mandar em vez do índice — e bastaria um número
--   digitado errado para a fada de 7 dias virar de 3.
--
--   EMPILHÁVEIS: a quantidade é EF_AMOUNT (61). Não há coluna de quantidade nesta
--   tabela de propósito: no jogo a quantidade É esse efeito, e uma coluna criaria duas
--   fontes para o mesmo número.
--
-- POR QUE A FORMA NÃO INICIADA IMPORTA para quem compra: com data absoluta, uma
-- montaria de 15 dias comprada numa sexta começa a gastar prazo na hora, mesmo que a
-- pessoa só entre no domingo — e quem viajar duas semanas recebe um item vencido. Com
-- EF_WDAY a contagem começa no PRIMEIRO USO. Quem paga por quinze dias recebe quinze
-- dias de uso, e não quinze dias de calendário a partir de um instante que não
-- escolheu.
INSERT INTO donate_pacote_item (pacote_id, item_index, eff1, effv1, ordem) VALUES
    -- Iniciante: Shire 3d, Fada Verde 3d, 3 Baús de Exp, 2 Baús Bronze.
    ('apoiador-iniciante', 3980, 106,  3, 1),
    ('apoiador-iniciante', 3900,   0,  0, 2),
    ('apoiador-iniciante', 4140,  61,  3, 3),
    ('apoiador-iniciante', 3304,  61,  2, 4),
    -- Bronze: Shire 5d, Fada Vermelha 3d, 5 Baús de Exp, 4 Baús Bronze.
    ('apoiador-bronze',    3980, 106,  5, 1),
    ('apoiador-bronze',    3902,   0,  0, 2),
    ('apoiador-bronze',    4140,  61,  5, 3),
    ('apoiador-bronze',    3304,  61,  4, 4),
    -- Prata: Shire 7d, Fada Vermelha 5d, 10 Baús de Exp, 8 Baús do Apoiador.
    ('apoiador-prata',     3980, 106,  7, 1),
    ('apoiador-prata',     3905,   0,  0, 2),
    ('apoiador-prata',     4140,  61, 10, 3),
    ('apoiador-prata',     3305,  61,  8, 4),
    -- Ouro: Thoroughbred 7d, Fada Vermelha 7d, 10 Baús de Exp, 12 do Apoiador.
    ('apoiador-ouro',      3981, 106,  7, 1),
    ('apoiador-ouro',      3908,   0,  0, 2),
    ('apoiador-ouro',      4140,  61, 10, 3),
    ('apoiador-ouro',      3305,  61, 12, 4),
    -- Platina: Klazedale 7d, Fada Vermelha 7d, 10 Baús de Exp, 16 do Apoiador.
    ('apoiador-platina',   3982, 106,  7, 1),
    ('apoiador-platina',   3908,   0,  0, 2),
    ('apoiador-platina',   4140,  61, 10, 3),
    ('apoiador-platina',   3305,  61, 16, 4),
    -- Diamante: Tigre de Fogo 7d, Fada Vermelha 7d, 10 Baús de Exp, 24 do Apoiador.
    ('apoiador-diamante',  3990, 106,  7, 1),
    ('apoiador-diamante',  3908,   0,  0, 2),
    ('apoiador-diamante',  4140,  61, 10, 3),
    ('apoiador-diamante',  3305,  61, 24, 4),
    -- Mestre: Tigre de Fogo 7d, Fada Vermelha 7d, 10 Exp, Divina 7d, 32 do Apoiador.
    ('apoiador-mestre',    3990, 106,  7, 1),
    ('apoiador-mestre',    3908,   0,  0, 2),
    ('apoiador-mestre',    4140,  61, 10, 3),
    ('apoiador-mestre',    3379,   0,  0, 4),
    ('apoiador-mestre',    3305,  61, 32, 5),
    -- Lenda: Tigre de Fogo 7d, Fada Vermelha 7d, 10 Exp, Divina 7d, Sephira 7d, 40.
    ('apoiador-lenda',     3990, 106,  7, 1),
    ('apoiador-lenda',     3908,   0,  0, 2),
    ('apoiador-lenda',     4140,  61, 10, 3),
    ('apoiador-lenda',     3379,   0,  0, 4),
    ('apoiador-lenda',     3361,   0,  0, 5),
    ('apoiador-lenda',     3305,  61, 40, 6),
    -- Supremo: Dragão Vermelho 15d, Fada Vermelha 7d, 20 Exp, Divina 15d,
    -- Sephira 15d, 64 do Apoiador.
    ('apoiador-supremo',   3991, 106, 15, 1),
    ('apoiador-supremo',   3908,   0,  0, 2),
    ('apoiador-supremo',   4140,  61, 20, 3),
    ('apoiador-supremo',   3380,   0,  0, 4),
    ('apoiador-supremo',   3362,   0,  0, 5),
    ('apoiador-supremo',   3305,  61, 64, 6);

-- O `teste-real` NÃO tem linha aqui, e a ausência é a definição: ele dá 10 Rcoins e
-- nada mais. Um brinde de teste entregaria item de verdade num teste de pagamento, e
-- aí a conta do estoque da Hanna passaria a ter uma origem que ninguém anotou.
