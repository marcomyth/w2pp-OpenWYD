-- 0103_pocoes_em_pilha_de_120 — as poções de 500 saem de 120 em 120, e pagam por
-- unidade (pedido de 22/09/2026).
--
-- A 0100 as pôs uma por compra, e uma por compra é inviável na prática: ninguém
-- clica cento e vinte vezes para encher a bolsa. A razão de terem saído assim
-- era o preço — o servidor cobra o preço do catálogo por COMPRA, não por
-- unidade, e uma pilha de 120 sairia por 2.000, dezesseis de ouro a poção.
--
-- A resposta não foi baratear a poção nem encarecer o jogo inteiro: a compra
-- destes DOIS índices passa a pagar por unidade (handler/shop.go,
-- cobradasPorUnidade), e a pilha de 120 custa 240.000 — exatamente o que o
-- jogador pagaria comprando uma a uma.
--
-- O resto do jogo fica como está, e é muita coisa: 252 vagas em 46 lojas vendem
-- em pilha hoje pelo preço de uma unidade (rações de sessenta, pergaminhos de
-- dez, Pedidos de Caça, Barras de Mithril). Corrigir todas de uma vez
-- reprecificaria o servidor inteiro sem ninguém ter pedido — fica registrado
-- como decisão pendente, não como dívida escondida.
--
-- As duas metades de sempre: os templates Release/TMsrv/run/npc/{Aki,Martin}
-- mudam no mesmo commit, senão o dbServer ressemeia a quantidade velha no boot.

UPDATE npc_shop_item
SET quantity = 120
WHERE item_index IN (404, 409)
  AND npc_id IN (SELECT id FROM npc_definition WHERE template_name IN ('Aki', 'Martin'));

UPDATE npc_config_meta SET version = version + 1 WHERE id = TRUE;
