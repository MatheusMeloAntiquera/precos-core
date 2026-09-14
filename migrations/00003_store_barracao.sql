-- +goose Up
-- Barracão Supermercado, loja online operada pela VipCommerce. O e-commerce tem
-- um único centro de distribuição, em Bauru.
insert into stores (id, slug, name, city) values
    ('barracao-bauru', 'barracao-bauru', 'Barracão Supermercado', 'Bauru');

-- +goose Down
delete from stores where id = 'barracao-bauru';
