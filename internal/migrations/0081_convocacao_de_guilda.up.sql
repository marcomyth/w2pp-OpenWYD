-- 0081_convocacao_de_guilda — quem a guilda designou para cada cidade.
--
-- A aba Cidades do Painel de Guild deixa de ser só informação e passa a guardar
-- uma escalação: o líder escolhe, de dentro da guilda, quem vai defender cada
-- cidade. Nada disso existe no legado — lá a convocação é um /convocar que puxa
-- todo mundo de uma vez, sem lista e sem memória.
--
-- A linha é (guilda, cidade, NOME do personagem), e o nome é de propósito:
--
--   O quadro de membros já é lido por nome (guild_member.name), a tela mostra
--   nomes, e uma escalação que apontasse para character_id precisaria de uma
--   junção só para desenhar. O nome é único no servidor — é o que a tabela
--   character já cobra.
--
--   E ele sobrevive ao que interessa: se a pessoa sair da guilda, a linha fica
--   órfã e some na primeira vez que a escalação for regravada. Perder a
--   escalação de quem saiu é o certo; perdê-la porque o personagem trocou de
--   nome seria um problema, e trocar de nome este servidor não permite.
CREATE TABLE guild_city_squad (
    guild_id   INTEGER NOT NULL REFERENCES guild(id) ON DELETE CASCADE,
    zone       SMALLINT NOT NULL CHECK (zone BETWEEN 0 AND 4),
    name       TEXT NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (guild_id, zone, name)
);

-- A leitura é sempre "a escalação desta guilda", das cinco cidades de uma vez:
-- a aba mostra as cinco juntas, e cinco consultas para desenhar uma tela seria
-- pagar quatro a mais do que o necessário.
CREATE INDEX guild_city_squad_guild_idx ON guild_city_squad(guild_id);
