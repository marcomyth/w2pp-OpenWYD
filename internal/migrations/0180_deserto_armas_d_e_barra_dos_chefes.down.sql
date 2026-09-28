-- Tira as regras da 0180: as Armas D voltam ao template, sem o add, e os chefes
-- param de soltar a Barra de Prata (100Mi) fixa. A ficha do Ciclope Tirano, se
-- existir, volta a um terço.
DELETE FROM drop_rule WHERE
    (mob = 'Taron_Assassino' AND item IN (869, 910, 809, 935, 854, 902, 899, 824, 839, 884)) OR
    (mob = 'Adamant_Tauron'  AND item IN (936, 855, 902)) OR
    (mob = 'Manticora'       AND item IN (870, 911, 810)) OR
    (mob = 'Verme_'          AND item = 885) OR
    (mob IN ('Boss_Manticora', 'Boss_Hidra_Dourada', 'Boss_Dragao_Lich', 'Frenzy_Hidra') AND item = 4010);

UPDATE drop_rule_meta SET version = version + 1 WHERE id = TRUE;

UPDATE mob_template_stat
   SET max_hp = max_hp / 3, hp = hp / 3, damage = damage / 3, updated_at = now()
 WHERE template_name = 'Ciclope_Tirano';
