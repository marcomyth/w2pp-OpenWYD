-- 0072_npc_shop_slot_cleared — vaga de loja esvaziada pelo painel continua vazia.
--
-- O dbServer ressemeia a loja de todo mercador a partir do template em cada boot
-- (store.SeedNPCDefinitions: INSERT ... ON CONFLICT (npc_id, slot) DO NOTHING).
-- Isso preenche qualquer vaga livre, inclusive a que um moderador acabou de
-- esvaziar no painel: o item sumia na hora e voltava no deploy seguinte.
--
-- Não dá para simplesmente parar de semear NPC existente: migrações como a 0070
-- apagam vagas da Aki justamente para o template recolocar os anéis no boot.
-- Então a seed continua preenchendo vaga livre, e só pula a vaga registrada aqui.
--
-- Quem grava: store.SetNPCShop, para cada vaga que tinha item e deixou de ter.
-- Quem apaga: o mesmo SetNPCShop quando a vaga volta a ter item, e o CASCADE
-- quando a definição some. Uma migração que queira o item do template de volta
-- numa vaga marcada precisa apagar a marca também.

CREATE TABLE npc_shop_slot_cleared (
    npc_id     BIGINT NOT NULL REFERENCES npc_definition(id) ON DELETE CASCADE,
    slot       SMALLINT NOT NULL CHECK (slot BETWEEN 0 AND 26),
    cleared_by BIGINT REFERENCES account(id) ON DELETE SET NULL,
    cleared_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (npc_id, slot)
);
