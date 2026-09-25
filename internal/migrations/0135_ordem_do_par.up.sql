-- A ORDEM DAS GRAVAÇÕES DO PAR.
--
-- O personagem e a carga vão ao banco juntos, mas as gravações saem em goroutines
-- independentes: o w.Go abre uma por chamada. Duas gravações do mesmo par podem
-- chegar FORA DE ORDEM, e a velha sobrescreve a nova.
--
-- Isso não duplica — cada gravação é um instantâneo inteiro —, mas APAGA. O caso
-- que dói: um par novo marca uma entrega como feita e grava o item na carga; um par
-- velho chega depois e regrava a carga SEM o item, com a linha da caixa postal já
-- em 'delivered'. É item comprado com dinheiro que some, e não há como reentregar.
--
-- A ordem é do MUNDO, que é quem tira os instantâneos: ele numera cada par, e o
-- banco recusa gravar um número menor que o último gravado. A conferência mora na
-- MESMA transação da gravação, senão seria só mais uma corrida.
--
-- E A ÉPOCA VEM DO BANCO, NÃO DO RELÓGIO. Um contador que zera quando o processo
-- sobe ficaria abaixo do número que o banco guardou da execução anterior, e daí
-- NENHUMA gravação passaria mais — perda total, calada. Cada boot do tmServer pega
-- um número novo desta sequência; a comparação é (época, número), nessa ordem.
-- Relógio não serve para isto: um acerto de hora para trás produziria o mesmo
-- travamento silencioso.
CREATE SEQUENCE par_epoca_seq;

ALTER TABLE account
  ADD COLUMN par_epoca BIGINT NOT NULL DEFAULT 0,
  ADD COLUMN par_seq   BIGINT NOT NULL DEFAULT 0;

COMMENT ON COLUMN account.par_epoca IS
  'Epoca (boot do tmServer) do ultimo par personagem+carga gravado. Ver par_seq.';
COMMENT ON COLUMN account.par_seq IS
  'Numero do ultimo par gravado dentro daquela epoca. O banco recusa par com (epoca, numero) menor: e a guarda de ordem entre gravacoes assincronas.';
