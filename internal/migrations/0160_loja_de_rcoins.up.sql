-- 0160_loja_de_rcoins — a vitrine da loja de Rcoins do site.
--
-- A loja existia inteira (0008: catálogo, carteira, compra, fila de entrega) e
-- nunca teve uma oferta: o site mostrava a vitrine vazia. Estas são as 72 ofertas
-- da lista da equipe (artefato "Loja de Rcoins", 25/09/2026), com os índices
-- conferidos no ItemList.csv.
--
-- TRÊS REGRAS DO SITE decidem a forma das linhas (w2pp-site, src/lib/loja.ts):
--
--   1. Oferta sem descrição NÃO aparece na vitrine ("não se vende o que não diz o
--      que dá"). Por isso a descrição é o texto para o jogador, conferido contra o
--      handler de cada item, e não o nome da categoria.
--   2. A duração mostrada sai SÓ de expires_days ("Dura N dias" / "Permanente").
--      Fadas e Esferas levam o prazo em expires_days, e não em um efeito 106 na
--      linha: com o 106 na linha e expires_days 0 o site anunciaria uma Fada de 3
--      dias como permanente. A compra transforma expires_days em EF_WDAY na forma
--      não iniciada (store.donateShopPayload), então o prazo continua começando
--      quando o item é equipado — e a fada continua gastando só enquanto vestida.
--   3. A vitrine sai em ordem de id e não tem abas. A ordem de inserção abaixo é a
--      ordem das categorias da lista: consumíveis, utilitários, montaria,
--      cosméticos, fadas, esferas.
--
-- Quantidade é EF_AMOUNT (61) na linha: o item chega como uma pilha e cada uso
-- gasta uma (consumeOneItem). Os sete empilháveis de verdade (Baú de Experiência,
-- Pergaminho da Água, RCoins, Classe D) precisam do 61 mesmo com uma unidade —
-- empilhável sem ele derruba o cliente —, e a compra carimba 61/1 se faltar.
--
-- Idempotente por (item_index, title): rodar de novo, ou por cima de uma oferta
-- que a equipe já tenha cadastrado igual no painel, não duplica nada.
INSERT INTO donate_shop_item
    (item_index, eff1, effv1, price, title, description, enabled, expires_days)
SELECT v.item_index, v.eff1, v.effv1, v.price, v.title, v.description, TRUE, v.expires_days
FROM (VALUES
    -- Consumíveis
    (1,  3379, 0, 0,   50,   'Poção Divina 7 dias',      '+20% de HP, MP e dano enquanto durar.', 0),
    (2,  3380, 0, 0,   90,   'Poção Divina 15 dias',     '+20% de HP, MP e dano enquanto durar.', 0),
    (3,  3381, 0, 0,   160,  'Poção Divina 30 dias',     '+20% de HP, MP e dano enquanto durar.', 0),
    (4,  3361, 0, 0,   50,   'Poção Sephira 7 dias',     '+5% de dano, +30 de dano e +5 de magia enquanto durar.', 0),
    (5,  3362, 0, 0,   90,   'Poção Sephira 15 dias',    '+5% de dano, +30 de dano e +5 de magia enquanto durar.', 0),
    (6,  3363, 0, 0,   160,  'Poção Sephira 30 dias',    '+5% de dano, +30 de dano e +5 de magia enquanto durar.', 0),
    (7,  3364, 0, 0,   50,   'Poção de Saúde 7 dias',    '+10% de HP e MP enquanto durar.', 0),
    (8,  3365, 0, 0,   90,   'Poção de Saúde 15 dias',   '+10% de HP e MP enquanto durar.', 0),
    (9,  3366, 0, 0,   160,  'Poção de Saúde 30 dias',   '+10% de HP e MP enquanto durar.', 0),
    (10, 3467, 0, 0,   60,   'Bolsa do Andarilho',       'Abre mais uma bolsa na mochila por 30 dias.', 0),
    (11, 3330, 61, 120, 20,  'Trombeta Mágica ×120',     'Com ela na mochila, /gritar fala com o servidor inteiro. Cada grito gasta uma.', 0),
    (12, 3343, 0, 0,   400,  'Pergaminho do Perdão',     'Zera o contador de caos (PK) e devolve a cor normal ao nome.', 0),
    (13, 3336, 0, 0,   30,   'Retorno da Habilidade',    'Entregue ao Mestre de Habilidade para redistribuir até 1.000 pontos de atributo.', 0),
    (14, 4140, 61, 1,  10,   'Baú de Experiência ×1',    '+100% de XP por 2 horas. Cada baú soma mais 2 horas.', 0),
    (15, 4140, 61, 10, 90,   'Baú de Experiência ×10',   '+100% de XP por 2 horas. Cada baú soma mais 2 horas.', 0),
    (16, 3314, 61, 5,  20,   'Frango Assado ×5',         '+2.000 de dano contra monstros por 4 horas. Cada frango soma mais 4 horas, até 24.', 0),
    (17, 3173, 61, 50, 160,  'Pergaminho da Água (N) ×50', 'Leva você e o grupo à primeira sala da Água (N). Limpar a sala a tempo dá o pergaminho da seguinte.', 0),
    -- Utilitários
    (18, 3393, 61, 1,  110,   'RCoin 100',  'Use no jogo para somar 100 Rcoins ao saldo da conta.', 0),
    (19, 3393, 61, 5,  520,   'RCoin 500',  'Cinco moedas de 100. Cada uma, usada no jogo, soma 100 Rcoins ao saldo da conta.', 0),
    (20, 3394, 61, 1,  1050,  'RCoin 1K',   'Use no jogo para somar 1.000 Rcoins ao saldo da conta.', 0),
    (21, 3395, 61, 1,  3100,  'RCoin 3K',   'Use no jogo para somar 3.000 Rcoins ao saldo da conta.', 0),
    (22, 3441, 61, 1,  10200, 'RCoin 10K',  'Use no jogo para somar 10.000 Rcoins ao saldo da conta.', 0),
    (23, 3386, 61, 30, 10,  'Gema de Diamante ×30',   'Troca a joia do equipamento ancestral equipado pela de Diamante. Cada uso gasta uma.', 0),
    (24, 3387, 61, 30, 10,  'Gema de Esmeralda ×30',  'Troca a joia do equipamento ancestral equipado pela de Esmeralda. Cada uso gasta uma.', 0),
    (25, 3389, 61, 30, 10,  'Gema de Garnet ×30',     'Troca a joia do equipamento ancestral equipado pela de Garnet. Cada uso gasta uma.', 0),
    (26, 3388, 61, 30, 10,  'Gema de Coral ×30',      'Troca a joia do equipamento ancestral equipado pela de Coral. Cada uso gasta uma.', 0),
    (27, 4019, 61, 30, 15,  'Repletion D (Classe D) ×30', 'Refaz o refino (até +6) e os dois adicionais de um equipamento de classe D que esteja na mochila. Cada uso gasta uma.', 0),
    (28, 2426, 61, 120, 8,  'Ração de Cavalo ×120',   'Alimenta as montarias da linha dos cavalos: repõe vida e ração. Cada uso gasta uma.', 0),
    -- Montaria
    (29, 3344, 0, 0, 30,  'Catalisador de Kapel',   'Transforma na hora a cria da linha Kapel, equipada, em montaria adulta.', 0),
    (30, 3345, 0, 0, 160, 'Catalisador de Acuban',  'Transforma na hora a cria da linha Acuban, equipada, em montaria adulta.', 0),
    (31, 3346, 0, 0, 260, 'Catalisador de Mencar',  'Transforma na hora a cria da linha Mencar, equipada, em montaria adulta.', 0),
    (32, 3351, 0, 0, 30,  'Restaurador de Kapel',   '+1 ou +2 de vitalidade na montaria adulta da linha Kapel, equipada e viva, com vitalidade entre 6 e 49.', 0),
    (33, 3352, 0, 0, 40,  'Restaurador de Acuban',  '+1 ou +2 de vitalidade na montaria adulta da linha Acuban, equipada e viva, com vitalidade entre 6 e 49.', 0),
    (34, 3353, 0, 0, 50,  'Restaurador de Mencar',  '+1 ou +2 de vitalidade na montaria adulta da linha Mencar, equipada e viva, com vitalidade entre 6 e 49.', 0),
    -- Cosméticos
    (35, 3407, 0, 0, 60,  'Feijão Mágico (Azul)',        'Pinta uma peça equipada do set na cor Azul.', 0),
    (36, 3408, 0, 0, 60,  'Feijão Mágico (Vermelho)',    'Pinta uma peça equipada do set na cor Vermelho.', 0),
    (37, 3409, 0, 0, 60,  'Feijão Mágico (Verde)',       'Pinta uma peça equipada do set na cor Verde.', 0),
    (38, 3410, 0, 0, 60,  'Feijão Mágico (Prateado)',    'Pinta uma peça equipada do set na cor Prateado.', 0),
    (39, 3411, 0, 0, 60,  'Feijão Mágico (Preto)',       'Pinta uma peça equipada do set na cor Preto.', 0),
    (40, 3412, 0, 0, 60,  'Feijão Mágico (Roxo)',        'Pinta uma peça equipada do set na cor Roxo.', 0),
    (41, 3413, 0, 0, 60,  'Feijão Mágico (Marrom)',      'Pinta uma peça equipada do set na cor Marrom.', 0),
    (42, 3414, 0, 0, 60,  'Feijão Mágico (Rosa)',        'Pinta uma peça equipada do set na cor Rosa.', 0),
    (43, 3415, 0, 0, 60,  'Feijão Mágico (Amarelo)',     'Pinta uma peça equipada do set na cor Amarelo.', 0),
    (44, 3416, 0, 0, 60,  'Feijão Mágico (Azul Claro)',  'Pinta uma peça equipada do set na cor Azul Claro.', 0),
    (45, 3480, 0, 0, 500, 'Pintura de Arma (Azul)',       'Pinta a arma equipada na cor Azul.', 0),
    (46, 3481, 0, 0, 500, 'Pintura de Arma (Vermelho)',   'Pinta a arma equipada na cor Vermelho.', 0),
    (47, 3482, 0, 0, 500, 'Pintura de Arma (Verde)',      'Pinta a arma equipada na cor Verde.', 0),
    (48, 3483, 0, 0, 500, 'Pintura de Arma (Prateado)',   'Pinta a arma equipada na cor Prateado.', 0),
    (49, 3484, 0, 0, 500, 'Pintura de Arma (Preto)',      'Pinta a arma equipada na cor Preto.', 0),
    (50, 3485, 0, 0, 500, 'Pintura de Arma (Roxo)',       'Pinta a arma equipada na cor Roxo.', 0),
    (51, 3486, 0, 0, 500, 'Pintura de Arma (Marrom)',     'Pinta a arma equipada na cor Marrom.', 0),
    (52, 3487, 0, 0, 500, 'Pintura de Arma (Rosa)',       'Pinta a arma equipada na cor Rosa.', 0),
    (53, 3488, 0, 0, 500, 'Pintura de Arma (Amarelo)',    'Pinta a arma equipada na cor Amarelo.', 0),
    (54, 3489, 0, 0, 500, 'Pintura de Arma (Azul Claro)', 'Pinta a arma equipada na cor Azul Claro.', 0),
    -- Fadas: o prazo vai em expires_days e só corre com a fada equipada.
    (55, 3901, 0, 0, 60,  'Fada Azul 3 dias',     '+32% de drop. O prazo só corre com a fada equipada.', 3),
    (56, 3901, 0, 0, 80,  'Fada Azul 5 dias',     '+32% de drop. O prazo só corre com a fada equipada.', 5),
    (57, 3901, 0, 0, 110, 'Fada Azul 7 dias',     '+32% de drop. O prazo só corre com a fada equipada.', 7),
    (58, 3900, 0, 0, 60,  'Fada Verde 3 dias',    '+16% de XP. O prazo só corre com a fada equipada.', 3),
    (59, 3900, 0, 0, 80,  'Fada Verde 5 dias',    '+16% de XP. O prazo só corre com a fada equipada.', 5),
    (60, 3900, 0, 0, 110, 'Fada Verde 7 dias',    '+16% de XP. O prazo só corre com a fada equipada.', 7),
    (61, 3902, 0, 0, 60,  'Fada Vermelha 3 dias', '+32% de XP e +16% de drop. O prazo só corre com a fada equipada.', 3),
    (62, 3902, 0, 0, 80,  'Fada Vermelha 5 dias', '+32% de XP e +16% de drop. O prazo só corre com a fada equipada.', 5),
    (63, 3902, 0, 0, 110, 'Fada Vermelha 7 dias', '+32% de XP e +16% de drop. O prazo só corre com a fada equipada.', 7),
    -- Esferas: o prazo vai em expires_days e começa quando a montaria é equipada.
    (64, 3980, 0, 0, 30,  'Shire 3 dias',         'Montaria: +150 de dano, +15 de magia, 20% de absorção contra monstros e +3% de XP. O prazo começa quando você equipa.', 3),
    (65, 3980, 0, 0, 40,  'Shire 5 dias',         'Montaria: +150 de dano, +15 de magia, 20% de absorção contra monstros e +3% de XP. O prazo começa quando você equipa.', 5),
    (66, 3980, 0, 0, 50,  'Shire 7 dias',         'Montaria: +150 de dano, +15 de magia, 20% de absorção contra monstros e +3% de XP. O prazo começa quando você equipa.', 7),
    (67, 3981, 0, 0, 70,  'Thoroughbred 3 dias',  'Montaria: +200 de dano, +30 de magia, 20% de absorção contra monstros e +5% de XP. O prazo começa quando você equipa.', 3),
    (68, 3981, 0, 0, 95,  'Thoroughbred 5 dias',  'Montaria: +200 de dano, +30 de magia, 20% de absorção contra monstros e +5% de XP. O prazo começa quando você equipa.', 5),
    (69, 3981, 0, 0, 120, 'Thoroughbred 7 dias',  'Montaria: +200 de dano, +30 de magia, 20% de absorção contra monstros e +5% de XP. O prazo começa quando você equipa.', 7),
    (70, 3982, 0, 0, 90,  'Klazedale 3 dias',     'Montaria: +250 de dano, +45 de magia, 20% de absorção contra monstros e +7% de XP. O prazo começa quando você equipa.', 3),
    (71, 3982, 0, 0, 125, 'Klazedale 5 dias',     'Montaria: +250 de dano, +45 de magia, 20% de absorção contra monstros e +7% de XP. O prazo começa quando você equipa.', 5),
    (72, 3982, 0, 0, 170, 'Klazedale 7 dias',     'Montaria: +250 de dano, +45 de magia, 20% de absorção contra monstros e +7% de XP. O prazo começa quando você equipa.', 7)
) AS v(ordem, item_index, eff1, effv1, price, title, description, expires_days)
WHERE NOT EXISTS (
    SELECT 1 FROM donate_shop_item d
    WHERE d.item_index = v.item_index AND d.title = v.title
)
ORDER BY v.ordem;
