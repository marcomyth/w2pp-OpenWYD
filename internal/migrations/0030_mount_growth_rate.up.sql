-- 0030_mount_growth_rate — a chance de um âmago fazer a montaria adulta subir
-- um nível, por linhagem e por faixa de nível.
--
-- Até aqui a chance era um 50 fixo no código (handler/amago.go), marcado como
-- UNVERIFIED porque BASE_GetGrowthRate não existe nas fontes que temos: o legado
-- lê a curva de algum lugar que não veio junto. Ou seja, não há original a
-- copiar — a curva é uma decisão de balanceamento deste servidor, e por isso
-- mora no banco e não numa constante.
--
-- Por que POR FAIXA e não um número por montaria: o custo de uma montaria não é
-- linear. Com a penalidade de um nível a cada cinco falhas, o avanço esperado
-- por âmago é (taxa - 0,2*(1-taxa)), e é isso que faz a última faixa dominar —
-- num Andaluz a 22%, os vinte níveis finais custam 313 dos 705 âmagos da
-- jornada inteira. Uma taxa única não consegue expressar "fácil de começar,
-- difícil de terminar", que é a forma que uma montaria de topo precisa ter.
--
-- band é a faixa de 20 níveis: 0 = 1..20, 1 = 21..40, ... 5 = 101..120. Seis
-- linhas por montaria, trinta montarias, 180 linhas no total quando tudo estiver
-- preenchido. Guardar por faixa em vez de por nível é o que mantém a tela
-- editável à mão; por nível seriam 3.600 campos que ninguém preenche.
--
-- mount_index é a montaria ADULTA (2360..2389). A cria não entra: ela sobe em
-- todo âmago, sem rolagem, e o que a segura é o limiar de crescimento, que vem
-- do cliente e não é editável.
--
-- A AUSÊNCIA de linha significa "usa o padrão do código", não "taxa zero" — a
-- mesma regra que 0023 e 0029 já seguem. Um servidor que nunca abriu esta tela
-- continua com o comportamento de hoje, e uma linhagem sem linha nenhuma não
-- vira uma montaria impossível por descuido.
--
-- Sem hot-reload, como todo o resto do overlay de conteúdo: o tmServer lê no
-- boot. Trocar a curva com o servidor de pé deixaria dois jogadores alimentando
-- a mesma montaria com chances diferentes na mesma tarde, e a diferença
-- apareceria como sorte, não como mudança.

CREATE TABLE IF NOT EXISTS mount_growth_rate (
    mount_index SMALLINT  NOT NULL CHECK (mount_index BETWEEN 2360 AND 2389),
    band        SMALLINT  NOT NULL CHECK (band BETWEEN 0 AND 5),
    -- 0..100. Vale registrar o que o servidor faz nas pontas: 100 é sucesso
    -- garantido, e abaixo de ~17 a penalidade come todo o avanço e a montaria
    -- trava — a tela avisa, o banco não proíbe, porque travar uma linhagem de
    -- propósito é uma decisão legítima de operação.
    rate        SMALLINT  NOT NULL CHECK (rate BETWEEN 0 AND 100),
    updated_by  TEXT      NOT NULL,
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (mount_index, band)
);
