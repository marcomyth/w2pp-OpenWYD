-- 0111_anuncio_barraca_caiu — a coluna que separa "está à venda" de "ainda tem
-- dinheiro em jogo".
--
-- O PROBLEMA QUE ELA RESOLVE, com nomes:
--
--   O vendedor monta a barraca, alguém abre o QR, e o vendedor fecha a barraca
--   antes de o Pix cair. Agora o anúncio precisa ser duas coisas ao mesmo tempo:
--   FORA da vitrine, porque a barraca não existe mais, e VIVO o bastante para o
--   pagamento atrasado ainda encontrar o que entregar.
--
-- Trocar o status não serve para isso. O `temItemParaEntregar` (cobranca_rmt.go)
-- exige ATIVO justamente para recusar entrega de anúncio morto, e a regra da
-- Hanna é que a confirmação atrasada AINDA ENTREGA. Um status novo faria a
-- confirmação atrasada cair em PAGA_SEM_ITEM — dívida com uma pessoa que pagou
-- direito.
--
-- Por isso é coluna própria e não estado: ela é ORTOGONAL ao status. O status
-- responde "esta venda ainda pode acontecer?"; esta coluna responde "a barraca
-- que a mostrava ainda está de pé?". As duas perguntas mudam por motivos
-- diferentes, em momentos diferentes.
--
-- E é ela que diz quando o CADEADO pode sair. A marca do escrow (0104) segura o
-- item do vendedor; ela só solta quando a barraca caiu E não há cobrança aberta.
-- Sem esta coluna não há como distinguir um anúncio ativo numa barraca de pé —
-- cujo item TEM de continuar preso — de um anúncio ativo esperando um Pix cuja
-- barraca já desceu.
ALTER TABLE rmt_anuncio
    ADD COLUMN barraca_caiu BOOLEAN NOT NULL DEFAULT FALSE;

-- A varredura da faxina: anúncio ativo, sem barraca, cujo item pode estar preso
-- à toa. É por aqui que o login do vendedor descobre o que soltar.
CREATE INDEX rmt_anuncio_sem_barraca
    ON rmt_anuncio (vendedor_conta) WHERE status = 1 AND barraca_caiu;
