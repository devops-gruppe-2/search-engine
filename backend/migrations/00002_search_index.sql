-- Søgning med PostgreSQL's fuldtekstsøgning i stedet for SQLite FTS5 (opgaverne
-- "Upgrade the Database" og "Indexing").
--
-- search_vector er en kolonne, som PostgreSQL selv udregner, når en side gemmes eller ændres
-- (GENERATED ... STORED). Derfor behøver vi ingen triggers, som vi havde med FTS5.
--   - Ord i titlen får vægt A og ord i teksten vægt B, så titlen kan tælle mere i rangeringen.
--   - Danske sider bruger den danske ordbog, engelske den engelske. Ordbogen sørger for
--     stemming ("languages" finder "language") og fjerner småord som "the" og "is".

-- +goose Up
ALTER TABLE pages ADD COLUMN search_vector tsvector GENERATED ALWAYS AS (
    setweight(to_tsvector(CASE WHEN language = 'da' THEN 'danish'::regconfig ELSE 'english'::regconfig END, title), 'A') ||
    setweight(to_tsvector(CASE WHEN language = 'da' THEN 'danish'::regconfig ELSE 'english'::regconfig END, content), 'B')
) STORED;

-- GIN-index: uden det skal PostgreSQL læse alle sider ved hver søgning (sequential scan)
CREATE INDEX pages_search_vector_idx ON pages USING GIN (search_vector);

-- +goose Down
DROP INDEX pages_search_vector_idx;
ALTER TABLE pages DROP COLUMN search_vector;
