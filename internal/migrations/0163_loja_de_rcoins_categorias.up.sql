-- 0163_loja_de_rcoins_categorias — as abas da Loja de Rcoin do jogo.
--
-- A janela do cliente (0x0F0C, tmserver/internal/protocol/lojarcoin.go) pede as
-- ofertas por categoria, 1 a 6, uma por aba. O número é parte do contrato com o
-- cliente, que o teste de lá prende na Fada = 5; a ordem é a da lista da equipe:
--
--   1 Consumíveis   2 Utilitários   3 Montaria   4 Cosméticos   5 Fadas   6 Esferas
--
-- 0 é "sem categoria": a oferta continua no site e NÃO aparece no jogo. Uma linha
-- que ninguém classificou não é empurrada para uma aba que não é a dela.
ALTER TABLE donate_shop_item
    ADD COLUMN category SMALLINT NOT NULL DEFAULT 0 CHECK (category BETWEEN 0 AND 6);

UPDATE donate_shop_item d SET category = v.category
FROM (VALUES
    (3379, 'Poção Divina 7 dias', 1), (3380, 'Poção Divina 15 dias', 1), (3381, 'Poção Divina 30 dias', 1),
    (3361, 'Poção Sephira 7 dias', 1), (3362, 'Poção Sephira 15 dias', 1), (3363, 'Poção Sephira 30 dias', 1),
    (3364, 'Poção de Saúde 7 dias', 1), (3365, 'Poção de Saúde 15 dias', 1), (3366, 'Poção de Saúde 30 dias', 1),
    (3467, 'Bolsa do Andarilho', 1), (3330, 'Trombeta Mágica ×120', 1), (3343, 'Pergaminho do Perdão', 1),
    (3336, 'Retorno da Habilidade', 1), (4140, 'Baú de Experiência ×1', 1), (4140, 'Baú de Experiência ×10', 1),
    (3314, 'Frango Assado ×5', 1), (3173, 'Pergaminho da Água (N) ×50', 1),
    (3393, 'RCoin 100', 2), (3393, 'RCoin 500', 2), (3394, 'RCoin 1K', 2), (3395, 'RCoin 3K', 2), (3441, 'RCoin 10K', 2),
    (3386, 'Gema de Diamante ×30', 2), (3387, 'Gema de Esmeralda ×30', 2), (3389, 'Gema de Garnet ×30', 2),
    (3388, 'Gema de Coral ×30', 2), (4019, 'Repletion D (Classe D) ×30', 2), (2426, 'Ração de Cavalo ×120', 2),
    (3344, 'Catalisador de Kapel', 3), (3345, 'Catalisador de Acuban', 3), (3346, 'Catalisador de Mencar', 3),
    (3351, 'Restaurador de Kapel', 3), (3352, 'Restaurador de Acuban', 3), (3353, 'Restaurador de Mencar', 3),
    (3407, 'Feijão Mágico (Azul)', 4), (3408, 'Feijão Mágico (Vermelho)', 4), (3409, 'Feijão Mágico (Verde)', 4),
    (3410, 'Feijão Mágico (Prateado)', 4), (3411, 'Feijão Mágico (Preto)', 4), (3412, 'Feijão Mágico (Roxo)', 4),
    (3413, 'Feijão Mágico (Marrom)', 4), (3414, 'Feijão Mágico (Rosa)', 4), (3415, 'Feijão Mágico (Amarelo)', 4),
    (3416, 'Feijão Mágico (Azul Claro)', 4),
    (3480, 'Pintura de Arma (Azul)', 4), (3481, 'Pintura de Arma (Vermelho)', 4), (3482, 'Pintura de Arma (Verde)', 4),
    (3483, 'Pintura de Arma (Prateado)', 4), (3484, 'Pintura de Arma (Preto)', 4), (3485, 'Pintura de Arma (Roxo)', 4),
    (3486, 'Pintura de Arma (Marrom)', 4), (3487, 'Pintura de Arma (Rosa)', 4), (3488, 'Pintura de Arma (Amarelo)', 4),
    (3489, 'Pintura de Arma (Azul Claro)', 4),
    (3901, 'Fada Azul 3 dias', 5), (3901, 'Fada Azul 5 dias', 5), (3901, 'Fada Azul 7 dias', 5),
    (3900, 'Fada Verde 3 dias', 5), (3900, 'Fada Verde 5 dias', 5), (3900, 'Fada Verde 7 dias', 5),
    (3902, 'Fada Vermelha 3 dias', 5), (3902, 'Fada Vermelha 5 dias', 5), (3902, 'Fada Vermelha 7 dias', 5),
    (3980, 'Shire 3 dias', 6), (3980, 'Shire 5 dias', 6), (3980, 'Shire 7 dias', 6),
    (3981, 'Thoroughbred 3 dias', 6), (3981, 'Thoroughbred 5 dias', 6), (3981, 'Thoroughbred 7 dias', 6),
    (3982, 'Klazedale 3 dias', 6), (3982, 'Klazedale 5 dias', 6), (3982, 'Klazedale 7 dias', 6)
) AS v(item_index, title, category)
WHERE d.item_index = v.item_index AND d.title = v.title;
