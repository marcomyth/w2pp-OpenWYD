-- 0080_buffs_de_guilda — até quando cada buff de guilda vale.
--
-- Os buffs nasceram (21/09/2026) só na memória do tmServer, e enquanto a ideia
-- era "uma hora de buff" isso passava: um restart custaria alguns minutos a
-- quem tivesse acabado de gastar o item. Com o item valendo 15 e 30 DIAS não
-- passa mais. O buff é comprado com cash, e um restart de manutenção no meio de
-- um mês de buff apagaria o que alguém pagou. Por isso ele vira linha de banco.
--
-- Uma linha por (guilda, buff): os quatro buffs têm relógios independentes, e
-- foi assim desde o começo — o item de hoje acende os quatro de uma vez, mas
-- nada no desenho obriga que continue assim, e uma linha só com quatro colunas
-- de data travaria isso.
CREATE TABLE guild_buff (
    guild_id   INTEGER NOT NULL REFERENCES guild(id) ON DELETE CASCADE,
    buff_type  SMALLINT NOT NULL CHECK (buff_type BETWEEN 1 AND 4),
    expires_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (guild_id, buff_type)
);

-- A leitura do boot pega só o que ainda vale, e é este índice que a serve.
-- Sem ele, um servidor com muitas guildas varre a tabela inteira para achar as
-- poucas linhas vivas.
CREATE INDEX guild_buff_expires_idx ON guild_buff(expires_at);

-- A guilda some, os buffs dela somem junto: é o ON DELETE CASCADE acima, e vale
-- dizer por quê. Um buff é da guilda, não dos membros — quem entra no meio pega
-- o resto do tempo, quem sai perde. Guardar o buff de uma guilda dissolvida
-- seria guardar uma vantagem sem dono.
