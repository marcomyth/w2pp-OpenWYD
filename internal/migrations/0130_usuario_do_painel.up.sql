-- O USUÁRIO DO PAINEL, separado da conta de jogo.
--
-- POR QUE. Até aqui, entrar no painel era entrar com a CONTA DE JOGO que tivesse
-- cargo (account.role). Uma coisa só respondia duas perguntas diferentes: "esta
-- pessoa pode jogar?" e "esta pessoa pode administrar?". Consequências que a Hanna
-- pediu para desfazer:
--
--   * quem administra é obrigado a ter personagem, e a aparecer no jogo;
--   * tirar o cargo tira as duas coisas de uma vez, sem escolha;
--   * e num ambiente NOVO (o servidor de teste) ninguém consegue abrir o painel,
--     porque não existe conta com cargo — e criar cargo exige o painel. Ovo e
--     galinha, e foi isso que travou o LOTM.
--
-- A SENHA usa o mesmo argon2id do jogo e do site (internal/secret): não há
-- algoritmo novo aqui, e não pode haver.
CREATE TABLE IF NOT EXISTS painel_usuario (
    id         BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    -- Guardado em minúsculas, como o login do painel já faz com o nome digitado.
    login      TEXT    NOT NULL UNIQUE CHECK (login = lower(login) AND login <> ''),
    senha_hash TEXT    NOT NULL,
    -- Os mesmos dois papéis do painel de hoje. Não é o account.role: aquele
    -- continua existindo e continua decidindo quem ENTRA NO JOGO.
    papel      TEXT    NOT NULL CHECK (papel IN ('moderator', 'admin')),
    -- Desativar em vez de apagar: a auditoria aponta para esta linha, e apagar o
    -- usuário arrancaria o nome de todas as ações que ele fez.
    ativo      BOOLEAN NOT NULL DEFAULT TRUE,
    criado_em  TIMESTAMPTZ NOT NULL DEFAULT now(),
    -- Quem criou. Nulo SÓ para o primeiro, que nasce pelo subcomando de linha em
    -- ambiente vazio e portanto não tem criador.
    criado_por BIGINT REFERENCES painel_usuario(id)
);

-- A AUDITORIA PASSA A ACEITAR OS DOIS TIPOS DE ATOR.
--
-- Este é o ponto que fez a separação ser migração e não só uma tabela nova: a
-- coluna actor_account_id era NOT NULL com chave estrangeira para account, então
-- era IMPOSSÍVEL registrar a ação de alguém sem personagem. Um usuário de painel
-- puro não conseguiria fazer nada auditável — ou seja, nada.
--
-- O histórico antigo continua inteiro e legível: as linhas de antes seguem
-- apontando para a conta de jogo que as fez.
ALTER TABLE admin_audit_log ALTER COLUMN actor_account_id DROP NOT NULL;
ALTER TABLE admin_audit_log
    ADD COLUMN IF NOT EXISTS actor_painel_usuario_id BIGINT REFERENCES painel_usuario(id);

-- EXATAMENTE UM DOS DOIS, nunca os dois e nunca nenhum.
--
-- Sem esta trava, um erro de código escreveria uma linha de auditoria sem ator —
-- que é uma ação que aconteceu e que ninguém consegue atribuir a ninguém. Numa
-- tabela cuja única razão de existir é dizer QUEM fez, isso é pior do que a linha
-- não existir.
ALTER TABLE admin_audit_log
    ADD CONSTRAINT admin_audit_log_um_ator CHECK (
        (actor_account_id IS NOT NULL AND actor_painel_usuario_id IS NULL)
        OR (actor_account_id IS NULL AND actor_painel_usuario_id IS NOT NULL)
    );

CREATE INDEX IF NOT EXISTS admin_audit_log_ator_painel_idx
    ON admin_audit_log (actor_painel_usuario_id);
