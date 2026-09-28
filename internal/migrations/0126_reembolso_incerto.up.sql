-- 0126_reembolso_incerto — o quinto estado do reembolso: "pedi e não sei se entrou".
--
-- A 0114 criou quatro estados pensando nas respostas que a processadora dá. Falta o
-- caso em que ela NÃO responde: a chamada sai, a resposta não volta, e o pedido de
-- devolução pode ter sido criado ou não. A ponte chama isso de `incerto`, e é o
-- mesmo estado que o repasse já tem (0124) pelo mesmo motivo.
--
-- POR QUE ELE PRECISA EXISTIR NA COLUNA, e não podia continuar como PENDENTE: a
-- varredura pega os pendentes e pede. Um incerto deixado em pendente seria pedido de
-- novo na varredura seguinte, e a cada vinte segundos até alguém notar — cada
-- tentativa podendo virar um segundo pedido de devolução sobre o MESMO dinheiro.
--
-- Um segundo reembolso não é um erro simétrico ao primeiro. O comprador receberia o
-- dobro do que pagou, e o dobro sai da conta da Hanna.
--
-- ELE NÃO SAI SOZINHO. Nenhuma transição automática tira uma linha daqui — só a
-- pessoa que abrir o painel da processadora e vir se o pedido existe. É a mesma
-- regra do repasse incerto, e ela vale pela mesma razão: não há consulta de
-- reembolso na ponte para desempatar.
--
-- 1=PENDENTE 2=PEDIDO 3=CONCLUIDO 4=RECUSADO 5=INCERTO

-- O CHECK ENTRA AGORA, e não é zelo tardio: a coluna nasceu sem ele na 0114, e o
-- valor novo é a ocasião de escrever no banco qual é o vocabulário inteiro. Sem ele,
-- um 6 digitado por engano numa correção à mão viraria uma linha que nenhum código
-- sabe ler — e o estado do dinheiro de alguém não é lugar para isso.
--
-- NULO continua passando, porque nulo é o normal: quase nenhuma cobrança tem
-- reembolso, e a ausência é o jeito de dizer isso.
ALTER TABLE rmt_cobranca
    DROP CONSTRAINT IF EXISTS rmt_cobranca_reembolso_status_conhecido;

ALTER TABLE rmt_cobranca
    ADD CONSTRAINT rmt_cobranca_reembolso_status_conhecido
    CHECK (reembolso_status IS NULL OR reembolso_status BETWEEN 1 AND 5);

-- O índice da 0114 (`<> 3`) já pega o 5, porque incerto também é reembolso aberto
-- que precisa de gente. Nada a criar aqui.
