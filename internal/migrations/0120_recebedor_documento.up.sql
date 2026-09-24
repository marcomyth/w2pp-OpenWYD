-- 0120_recebedor_documento — o CPF de quem RECEBE.
--
-- A ponte exige documento para fazer o repasse. Sem ele o dinheiro do vendedor
-- fica na conta de quem administra o servidor, sem caminho de saída — que é
-- exatamente o estado em que o mercado está enquanto isto não existe.
--
-- COLUNA E NÃO TABELA, ao contrário do que a 0105 fez com a chave: o documento é
-- do MESMO dono, tem a MESMA vida e some junto. Uma tabela a mais aqui só
-- acrescentaria um JOIN a toda leitura, sem separar nada que precise ser separado.
--
-- NULA POR ENQUANTO, e é o que tem de ser: todo vendedor cadastrado antes desta
-- migração não tem documento, e um valor de mentira para preencher seria pior do
-- que o nulo — o repasse tentaria e seria recusado, e ninguém saberia por quê. O
-- nulo diz "esta pessoa ainda precisa preencher", que é a verdade.
--
-- E o CASCADE acompanha a tabela: dado pessoal some com a conta. O que NÃO pode
-- sumir é o registro do dinheiro, e ele mora em rmt_cobranca, com RESTRICT.
ALTER TABLE rmt_recebedor
    ADD COLUMN IF NOT EXISTS documento TEXT;

-- Guardado em DÍGITOS, sem ponto e sem traço, e o CHECK é o que garante.
--
-- Normalizar na escrita e não na leitura: "111.444.777-35" e "11144477735" são a
-- mesma pessoa, e guardar os dois formatos faria uma busca por documento achar
-- metade das linhas. A formatação é enfeite de tela e se refaz a qualquer momento;
-- o que se guarda é o número.
ALTER TABLE rmt_recebedor
    ADD CONSTRAINT rmt_recebedor_documento_digitos
    CHECK (documento IS NULL OR documento ~ '^[0-9]{11}$');
