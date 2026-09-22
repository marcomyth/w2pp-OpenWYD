-- 0093_molar_gargula — a marca de que este personagem já usou o Molar de Gárgula.
--
-- O Molar (item 4122, EF_VOLATILE 194) passa a subir para +7 o refino das cinco
-- peças que o personagem está vestindo, uma única vez por personagem. Sem
-- guardar isto, bastaria trocar de set e usar outro molar — e ele cai a 35,6%
-- de uma única Gárgula Inf do 2º andar da Dungeon.
--
-- SMALLINT como os outros portões de quest (0058 newbie_quest, 0013
-- terra_mistica): o valor cabe num byte e o proto o carrega como int32.
ALTER TABLE character ADD COLUMN IF NOT EXISTS molar_gargula SMALLINT NOT NULL DEFAULT 0;
