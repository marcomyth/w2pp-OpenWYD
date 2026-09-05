-- 0023_item_stats — moderator-editable item base stats, the item-side sibling of
-- 0013_mob_template_stats.
--
-- What this covers: the static per-catalog-entry numbers an item grants when
-- equipped (STRUCT_ITEMLIST.stEffect, the EF_* pairs of Release/Common/
-- ItemList.csv) and the requirement to equip it (the "L.S.I.D.C" column). It does
-- NOT cover a single item instance a player owns: refines and per-item effects
-- live on STRUCT_ITEM and are untouched by this table.
--
-- Price is deliberately absent. It already has item_price (0005_npc_editing) and
-- hot-reloads through the NPC config poll, because a price is only read at the
-- moment of a shop transaction. These numbers are different: they feed the equip
-- score model, which is recomputed per character on equip/level/etc. Swapping
-- them under a running server would leave two players wearing the same item with
-- different stats until each happened to recompute. So, exactly like
-- mob_template_stat, the tmServer reads this ONCE at boot and a moderator edit
-- needs a restart to take effect.
--
-- Absence of a row means ItemList.csv is used unchanged: no behavior change for
-- unedited items.
--
-- The override REPLACES an item's whole effect list rather than merging into it.
-- Merging would need a value that means "not overridden", and 0 cannot be that
-- value: 0 is a legitimate setting for every column here. Replacing keeps the
-- rule sayable in one sentence — the row is the item's effects — at the cost of
-- the editor having to seed the form from the CSV, which the webServer does
-- because it is the only service that mounts the content tree.

CREATE TABLE item_stat (
    item_index  INTEGER PRIMARY KEY,

    -- Requirement to equip: ItemList.csv column 3, "Lvl.Str.Int.Dex.Con".
    req_level   SMALLINT NOT NULL DEFAULT 0,
    req_str     SMALLINT NOT NULL DEFAULT 0,
    req_int     SMALLINT NOT NULL DEFAULT 0,
    req_dex     SMALLINT NOT NULL DEFAULT 0,
    req_con     SMALLINT NOT NULL DEFAULT 0,

    -- Base effects. One column per EF_* token the score model understands
    -- (tmserver/internal/content/catalog.go efName). Column names drop the EF_
    -- prefix and lowercase the rest; "int" is a reserved word, hence intel.
    --
    -- SMALLINT throughout because STRUCT_EFFECT carries an int16 value, and the
    -- loader narrows to int16 regardless — a wider column would only let the
    -- panel store a number the game then truncates.

    -- Combat
    damage      SMALLINT NOT NULL DEFAULT 0,
    damageadd   SMALLINT NOT NULL DEFAULT 0,
    ac          SMALLINT NOT NULL DEFAULT 0,
    acadd       SMALLINT NOT NULL DEFAULT 0,
    magic       SMALLINT NOT NULL DEFAULT 0,
    magicadd    SMALLINT NOT NULL DEFAULT 0,
    critical    SMALLINT NOT NULL DEFAULT 0,
    critical2   SMALLINT NOT NULL DEFAULT 0,
    runspeed    SMALLINT NOT NULL DEFAULT 0,

    -- Attributes
    str         SMALLINT NOT NULL DEFAULT 0,
    intel       SMALLINT NOT NULL DEFAULT 0,
    dex         SMALLINT NOT NULL DEFAULT 0,
    con         SMALLINT NOT NULL DEFAULT 0,

    -- Life
    hp          SMALLINT NOT NULL DEFAULT 0,
    hpadd       SMALLINT NOT NULL DEFAULT 0,
    hpadd2      SMALLINT NOT NULL DEFAULT 0,
    mp          SMALLINT NOT NULL DEFAULT 0,
    mpadd       SMALLINT NOT NULL DEFAULT 0,
    mpadd2      SMALLINT NOT NULL DEFAULT 0,

    -- Resistances
    resist1     SMALLINT NOT NULL DEFAULT 0,
    resist2     SMALLINT NOT NULL DEFAULT 0,
    resist3     SMALLINT NOT NULL DEFAULT 0,
    resist4     SMALLINT NOT NULL DEFAULT 0,
    resistall   SMALLINT NOT NULL DEFAULT 0,

    -- Masteries
    special1    SMALLINT NOT NULL DEFAULT 0,
    special2    SMALLINT NOT NULL DEFAULT 0,
    special3    SMALLINT NOT NULL DEFAULT 0,
    special4    SMALLINT NOT NULL DEFAULT 0,
    specialall  SMALLINT NOT NULL DEFAULT 0,

    -- Identity and mechanics. Not "attributes" a balancer reaches for, but they
    -- are part of the effect list, and since the override replaces that list
    -- whole they have to be carried or a weapon would lose its type and a joia
    -- its refine the first time somebody edited its damage.
    itemlevel   SMALLINT NOT NULL DEFAULT 0,
    itemtype    SMALLINT NOT NULL DEFAULT 0,
    mobtype     SMALLINT NOT NULL DEFAULT 0,
    wtype       SMALLINT NOT NULL DEFAULT 0,
    pos         SMALLINT NOT NULL DEFAULT 0,
    sanc        SMALLINT NOT NULL DEFAULT 0,
    nosanc      SMALLINT NOT NULL DEFAULT 0,
    incubate    SMALLINT NOT NULL DEFAULT 0,
    incudelay   SMALLINT NOT NULL DEFAULT 0,

    updated_by  BIGINT REFERENCES account(id) ON DELETE SET NULL,
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- The panel's pending-restart count asks "what changed since the server booted?"
-- across every boot-bound override table, so this column is scanned on its own.
CREATE INDEX item_stat_updated_at_idx ON item_stat (updated_at);
