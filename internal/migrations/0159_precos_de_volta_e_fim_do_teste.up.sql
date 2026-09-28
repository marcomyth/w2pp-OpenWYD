-- 0159_precos_de_volta_e_fim_do_teste — acaba a promoção de R$ 1,00 e fecha os espelhos.
--
-- MIGRAÇÃO NOVA, E NÃO O `down` DA 0158. Desfazer uma migração já aplicada em produção é
-- operação de conserto — ela diz "isto não devia ter entrado". Aqui a 0158 devia entrar,
-- entrou, serviu para o teste e agora acabou. Mudar preço é rotina, e rotina vai para a
-- frente. Confundir as duas é como um "voltar tudo" vira um susto.
--
-- OS VALORES VÊM DA 0123, que é onde eles nasceram (lidos do site, src/config/pacotes.ts),
-- e não do `down` da 0158. As duas fontes concordam hoje, valor por valor — conferido —,
-- mas a 0123 é a original e o `down` é cópia. Cópia é o lugar onde um número erra sozinho.
--
-- Uma linha por pacote, e não um CASE: assim a revisão mostra preço por preço, e é
-- exatamente isso que alguém confere quando os valores voltam.
UPDATE donate_pacote SET amount_cents =  2990, atualizado_em = now() WHERE id = 'apoiador-iniciante';
UPDATE donate_pacote SET amount_cents =  4990, atualizado_em = now() WHERE id = 'apoiador-bronze';
UPDATE donate_pacote SET amount_cents =  9990, atualizado_em = now() WHERE id = 'apoiador-prata';
UPDATE donate_pacote SET amount_cents = 14990, atualizado_em = now() WHERE id = 'apoiador-ouro';
UPDATE donate_pacote SET amount_cents = 19990, atualizado_em = now() WHERE id = 'apoiador-platina';
UPDATE donate_pacote SET amount_cents = 29990, atualizado_em = now() WHERE id = 'apoiador-diamante';
UPDATE donate_pacote SET amount_cents = 39990, atualizado_em = now() WHERE id = 'apoiador-mestre';
UPDATE donate_pacote SET amount_cents = 49990, atualizado_em = now() WHERE id = 'apoiador-lenda';
UPDATE donate_pacote SET amount_cents = 79990, atualizado_em = now() WHERE id = 'apoiador-supremo';

-- OS ESPELHOS SAEM DE CENA, DESLIGADOS E NÃO APAGADOS. É o caminho que a própria 0149
-- escreveu, e a regra é da 0122: um pedido antigo que ainda não confirmou precisa
-- continuar achando a linha dele, senão a pessoa paga e o servidor não sabe o que
-- prometeu. Apagar as nove linhas transformaria um Pix em trânsito em dinheiro recebido
-- sem contrapartida.
--
-- Pelo prefixo, e não por uma lista de nomes: os nove nasceram por `'teste-' || p.id` na
-- 0149, então o prefixo é o que os define. Uma lista digitada aqui deixaria de fora
-- qualquer espelho que alguém acrescentasse depois.
UPDATE donate_pacote SET ativo = FALSE, atualizado_em = now() WHERE id LIKE 'teste-apoiador-%';
