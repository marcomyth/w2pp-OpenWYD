-- A volta apaga o que a subida criou, na ordem do filho para o pai.
--
-- Se alguém JÁ COMPROU um espelho, o `donate_topup_order.pacote_id` aponta para a
-- linha e o REFERENCES ... ON DELETE RESTRICT da 0122 faz esta migração FALHAR. Isso é
-- o certo, e não um defeito a contornar: apagar o pacote de uma compra deixaria o
-- pedido sem dizer o que foi prometido. Para tirar um espelho de venda depois do teste,
-- o caminho é `ativo = FALSE`, e não a volta desta migração.
DELETE FROM donate_pacote_item WHERE pacote_id IN (
    'teste-apoiador-iniciante','teste-apoiador-bronze','teste-apoiador-prata',
    'teste-apoiador-ouro','teste-apoiador-platina','teste-apoiador-diamante',
    'teste-apoiador-mestre','teste-apoiador-lenda','teste-apoiador-supremo');
DELETE FROM donate_pacote WHERE id IN (
    'teste-apoiador-iniciante','teste-apoiador-bronze','teste-apoiador-prata',
    'teste-apoiador-ouro','teste-apoiador-platina','teste-apoiador-diamante',
    'teste-apoiador-mestre','teste-apoiador-lenda','teste-apoiador-supremo');
