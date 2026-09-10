-- +goose Up
-- Coletas restritas a algumas categorias não podem servir de baseline para a
-- guarda de sanidade: comparar um catálogo inteiro com uma seção só neutraliza
-- a checagem.
alter table collection_runs add column partial boolean not null default false;

create index collection_runs_baseline_idx
    on collection_runs (store_id, finished_at desc)
    where status = 'success' and not partial;

-- +goose Down
drop index collection_runs_baseline_idx;
alter table collection_runs drop column partial;
