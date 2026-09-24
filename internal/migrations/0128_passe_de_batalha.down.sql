-- Tira a coluna e a faixa. Repare que isso APAGA quem tem passe: se alguém já
-- comprou, o nível se perde e tem de ser dado de novo à mão.
ALTER TABLE account DROP CONSTRAINT IF EXISTS account_passe_nivel_faixa;
ALTER TABLE account DROP COLUMN IF EXISTS passe_nivel;
