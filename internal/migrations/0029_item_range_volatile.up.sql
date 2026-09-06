-- 0029_item_range_volatile — os dois efeitos que ficam de fora do modelo de
-- score, agora editáveis como o resto.
--
-- Por que não entraram no 0023: aquela tabela guarda o que o item concede, e a
-- lista branca de efeitos (internal/itemeffect) só admite o que o score sabe
-- representar. EF_RANGE e EF_VOLATILE são lidos pelo jogo por caminhos próprios,
-- e colocá-los na lista branca faria os dois entrarem no score — que é
-- exatamente o que o comentário de lá proíbe. Daí duas colunas separadas,
-- aplicadas nos mapas certos no boot.
--
-- 0 quer dizer "o item não tem", não "não mexido", e isso está certo pela regra
-- que o 0023 já usa: a LINHA substitui, e a ausência de linha é que significa
-- "usa o CSV como está". Então um item nunca editado continua intacto, e um
-- moderador que quiser tirar o alcance de um item digita 0. É também como o
-- jogo já se comporta: ItemRanges e itemVolatiles são mapas, e a chave ausente
-- lê 0 do mesmo jeito.
--
-- ef_range é o alcance, e vale saber o que ele faz DE VERDADE aqui: o mapa que
-- ele alimenta é lido no nascimento de MONSTRO (world/api.go — o maior EF_RANGE
-- entre os 16 equipamentos do template). O alcance do jogador não sai daqui
-- neste servidor. Ou seja, isto afina monstro que carrega o item, e é a única
-- maneira que existe de mexer no alcance de um monstro: o editor de monstro não
-- tem esse botão porque o número mora no item.
--
-- ef_volatile é o que o item FAZ ao ser usado — poção, divina, pergaminho,
-- pedra. Não é estatística, é a classe do item. Mudar aqui muda toda cópia que
-- já existe no inventário de todo mundo, e um valor que o servidor não trata faz
-- o item simplesmente não fazer nada. Por isso a tela oferece escolha entre os
-- valores realmente implementados em vez de um campo numérico solto.

ALTER TABLE item_stat
    ADD COLUMN IF NOT EXISTS ef_range    SMALLINT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS ef_volatile SMALLINT NOT NULL DEFAULT 0;
