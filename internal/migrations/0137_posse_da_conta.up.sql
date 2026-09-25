-- A POSSE DA CONTA, NO BANCO.
--
-- Até aqui, "esta conta já está em jogo" era uma pergunta que só o processo sabia
-- responder sobre si mesmo (handler/login.go, accountInUse). Com mais de um
-- tmServer no mesmo banco — e a sobreposição de um deploy é exatamente isso —
-- ninguém impedia a mesma conta de estar viva nos dois lados. O jogador cai de um,
-- entra no outro antes do save final do primeiro, carrega o estado velho, e os dois
-- lados divergem. É dupe, e é o mesmo motivo pelo qual a trava dentro do processo
-- existe desde 11/09/2026.
--
-- POR QUE COLUNAS NOVAS, e não a marca de presença que já existe: o
-- character.online_since é recado para o painel, ninguém no jogo o lê, e o BOOT
-- APAGA TODOS (ClearAllPresence, para não deixar personagem preso como online
-- depois de uma queda suja). Com dois tmServers, o que sobe apagaria a marca do que
-- está vivo. Qualquer posse que morasse ali nasceria inútil.
--
-- O DONO É UM NÚMERO DE EXECUÇÃO, e é o MESMO do 0135: a época que o tmServer pega
-- no boot. Um número só serve às duas travas, e ele já é único por execução porque
-- vem da sequência do banco.
--
-- A posse é da CONTA e não do personagem: a carga é da conta, e a trava de dentro do
-- processo também é por conta.
ALTER TABLE account
  ADD COLUMN dono_epoca     BIGINT,
  ADD COLUMN dono_desde     TIMESTAMPTZ,
  ADD COLUMN dono_batimento TIMESTAMPTZ;

-- Quem está com dono, para o batimento e para a tela do painel.
CREATE INDEX account_dono_epoca_idx ON account(dono_epoca) WHERE dono_epoca IS NOT NULL;

COMMENT ON COLUMN account.dono_epoca IS
  'Execucao do tmServer que esta com esta conta em jogo (a mesma epoca do 0135), ou NULO. Tomada no login, solta na MESMA transacao do save final.';
COMMENT ON COLUMN account.dono_batimento IS
  'Ultimo sinal de vida do dono. Sem sinal por mais que o prazo, a posse pode ser tomada: e o que impede um processo morto de trancar a conta para sempre.';
