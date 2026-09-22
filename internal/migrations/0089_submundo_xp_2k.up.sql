-- 0089_submundo_xp_2k — os monstros do Submundo passam a pagar 2.000 de
-- experiência. A área paga em drop, não em XP.
--
-- Dez templates das duas salas (1283,3714-1538,3838 e 1153,3966-1533,4089):
-- Guarda, Argos Errante, Troll Ghoul, Aqua Golem, Morlock, Demon Gorgon,
-- CH Troll Ghoul, Cav. Elfo Negro, Elfo Negro Abj e FrenzyDemonLord. Vinham de
-- 23.940 a 2.990.849 — o Frenzy sozinho pagava três milhões por abate.
--
-- Dois monstros que ficam DENTRO dessas coordenadas não entram, cada um por um
-- motivo:
--
--   Mestre Elfo e Servo Elfo são a arena dos Elfos da Quest 256 (blocos
--   3509-3515), que por acaso fica na caixa que o Regions.txt chama de
--   Submundo_2. É conteúdo de quest paga, com saque próprio desde a 0075, e
--   mexer na XP dela não foi o que se pediu.
--
--   A Flame Gargula tem sete blocos fora do Submundo (3494,3559 e 3560,3622,
--   contra onze dentro), e a Exp mora no template: baixá-la levaria a XP daquela
--   outra área junto. Ela é também a escolta do chefe, oito por bloco.
--
-- A Exp fica no STRUCT_MOB (offset 32) e os dez arquivos mudam no mesmo commit.
-- A migração cobre só o caso em que o painel já tem ficha editada para o
-- template: mob_template_stat substitui a ficha INTEIRA no boot (mobstat.Apply),
-- então uma linha lá com a Exp velha desfaria o arquivo. É UPDATE e não INSERT
-- de propósito — inserir uma linha nova zeraria todos os outros campos da ficha.
UPDATE mob_template_stat SET exp = 2000, updated_at = now()
WHERE exp <> 2000 AND template_name IN (
    'Guarda', 'Argos_Errante', 'Troll_Ghoul', 'Aqua_Golem', 'Morlock', 'Demon_Gorgon',
    'CH_Troll_Ghoul', 'Cav._Elfo_Negro', 'Elfo_Negro_Abj', 'FrenzyDemonLord');
