-- O HISTÓRICO DA CHAVE PIX PASSA A ACEITAR O APAGAR.
--
-- A tabela nasceu para registrar TROCA: tinha um lado antigo e um lado novo, e os dois
-- do lado novo eram NOT NULL porque toda troca tem um destino.
--
-- Apagar é um evento de outra espécie: ele tem lado antigo e NÃO TEM lado novo. Com as
-- colunas obrigatórias, a única forma de gravar o apagar seria inventar um valor para o
-- "novo" — um tipo qualquer e uma máscara vazia —, e aí a linha passaria a MENTIR: quem
-- lesse o histórico veria uma troca para uma chave que nunca existiu.
--
-- Nulo aqui quer dizer exatamente o que aconteceu: não há chave nova porque a chave foi
-- removida. É a mesma disciplina que a gente usa no resto deste sistema — nulo é
-- ignorância ou ausência, e zero é afirmação.
--
-- As linhas ANTIGAS não mudam: elas são trocas, e trocas continuam tendo os dois lados.
ALTER TABLE rmt_recebedor_historico ALTER COLUMN tipo_novo DROP NOT NULL;
ALTER TABLE rmt_recebedor_historico ALTER COLUMN chave_nova_mascarada DROP NOT NULL;

COMMENT ON COLUMN rmt_recebedor_historico.tipo_novo IS
  'Tipo da chave nova. NULO quando o evento foi APAGAR: nao ha chave nova.';
COMMENT ON COLUMN rmt_recebedor_historico.chave_nova_mascarada IS
  'Mascara da chave nova. NULO quando o evento foi APAGAR.';
