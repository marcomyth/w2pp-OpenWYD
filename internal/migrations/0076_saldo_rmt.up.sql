-- 0076_saldo_rmt — a carteira de RMT da conta, para a Loja do Servidor.
--
-- O Cash já existia: é account.donate_balance, a carteira que a recarga por PIX
-- credita (0001_init, 0008_donate_shop). O RMT é a segunda moeda que o vendedor
-- pode escolher na vitrine e não tinha onde morar, então ganha a própria coluna,
-- com a mesma forma da outra: inteiro, nunca nulo, começando em zero.
--
-- Ouro não está aqui de propósito: ele é do personagem, não da conta, e quem o
-- move é o tmServer dentro do próprio laço.
ALTER TABLE account ADD COLUMN rmt_balance INTEGER NOT NULL DEFAULT 0;

-- Nenhuma carteira fica negativa. A transferência já confere antes de debitar;
-- isto é a rede embaixo, para o caso de alguém escrever direto na tabela.
ALTER TABLE account ADD CONSTRAINT account_rmt_balance_nao_negativo CHECK (rmt_balance >= 0);
