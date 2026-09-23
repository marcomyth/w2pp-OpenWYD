-- 0079_painel_de_guilda — o que o Painel de Guilda mostra e o banco ainda não
-- guardava.
--
-- O 0012_guild_system já trouxe nome, id, clã, fama, os membros com cargo 0..9 e
-- as relações de aliança e guerra. Tudo isso o painel só transporta. O que falta
-- são três coisas que nunca existiram nem no legado: o recado que o líder deixa
-- para a guilda, o teto de membros, e a linha de status que cada membro escreve
-- para si.
--
-- Nenhuma delas tem equivalente em Guilds.txt nem em GuildInfo — o legado não
-- tinha janela de guilda, só comandos de chat -, então não há formato original a
-- imitar e as três são decisão nossa.

-- O recado da guilda. Texto livre, limitado no banco e não só na tela, porque
-- quem escreve é o cliente e o cliente mente.
--
-- notice_at é quando o recado foi escrito, não quando a linha mudou: o painel
-- mostra "Atualizado: ..." e esse carimbo tem de morrer junto com o texto. Por
-- isso ele não é um updated_at automático — fica nulo enquanto não houver recado.
ALTER TABLE guild ADD COLUMN IF NOT EXISTS notice TEXT NOT NULL DEFAULT '';
ALTER TABLE guild ADD COLUMN IF NOT EXISTS notice_at TIMESTAMPTZ;
ALTER TABLE guild ADD COLUMN IF NOT EXISTS notice_by TEXT NOT NULL DEFAULT '';

ALTER TABLE guild DROP CONSTRAINT IF EXISTS guild_notice_len_check;
ALTER TABLE guild ADD CONSTRAINT guild_notice_len_check
    CHECK (char_length(notice) <= 240);

-- O teto de membros, por guilda e não por constante, para poder ser prêmio de
-- alguma coisa depois sem mexer em código.
--
-- 250 é o padrão de propósito: é maior que qualquer guilda que este servidor
-- tem hoje, então ligar a migração não tranca ninguém para fora da própria
-- guilda. O piso do CHECK é 1 porque uma guilda sem o líder não existe.
ALTER TABLE guild ADD COLUMN IF NOT EXISTS member_cap SMALLINT NOT NULL DEFAULT 250;

ALTER TABLE guild DROP CONSTRAINT IF EXISTS guild_member_cap_check;
ALTER TABLE guild ADD CONSTRAINT guild_member_cap_check
    CHECK (member_cap BETWEEN 1 AND 1000);

-- A linha de status do membro, a coluna que na tela fica entre o cargo e o
-- online. É do MEMBRO, não da guilda: quem escreve é ele, e ela o segue se ele
-- trocar de guilda? Não — guild_member some quando ele sai (ON DELETE CASCADE
-- do 0012), e é isso que queremos: o status é dito para aquela guilda.
ALTER TABLE guild_member ADD COLUMN IF NOT EXISTS status TEXT NOT NULL DEFAULT '';

ALTER TABLE guild_member DROP CONSTRAINT IF EXISTS guild_member_status_len_check;
ALTER TABLE guild_member ADD CONSTRAINT guild_member_status_len_check
    CHECK (char_length(status) <= 32);

-- Quando o membro esteve online pela última vez, para a coluna "Online" poder
-- dizer "Hoje" em vez de só "não".
--
-- Fica em guild_member em vez de sair de character porque o painel lista a
-- guilda inteira de uma consulta só, e character já é a tabela mais disputada
-- do banco: uma juncao por membro em cada abertura de painel é o tipo de peso
-- que aparece na guerra, exatamente quando ninguém pode pagar por ele.
ALTER TABLE guild_member ADD COLUMN IF NOT EXISTS last_seen TIMESTAMPTZ;
