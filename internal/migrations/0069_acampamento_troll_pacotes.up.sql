-- 0069_acampamento_troll_pacotes — o Troll Caos troca armas por pacotes de
-- âmago, e o Troll Enigma passa a soltar Pergaminhos da Água.
--
-- Pedido do Marco em 16/09/2026, depois de um Troll Caos soltar 3 Armas D numa
-- morte só: menos arma no Caos, e âmagos em pacote. A Mesa de Drops diz se o item
-- cai e com que chance; o tamanho do pacote não cabe nela e fica no tmServer
-- (acampamentoTrollPacks, handler/acampamento_troll.go), como nos guardiões do
-- Castelo Orc.
--
-- Por entrada há 4 Troll Caos e 1 Enigma. Chance em centésimos de por cento:
--
--   Troll Caos                         antes    agora    por entrada
--   cada uma das 8 Armas D             10%      2,5%     ~0,8 arma (era ~3,2)
--   Âmago Cav. s/ Sela N 2396   ×20    —        15%      ~12
--   Âmago Cav. s/ Sela B 2401   ×20    —        10%      ~8
--   Âmago Cav. Fantasma N 2397  ×10    25% ×1   15%      ~6 (era ~1)
--   Âmago Cav. Fantasma B 2402  ×10    15% ×1   10%      ~4 (era ~0,6)
--
--   Troll Enigma
--   Pergaminho da Água (N) LV1 3173 ×5 —        30%      ~1,5
--
-- Com 8 rolagens a 2,5%, duas armas do mesmo Caos saem ~1,6% das vezes; três,
-- quase nunca.
--
-- As linhas que já existiam (armas e Fantasma do Caos, da 0064) são sobrescritas:
-- o pedido é justamente mudar esses números. As novas entram por cima do que o
-- painel tiver gravado também, pelo mesmo motivo.

INSERT INTO drop_rule (mob, item, chance) VALUES
    ('ATroll_Caos',    869,  250), ('ATroll_Caos',    809,  250),
    ('ATroll_Caos',    910,  250), ('ATroll_Caos',    824,  250),
    ('ATroll_Caos',    935,  250), ('ATroll_Caos',    899,  250),
    ('ATroll_Caos',    854,  250), ('ATroll_Caos',    902,  250),
    ('ATroll_Caos',   2396, 1500), ('ATroll_Caos',   2401, 1000),
    ('ATroll_Caos',   2397, 1500), ('ATroll_Caos',   2402, 1000),
    ('ATroll_Enigma', 3173, 3000)
ON CONFLICT (mob, item) DO UPDATE SET chance = EXCLUDED.chance, updated_at = now();

-- O tmServer relê a mesa quando a versão muda.
UPDATE drop_rule_meta SET version = version + 1 WHERE id = TRUE;
