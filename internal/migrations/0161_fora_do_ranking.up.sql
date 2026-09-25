-- 0161_fora_do_ranking — esconder uma conta do ranking sem dar cargo a ela.
--
-- O PROBLEMA QUE ISTO RESOLVE: o ranking do site e do bot mostrava a equipe no topo, no
-- dia em que o servidor abriu. Filtrar por CARGO resolve a staff que tem cargo — e não
-- resolve quem está testando com conta comum, que é o caso das contas do FireBall e do
-- TheLenda.
--
-- POR QUE NÃO FILTRAR POR NOME. Depois do wipe de 25/09/2026 os nomes antigos ficaram
-- livres, e um jogador de verdade pode ter criado um deles. Uma lista de nomes no código
-- tiraria esse jogador do ranking sem ninguém entender por quê, e ela envelhece calada:
-- ninguém revisa uma lista que "já está funcionando".
--
-- A COLUNA NÃO DÁ PODER NENHUM. Ela esconde, e só. É deliberado que esconder do ranking
-- e ser da equipe sejam coisas separadas: dar cargo de moderador a uma conta só para
-- tirá-la de uma lista daria a ela os comandos de GM junto, que é caro demais para o
-- problema.
ALTER TABLE account
  ADD COLUMN fora_do_ranking BOOLEAN NOT NULL DEFAULT FALSE;

COMMENT ON COLUMN account.fora_do_ranking IS
  'Esconde a conta do ranking de EXP e de duelo, sem dar cargo nenhum a ela. Marcada '
  'pelo painel, por admin. Não afeta nada além das listas.';

-- Índice parcial: as marcadas são poucas — três, no dia em que isto nasceu — e o
-- ranking pergunta pelo NÃO marcado, que é quase todo mundo. Um índice cheio não
-- ajudaria a consulta e custaria escrita em toda conta.
CREATE INDEX account_fora_do_ranking_idx ON account(id) WHERE fora_do_ranking;
