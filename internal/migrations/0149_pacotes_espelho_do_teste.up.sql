-- 0149_pacotes_espelho_do_teste — nove cópias de R$ 1,00, só staff, para testar a
-- ENTREGA dos brindes sem gastar R$ 3.500.
--
-- POR QUE CÓPIA E NÃO DESCONTO NOS NOVE DE VERDADE. Baixar o preço dos pacotes reais
-- para R$ 1,00 abriria o Supremo a R$ 1,00 para QUALQUER conta enquanto o teste
-- durasse, e esquecer de voltar venderia barato de verdade, sem nada quebrar para
-- avisar. A cópia usa a trava que já existe — `so_staff`, a mesma do `teste-real` —,
-- então ela nasce fechada e o pacote real nunca muda de preço.
--
-- OS BRINDES SÃO COPIADOS, E NÃO DIGITADOS. O teste é justamente da entrega: um
-- espelho com a lista redigitada testaria a lista que eu escrevi, e não a que o
-- jogador recebe. O INSERT ... SELECT abaixo lê os `donate_pacote_item` do pacote real
-- e os repete, com os três pares de efeito e a ordem — baús de sorteio incluídos, que
-- são itens como os outros nesta tabela.
--
-- OS CRÉDITOS SÃO OS DO PACOTE REAL, de propósito, e é o `credits` do espelho que
-- também vem por SELECT. Espelho que entrega menos Rcoins não prova a entrega do
-- pacote; e a conferência de preço do `ConferirPacote` compara o pedido contra ESTA
-- linha, então os R$ 1,00 aqui são o preço que o site tem de mandar.
--
-- QUEM PASSA NA TRAVA: o servidor lê `account.role` e aceita 'admin' ou 'moderator'
-- (store.ContaEhStaff, internal/store/pacote_doacao.go:207). Conta sem esses valores
-- recebe a recusa `ErrPacoteSoStaff` mesmo chamando a RPC direto — a tela é uma das
-- portas, não a trava.
--
-- SAIR DO TESTE é uma migração que põe `ativo = FALSE` nestas nove linhas. NÃO apagar,
-- pela regra escrita na 0122: pedido antigo que ainda não confirmou tem de continuar
-- achando a linha dele, senão a pessoa paga e o servidor não sabe o que prometeu.

-- O id é o do pacote real com o prefixo "teste-", combinado com o site.
INSERT INTO donate_pacote (id, credits, amount_cents, so_staff, ativo)
SELECT 'teste-' || p.id, p.credits, 100, TRUE, TRUE
  FROM donate_pacote AS p
 WHERE p.id IN (
        'apoiador-iniciante', 'apoiador-bronze',   'apoiador-prata',
        'apoiador-ouro',      'apoiador-platina',  'apoiador-diamante',
        'apoiador-mestre',    'apoiador-lenda',    'apoiador-supremo'
       )
    ON CONFLICT (id) DO NOTHING;

-- Os brindes, lidos do pacote real. O `ON CONFLICT` de cima não serve aqui: esta
-- tabela tem id gerado e nenhuma restrição única, então rodar duas vezes duplicaria os
-- brindes. O NOT EXISTS é o que segura isso — e ele olha o ESPELHO, que é quem não
-- pode ter linha nenhuma antes desta migração.
INSERT INTO donate_pacote_item
       (pacote_id, item_index, eff1, effv1, eff2, effv2, eff3, effv3, ordem)
SELECT 'teste-' || i.pacote_id, i.item_index,
       i.eff1, i.effv1, i.eff2, i.effv2, i.eff3, i.effv3, i.ordem
  FROM donate_pacote_item AS i
 WHERE i.pacote_id IN (
        'apoiador-iniciante', 'apoiador-bronze',   'apoiador-prata',
        'apoiador-ouro',      'apoiador-platina',  'apoiador-diamante',
        'apoiador-mestre',    'apoiador-lenda',    'apoiador-supremo'
       )
   AND NOT EXISTS (
        SELECT 1 FROM donate_pacote_item AS j
         WHERE j.pacote_id = 'teste-' || i.pacote_id
       );
