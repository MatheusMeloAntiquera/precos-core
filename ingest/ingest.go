// Package ingest é a única porta de escrita no banco. Workers entregam
// []domain.Item e não conhecem nomes de tabela nem SQL.
package ingest

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/MatheusMeloAntiquera/precos-core/domain"
)

// Ingester escreve coletas no banco compartilhado.
type Ingester struct{ pool *pgxpool.Pool }

func New(pool *pgxpool.Pool) *Ingester { return &Ingester{pool: pool} }

// Result relata o que uma submissão gravou.
type Result struct {
	Products int64
	Prices   int64
}

const tmpTableDDL = `
create temp table tmp_items (
    match_key        text,
    ean              text,
    name             text,
    name_at_store    text,
    image_url        text,
    product_url      text,
    unit             text,
    store_sku_key    text,
    price_cents      bigint,
    list_price_cents bigint,
    club_price_cents bigint,
    is_available     boolean
) on commit drop`

// O distinct on protege contra dois SKUs compartilharem o mesmo EAN: sem ele o
// Postgres recusa a instrução inteira com "ON CONFLICT DO UPDATE command cannot
// affect row a second time".
const upsertProducts = `
insert into products (match_key, ean, name, image_url, unit, created_at, updated_at)
select distinct on (match_key)
       match_key, nullif(ean, ''), name, nullif(image_url, ''), nullif(unit, ''), now(), now()
from tmp_items
order by match_key
on conflict (match_key) do update set
    ean        = excluded.ean,
    name       = excluded.name,
    image_url  = excluded.image_url,
    unit       = excluded.unit,
    updated_at = now()`

const upsertPrices = `
insert into prices (product_id, store_id, store_sku_key, name_at_store, product_url,
                    price_cents, list_price_cents, club_price_cents, is_available, updated_at)
select distinct on (t.store_sku_key)
       p.id, $1, t.store_sku_key, nullif(t.name_at_store, ''), nullif(t.product_url, ''),
       t.price_cents, t.list_price_cents, t.club_price_cents, t.is_available, now()
from tmp_items t
join products p on p.match_key = t.match_key
order by t.store_sku_key
on conflict (store_id, store_sku_key) do update set
    product_id       = excluded.product_id,
    name_at_store    = excluded.name_at_store,
    product_url      = excluded.product_url,
    price_cents      = excluded.price_cents,
    list_price_cents = excluded.list_price_cents,
    club_price_cents = excluded.club_price_cents,
    is_available     = excluded.is_available,
    updated_at       = now()`

var tmpColumns = []string{
	"match_key", "ean", "name", "name_at_store", "image_url", "product_url",
	"unit", "store_sku_key", "price_cents", "list_price_cents", "club_price_cents", "is_available",
}

// Submit grava a coleta inteira numa única transação.
//
// Os dois passos são upsert, então CopyFrom não serve direto (não faz ON
// CONFLICT). O caminho rápido para ~15 mil linhas é CopyFrom para uma tabela
// temporária e um INSERT ... SELECT ... ON CONFLICT DO UPDATE a partir dela:
// uma ida ao banco em vez de quinze mil.
func (in *Ingester) Submit(ctx context.Context, storeID string, items []domain.Item) (Result, error) {
	var res Result
	if len(items) == 0 {
		return res, fmt.Errorf("submissão vazia para %s", storeID)
	}

	tx, err := in.pool.Begin(ctx)
	if err != nil {
		return res, fmt.Errorf("abrir transação: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, tmpTableDDL); err != nil {
		return res, fmt.Errorf("criar tabela temporária: %w", err)
	}

	rows := make([][]any, 0, len(items))
	for _, it := range items {
		if err := it.Validate(); err != nil {
			return res, fmt.Errorf("item inválido: %w", err)
		}
		// A coluna ean recebe o valor normalizado: nunca grave em ean algo que
		// não serviria de chave compartilhada.
		rows = append(rows, []any{
			it.MatchKey(storeID), domain.NormalizeEAN(it.EAN), it.Name, it.NameAtStore, it.ImageURL,
			it.ProductURL, it.Unit, it.StoreSKUKey,
			it.Price.Cents, it.Price.ListCents, it.Price.ClubCents, it.Available,
		})
	}
	if _, err := tx.CopyFrom(ctx, pgx.Identifier{"tmp_items"}, tmpColumns, pgx.CopyFromRows(rows)); err != nil {
		return res, fmt.Errorf("copiar itens: %w", err)
	}

	tag, err := tx.Exec(ctx, upsertProducts)
	if err != nil {
		return res, fmt.Errorf("upsert de products: %w", err)
	}
	res.Products = tag.RowsAffected()

	tag, err = tx.Exec(ctx, upsertPrices, storeID)
	if err != nil {
		return res, fmt.Errorf("upsert de prices: %w", err)
	}
	res.Prices = tag.RowsAffected()

	if err := tx.Commit(ctx); err != nil {
		return res, fmt.Errorf("commit: %w", err)
	}
	return res, nil
}
