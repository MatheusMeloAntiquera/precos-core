-- +goose Up
create table stores (
    id   text primary key,
    slug text not null,
    name text not null,
    city text not null
);

-- products guarda o produto: EAN, nome e imagem canônicos.
-- match_key é a chave de upsert (ean, ou "<store_id>:<store_sku_key>" quando
-- o mercado não expõe código de barras).
create table products (
    id         bigserial primary key,
    match_key  text not null unique,
    ean        text,
    name       text not null,
    image_url  text,
    unit       text,
    created_at timestamptz not null default now(),
    updated_at timestamptz not null default now()
);

-- prices guarda uma linha por produto por mercado, sobrescrita a cada coleta.
-- Os campos que variam por mercado moram aqui, e não numa tabela intermediária.
create table prices (
    id               bigserial primary key,
    product_id       bigint not null references products (id),
    store_id         text   not null references stores (id),
    store_sku_key    text   not null,
    name_at_store    text,
    product_url      text,
    price_cents      bigint,
    list_price_cents bigint,
    club_price_cents bigint,
    is_available     boolean not null default true,
    updated_at       timestamptz not null default now(),
    unique (store_id, store_sku_key)
);

create table collection_runs (
    id            bigserial primary key,
    store_id      text not null references stores (id),
    started_at    timestamptz not null default now(),
    finished_at   timestamptz,
    status        text not null,
    items_found   integer not null default 0,
    requests_made integer not null default 0,
    error         text
);

create index products_ean_idx on products (ean);
create index prices_product_id_idx on prices (product_id);
create index collection_runs_store_status_idx on collection_runs (store_id, status, finished_at desc);

insert into stores (id, slug, name, city) values
    ('confianca-bauru', 'confianca-bauru', 'Confiança Supermercados', 'Bauru');

-- +goose Down
drop table collection_runs;
drop table prices;
drop table products;
drop table stores;
