-- 0077_saldo_rmt — a carteira de RMT da conta, para a Loja do Servidor.
--
-- O Cash já existia: é account.donate_balance, a carteira que a recarga por PIX
-- credita (0001_init, 0008_donate_shop). O RMT é a segunda moeda que o vendedor
-- pode escolher na vitrine e não tinha onde morar, então ganha a própria coluna,
-- com a mesma forma da outra: inteiro, nunca nulo, começando em zero.
--
-- Ouro não está aqui de propósito: ele é do personagem, não da conta, e quem o
-- move é o tmServer dentro do próprio laço.
--
-- Idempotente de propósito: esta migração nasceu numerada 0076 e já rodou assim
-- no banco de teste, antes de a 0076_combat_rule_garnet chegar do main. Com a
-- renumeração para 0077 ela volta a ser "nova" naquele banco, onde a coluna já
-- existe — então aplicar duas vezes não pode quebrar.
ALTER TABLE account ADD COLUMN IF NOT EXISTS rmt_balance INTEGER NOT NULL DEFAULT 0;

-- Nenhuma carteira fica negativa. A transferência já confere antes de debitar;
-- isto é a rede embaixo, para o caso de alguém escrever direto na tabela.
DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM pg_constraint WHERE conname = 'account_rmt_balance_nao_negativo'
  ) THEN
    ALTER TABLE account
      ADD CONSTRAINT account_rmt_balance_nao_negativo CHECK (rmt_balance >= 0);
  END IF;
END $$;
