-- 0092_lojas_gemas_repletions_e_caca — ajustes de vitrine pedidos em 21/09/2026,
-- todos na mesma família da 0086 (limpeza de loja para o lançamento).
--
-- 1. O Coral (2443) sai de TODA loja. A 0086 tirou o Diamante, a Esmeralda e o
--    Garnet e deixou o Coral para trás — era a quarta pedra da mesma família, e
--    a falha apareceu no Parche, em Erion. O drop de monstro NÃO é tocado: o
--    Coral continua caindo, como as outras três.
--
-- 2. O Lactolerium 100 (4141) sai de TODA loja. A 0088 tirou o item do drop
--    porque é o refino garantido (força o âmago a 100% na Mesa das Máquinas) e
--    disse, com todas as letras, que as lojas ficariam para depois. É este
--    depois: sete vitrines o vendiam, entre elas a Aki e a Loja de Pontos.
--
-- 3. O Pedido de Caça (3432-3437) passa a sair de dez em dez. A 0086 baixou todo
--    pergaminho para dez por uma lista de índices, e os seis Pedidos não estavam
--    nela: vinham de 240 (Acessorios3), 100 (Parche, Martin_) e 20 (Holis).
--    Mesma regra da 0086, "só quem está acima de dez", para não inflar quem
--    vende a unidade.
--
-- 4. A Lucy, em Azran (2550,1716), é a loja que o pedido chama de "organizar":
--    a Classe C e a Classe D saem, a Classe B encosta na Classe A (vaga 2 -> 1)
--    e as quatro Gemas ficam juntas e em ordem nas vagas 18-21 — Diamante,
--    Esmeralda, Coral (que já estava na 20) e Garnet. As três novas voltam a uma
--    loja de onde a 0086 as tinha tirado; é reversão deliberada, e só na Lucy.
--    Endereçada por template_name porque a Lucy tem um bloco só no NPCGener, e
--    porque o slug carrega a posição do bloco, que anda quando o arquivo é
--    editado.
--
-- As duas metades de sempre: os 14 templates de Release/TMsrv/run/npc/ mudam no
-- mesmo commit. Sem isso o dbServer ressemeia no boot seguinte o que o DELETE
-- daqui apagou — e, do outro lado, sem o INSERT daqui as Gemas novas da Lucy só
-- apareceriam no próximo boot, em vez de valerem quando a versão subir.
DELETE FROM npc_shop_item WHERE item_index IN (2443, 4141);

UPDATE npc_shop_item SET quantity = 10
WHERE quantity > 10 AND item_index IN (3432, 3433, 3434, 3435, 3436, 3437);

DELETE FROM npc_shop_item
WHERE item_index IN (4018, 4019)
  AND npc_id IN (SELECT id FROM npc_definition WHERE template_name = 'Lucy');

-- A Classe B muda de vaga: some da 2 e entra na 1, que a Classe A deixou livre
-- ao lado dela.
DELETE FROM npc_shop_item
WHERE slot IN (1, 2)
  AND npc_id IN (SELECT id FROM npc_definition WHERE template_name = 'Lucy');

-- As quatro Gemas nas vagas 18-21. A marca de vaga esvaziada pelo painel
-- (0072) precisa sair junto, senão a seed do boot pula a vaga se a linha se
-- perder um dia.
DELETE FROM npc_shop_slot_cleared
WHERE slot IN (1, 18, 19, 20, 21)
  AND npc_id IN (SELECT id FROM npc_definition WHERE template_name = 'Lucy');

INSERT INTO npc_shop_item (npc_id, slot, item_index, quantity)
SELECT d.id, v.slot, v.item, 1
FROM npc_definition d
CROSS JOIN (VALUES (1, 4017), (18, 3386), (19, 3387), (20, 3388), (21, 3389))
    AS v(slot, item)
WHERE d.template_name = 'Lucy'
ON CONFLICT (npc_id, slot) DO UPDATE
SET item_index = EXCLUDED.item_index, quantity = EXCLUDED.quantity,
    eff1 = 0, effv1 = 0, eff2 = 0, effv2 = 0, eff3 = 0, effv3 = 0;

UPDATE npc_config_meta SET version = version + 1 WHERE id = TRUE;
