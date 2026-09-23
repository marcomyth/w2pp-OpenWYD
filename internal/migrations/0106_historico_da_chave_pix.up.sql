-- 0106_historico_da_chave_pix — o rastro de quem trocou a chave de recebimento.
--
-- Trocar a chave Pix é a ação mais sensível que um jogador faz neste sistema:
-- ela REDIRECIONA DINHEIRO REAL. Sem rastro, o caso da conta invadida não tem
-- resposta — o jogador vende, o dinheiro cai na conta de outro, ele reclama, e
-- quem for olhar não tem o que olhar.
--
-- TABELA PRÓPRIA, E NÃO O admin_audit_log, e a razão está escrita na 0022: aquela
-- tabela é para "every administrative write", e carrega `actor_role` justamente
-- para que rebaixar alguém não reescreva como as ações passadas dele se leem.
-- Isso é semântica de STAFF. Aqui o autor é o próprio dono da conta, o papel dele
-- não diz nada, e misturar os dois faria duas coisas ruins de uma vez: a consulta
-- "o que a moderação fez" passaria a ter de filtrar ruído de jogador, e qualquer
-- decisão futura sobre o log de moderação — retenção, limpeza, mudança de forma —
-- alcançaria prova de dinheiro sem ninguém perceber.
--
-- AS CHAVES FICAM MASCARADAS AQUI TAMBÉM, e isso não é descuido de completude: a
-- máscara já responde a pergunta da disputa, que é "mudou de ...1234 para
-- ...9876". Guardar as chaves inteiras criaria um segundo lugar com dado pessoal
-- de pagamento, e um log é exatamente o tipo de lugar que ninguém lembra de
-- proteger porque "é só log".
CREATE TABLE rmt_recebedor_historico (
    id                     BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    -- RESTRICT, como nas outras duas tabelas de dinheiro (0105): isto é prova, e
    -- prova sobrevive à conta. Na prática não aperta nada para quem vende — quem
    -- tem anúncio já não pode ser apagado —, e para quem só cadastrou a chave o
    -- registro de ter apontado um destino de dinheiro vale ser mantido.
    account_id             BIGINT   NOT NULL REFERENCES account(id) ON DELETE RESTRICT,
    -- Nulos no PRIMEIRO cadastro, que não tem "de onde". Nulo aqui significa
    -- "não havia chave antes", e não "não sei qual era".
    tipo_antigo            SMALLINT,
    chave_antiga_mascarada TEXT,
    tipo_novo              SMALLINT NOT NULL,
    chave_nova_mascarada   TEXT     NOT NULL,
    criado_em              TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- A pergunta é sempre "o que aconteceu com a chave DESTA conta, do mais novo
-- para o mais velho" — é assim que uma disputa se investiga.
CREATE INDEX rmt_recebedor_historico_conta_idx
    ON rmt_recebedor_historico (account_id, criado_em DESC);
