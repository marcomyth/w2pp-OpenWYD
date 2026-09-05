-- 0025_trade_log — what changed hands in a player-to-player trade.
--
-- Nothing recorded trades before this. tmserver/internal/handler/trade.go swaps
-- the items and the gold and writes nothing anywhere, so a moderator answering
-- "he scammed me" had the players' word and nothing else.
--
-- Deliberately NO foreign key to character or account. DeleteCharacter is
-- player-invocable and physically deletes the row (dbserver SetPin path), so a
-- CASCADE would let the suspect delete the evidence, and even RESTRICT would let
-- them block their own character deletion into a support ticket. Names and ids
-- are copied in as plain values, the way donate_shop_audit already does it.
--
-- What this cannot see, and the screen says so: an item handed over by dropping
-- it on the ground for the other player to pick up. getItem gives a ground item
-- to anyone within three tiles with no owner check and no log, so a determined
-- scammer has a path that leaves nothing here.
--
-- One more honest limit. There is no periodic character save: a character's
-- items reach Postgres only when they log out. This row is durable the moment
-- the trade happens, so for a while it describes a state the database does not
-- yet show. It is a record of what the server did, not proof of what the
-- database holds.

CREATE TABLE trade_log (
    id          BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    ocorrido_em TIMESTAMPTZ NOT NULL DEFAULT now(),

    -- The two sides. char_* is what a moderator searches by; conta_* links to
    -- the account page. Both copied, neither referenced.
    char_a  TEXT NOT NULL,
    char_b  TEXT NOT NULL,
    conta_a BIGINT,
    conta_b BIGINT,

    -- Gold each side handed over, captured before the handler zeroes the trade
    -- state. INTEGER because the in-world coin field is int32.
    ouro_a INTEGER NOT NULL DEFAULT 0,
    ouro_b INTEGER NOT NULL DEFAULT 0,

    -- Items each side handed over: [{"index":N,"eff":[[e,v],[e,v],[e,v]]}, …].
    -- JSONB rather than a child table because nothing queries inside them — the
    -- screen shows a trade whole, and a child table would only add a join.
    itens_a JSONB NOT NULL DEFAULT '[]'::jsonb,
    itens_b JSONB NOT NULL DEFAULT '[]'::jsonb
);

-- Searching by either participant. Two indexes rather than one on a combined
-- column so a name matches whichever side it sat on, newest first, which is the
-- order every complaint arrives in.
CREATE INDEX trade_log_char_a_idx ON trade_log (char_a, ocorrido_em DESC);
CREATE INDEX trade_log_char_b_idx ON trade_log (char_b, ocorrido_em DESC);
CREATE INDEX trade_log_ocorrido_idx ON trade_log (ocorrido_em DESC);
