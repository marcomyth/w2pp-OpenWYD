-- Tira da vitrine as ofertas que a 0160 inseriu, pela mesma chave (item_index,
-- title). O histórico de compras não depende delas: donate_shop_audit guarda o
-- shop_item_id sem FK, e o item já comprado vive na delivery_queue.
DELETE FROM donate_shop_item d
USING (VALUES
    (3379, 'Poção Divina 7 dias'), (3380, 'Poção Divina 15 dias'), (3381, 'Poção Divina 30 dias'),
    (3361, 'Poção Sephira 7 dias'), (3362, 'Poção Sephira 15 dias'), (3363, 'Poção Sephira 30 dias'),
    (3364, 'Poção de Saúde 7 dias'), (3365, 'Poção de Saúde 15 dias'), (3366, 'Poção de Saúde 30 dias'),
    (3467, 'Bolsa do Andarilho'), (3330, 'Trombeta Mágica ×120'), (3343, 'Pergaminho do Perdão'),
    (3336, 'Retorno da Habilidade'), (4140, 'Baú de Experiência ×1'), (4140, 'Baú de Experiência ×10'),
    (3314, 'Frango Assado ×5'), (3173, 'Pergaminho da Água (N) ×50'),
    (3393, 'RCoin 100'), (3393, 'RCoin 500'), (3394, 'RCoin 1K'), (3395, 'RCoin 3K'), (3441, 'RCoin 10K'),
    (3386, 'Gema de Diamante ×30'), (3387, 'Gema de Esmeralda ×30'), (3389, 'Gema de Garnet ×30'),
    (3388, 'Gema de Coral ×30'), (4019, 'Repletion D (Classe D) ×30'), (2426, 'Ração de Cavalo ×120'),
    (3344, 'Catalisador de Kapel'), (3345, 'Catalisador de Acuban'), (3346, 'Catalisador de Mencar'),
    (3351, 'Restaurador de Kapel'), (3352, 'Restaurador de Acuban'), (3353, 'Restaurador de Mencar'),
    (3407, 'Feijão Mágico (Azul)'), (3408, 'Feijão Mágico (Vermelho)'), (3409, 'Feijão Mágico (Verde)'),
    (3410, 'Feijão Mágico (Prateado)'), (3411, 'Feijão Mágico (Preto)'), (3412, 'Feijão Mágico (Roxo)'),
    (3413, 'Feijão Mágico (Marrom)'), (3414, 'Feijão Mágico (Rosa)'), (3415, 'Feijão Mágico (Amarelo)'),
    (3416, 'Feijão Mágico (Azul Claro)'),
    (3480, 'Pintura de Arma (Azul)'), (3481, 'Pintura de Arma (Vermelho)'), (3482, 'Pintura de Arma (Verde)'),
    (3483, 'Pintura de Arma (Prateado)'), (3484, 'Pintura de Arma (Preto)'), (3485, 'Pintura de Arma (Roxo)'),
    (3486, 'Pintura de Arma (Marrom)'), (3487, 'Pintura de Arma (Rosa)'), (3488, 'Pintura de Arma (Amarelo)'),
    (3489, 'Pintura de Arma (Azul Claro)'),
    (3901, 'Fada Azul 3 dias'), (3901, 'Fada Azul 5 dias'), (3901, 'Fada Azul 7 dias'),
    (3900, 'Fada Verde 3 dias'), (3900, 'Fada Verde 5 dias'), (3900, 'Fada Verde 7 dias'),
    (3902, 'Fada Vermelha 3 dias'), (3902, 'Fada Vermelha 5 dias'), (3902, 'Fada Vermelha 7 dias'),
    (3980, 'Shire 3 dias'), (3980, 'Shire 5 dias'), (3980, 'Shire 7 dias'),
    (3981, 'Thoroughbred 3 dias'), (3981, 'Thoroughbred 5 dias'), (3981, 'Thoroughbred 7 dias'),
    (3982, 'Klazedale 3 dias'), (3982, 'Klazedale 5 dias'), (3982, 'Klazedale 7 dias')
) AS v(item_index, title)
WHERE d.item_index = v.item_index AND d.title = v.title;
