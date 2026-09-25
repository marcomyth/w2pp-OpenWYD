-- O EMBLEMA DE GUILDA.
--
-- Não existia nada de emblema no servidor: nem coluna, nem campo, nem rota. A
-- imagem é um BMP de 630 bytes que o cliente 1.0.0 lê e desenha ao lado do nome da
-- guilda, e quem a troca é o líder, pelo site.
--
-- A IMAGEM MORA NO BANCO, e não num arquivo. Um arquivo exigiria um disco
-- compartilhado entre serviços que hoje não compartilham nada além do Postgres, e
-- daria para a imagem sumir sem a linha sumir — estado pela metade, que é
-- exatamente o que não se quer de uma coisa que todo mundo na guilda vê. São 630
-- bytes: o tamanho de uma linha de texto médio.
--
-- emblema_trocado_em SERVE A DUAS COISAS e por isso é uma coluna e não duas:
--
--   1. o Last-Modified da rota de imagem do site, que só é pedido quando existe
--      emblema;
--   2. o limite de uma troca a cada 5 minutos POR GUILDA — e aí ele tem de contar
--      também o REMOVER, senão alternar pôr-e-tirar burlaria o limite.
--
-- Por guilda e não por conta: o custo da troca é a imagem mudando para todos os
-- membros de uma vez.
ALTER TABLE guild
  ADD COLUMN emblema            BYTEA,
  ADD COLUMN emblema_trocado_em TIMESTAMPTZ;

-- O tamanho exato no banco também, e não só no código que grava: coluna é contrato
-- mais duradouro que função. 630 é o BMP inteiro — 54 de cabeçalho e 576 de pixels.
ALTER TABLE guild
  ADD CONSTRAINT guild_emblema_630_bytes
  CHECK (emblema IS NULL OR octet_length(emblema) = 630);

COMMENT ON COLUMN guild.emblema IS
  'BMP 24 bits 16x12 sem transparencia, 630 bytes exatos, como o cliente 1.0.0 le. NULO = guilda sem emblema.';
COMMENT ON COLUMN guild.emblema_trocado_em IS
  'Ultima troca do emblema, incluindo a remocao. Serve ao Last-Modified da imagem e ao limite de uma troca por 5 minutos por guilda.';
