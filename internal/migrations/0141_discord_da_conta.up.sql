-- O DISCORD VINCULADO À CONTA.
--
-- O site faz o OAuth do Discord e guarda aqui o snowflake da pessoa. Quem lê é o
-- bot, para dar cargo a quem joga, e a staff, para responder "de quem é este
-- Discord".
--
-- UM DISCORD PARA UMA CONTA, e são DUAS travas, não uma:
--
--   1. o índice único PARCIAL, onde o campo não é nulo;
--   2. o CHECK proibindo a string vazia.
--
-- As duas porque em Postgres NULL não conflita com NULL: sem o CHECK, bastaria
-- alguém gravar '' em duas contas para o índice deixar de valer, e o vínculo único
-- viraria uma promessa que o banco não cumpre. "Sem Discord" é NULO, e só.
--
-- O SNOWFLAKE É TEXTO, e não bigint. Ele cabe num int64 hoje, mas é um
-- identificador e não um número: ninguém soma nem compara por ordem, o Discord o
-- documenta como string no JSON, e guardá-lo como número convida alguém a formatá-lo
-- com separador de milhar algum dia.
ALTER TABLE account ADD COLUMN discord_id TEXT;

ALTER TABLE account
  ADD CONSTRAINT account_discord_id_nao_vazio
  CHECK (discord_id IS NULL OR discord_id <> '');

CREATE UNIQUE INDEX account_discord_id_unico
  ON account(discord_id) WHERE discord_id IS NOT NULL;

COMMENT ON COLUMN account.discord_id IS
  'Snowflake do Discord vinculado a esta conta, ou NULO. Unico entre contas (indice parcial + CHECK de nao vazio). Dado pessoal leve: nao vai para log.';
