-- A TAXA DA PROCESSADORA, para o vendedor receber o LÍQUIDO.
--
-- Até aqui o repasse nascia com o valor CHEIO da cobrança, e a casa bancava a taxa
-- em silêncio. Não era uma decisão: era a ausência do dado. A ponte não dizia quanto
-- a processadora tinha retido, então não havia de onde subtrair.

-- taxa_centavos fica na COBRANÇA, e não no repasse, porque é um fato do dinheiro que
-- ENTROU: a processadora reteve daquela entrada, e a entrada existe mesmo quando não
-- há repasse nenhum (venda sem item, valor divergente, devolução). Guardar na
-- cobrança mantém o fato junto do evento que o produziu.
--
-- NULO NÃO É ZERO, e é a regra inteira desta coluna. Nulo quer dizer "ninguém sabe a
-- taxa": a fonte não trouxe, veio vazia, ou a ponte ainda não foi atualizada. Zero é
-- uma AFIRMAÇÃO, "a taxa foi zero". Por isso a coluna admite nulo em vez de ter
-- DEFAULT 0: um default aqui transformaria toda ignorância em afirmação, e o repasse
-- sairia com o valor cheio parecendo certo.
ALTER TABLE rmt_cobranca ADD COLUMN taxa_centavos bigint;

COMMENT ON COLUMN rmt_cobranca.taxa_centavos IS
  'Quanto a processadora reteve do que entrou, em centavos. NULO = nao se sabe, que NAO e zero.';

-- bruto_centavos no repasse guarda de quanto se partiu.
--
-- valor_centavos continua sendo O QUE SE PAGA AO VENDEDOR, agora líquido. Sem o bruto
-- ao lado, a conta deixaria de fechar: ninguém conseguiria responder "por que recebi
-- menos do que o anúncio dizia" sem ir procurar a cobrança e adivinhar qual taxa
-- valia naquele dia. Numa disputa sobre dinheiro, a resposta tem de estar na linha.
--
-- Nulo nas linhas ANTIGAS, e de propósito: elas nasceram antes desta coluna existir e
-- foram pagas pelo valor cheio. Preencher com valor_centavos agora afirmaria que o
-- bruto era igual ao líquido, o que é verdade por acidente e mentira como registro.
ALTER TABLE rmt_repasse ADD COLUMN bruto_centavos bigint;

COMMENT ON COLUMN rmt_repasse.bruto_centavos IS
  'De quanto se partiu, antes da taxa. NULO nas linhas anteriores a 0131.';

-- O ESTADO 6 EXISTE PARA O REPASSE NÃO SAIR CHEIO quando a taxa é desconhecida.
--
-- Sem ele, as saídas seriam duas e as duas ruins: pagar o bruto (a casa perde a taxa
-- em silêncio, e ninguém descobre porque um repasse cheio parece certo), ou não criar
-- o repasse (o vendedor fica invisível, que é exatamente o bug que a 0124 consertou).
--
-- E NÃO se reusou o estado 5, incerto, embora "segurar para alguém olhar" pareça a
-- mesma coisa: incerto quer dizer "a chamada saiu e pode ter pago", e a staff tem uma
-- ação que resolve incerto como PAGO. Um repasse segurado por taxa desconhecida
-- nunca saiu; cair naquela ação o daria como pago sem nenhum dinheiro ter andado.
--
-- Não há CHECK de status nesta tabela, então o estado 6 não exige alterar restrição
-- nenhuma. Quem o exclui da fila de pagar é o `status = pendente` da consulta, que
-- lista um estado por nome e não "tudo menos".
COMMENT ON COLUMN rmt_repasse.status IS
  '1 pendente, 2 enviado, 3 pago, 4 recusado, 5 incerto (pode ter pago), 6 sem taxa conhecida (segurado).';

-- A TERCEIRA FILA DA STAFF, no mesmo molde das duas da 0124: o que está segurado por
-- taxa desconhecida e ninguém resolveu. Sem ela, o estado 6 seria um lugar onde
-- dinheiro para e não aparece em consulta nenhuma — o mesmo defeito que a 0124 nomeou
-- ao criar as filas de recusado e de incerto.
CREATE INDEX rmt_repasse_sem_taxa ON rmt_repasse (criado_em)
    WHERE status = 6 AND resolvido_em IS NULL;
