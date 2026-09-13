-- 0062_newbie_kit — a trava do /novato: o kit de entrada sai UMA vez por conta.
--
-- Por conta, não por personagem, e é isso que exige tabela própria. O estado do
-- personagem já viaja inteiro no save, mas uma marca ali seria contornada em
-- dez segundos: apaga o personagem, cria outro, pede o kit de novo. A conta é o
-- único nível em que "uma vez" quer dizer uma vez.
--
-- A linha é a trava, não uma coluna booleana: existir é ter recebido. Assim a
-- concessão é um INSERT ... ON CONFLICT DO NOTHING, que o Postgres resolve
-- atomicamente mesmo com dois personagens da mesma conta pedindo ao mesmo tempo
-- em canais diferentes — um insere, o outro não insere, e quem não inseriu não
-- entrega item nenhum. Um SELECT seguido de UPDATE teria a janela entre os dois
-- que é exatamente onde nasce o kit duplicado.
--
-- character_name é informativo e fica solto de propósito (sem FK), como no
-- extrato de 0060: serve para o suporte saber quem pediu, e nome de personagem
-- não é único neste servidor — Arch e Celestial herdam o nome do Mortal.

CREATE TABLE IF NOT EXISTS newbie_kit_claim (
    account_id     BIGINT PRIMARY KEY REFERENCES account(id) ON DELETE CASCADE,
    character_name TEXT NOT NULL DEFAULT '',
    claimed_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);
