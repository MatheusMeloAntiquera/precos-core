// Command precosctl administra o banco compartilhado da plataforma de preços.
//
// Só este binário aplica migrations. Workers nunca migram: isso causaria
// corrida e versões divergentes de schema entre os repositórios.
package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"os"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"

	"github.com/MatheusMeloAntiquera/precos-core/migrations"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "erro:", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Println(`precosctl — administração do banco de preços

  precosctl migrate [--down]   aplica (ou reverte) as migrations
  precosctl stats              resumo do que há no banco
  precosctl prune              apaga produtos que nenhum preço referencia

Configuração: DATABASE_URL`)
}

func run() error {
	if len(os.Args) < 2 {
		usage()
		return nil
	}

	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		return fmt.Errorf("DATABASE_URL não definida")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return fmt.Errorf("abrir conexão: %w", err)
	}
	defer db.Close()

	if err := db.PingContext(ctx); err != nil {
		return fmt.Errorf("conectar no Postgres: %w", err)
	}

	switch os.Args[1] {
	case "migrate":
		fs := flag.NewFlagSet("migrate", flag.ExitOnError)
		down := fs.Bool("down", false, "reverte a última migration")
		if err := fs.Parse(os.Args[2:]); err != nil {
			return err
		}
		return migrate(ctx, db, *down)
	case "stats":
		return stats(ctx, db)
	case "prune":
		return prune(ctx, db)
	default:
		usage()
		return fmt.Errorf("subcomando desconhecido: %s", os.Args[1])
	}
}

func migrate(ctx context.Context, db *sql.DB, down bool) error {
	goose.SetBaseFS(migrations.FS)
	if err := goose.SetDialect("postgres"); err != nil {
		return err
	}
	if down {
		return goose.DownContext(ctx, db, ".")
	}
	if err := goose.UpContext(ctx, db, "."); err != nil {
		return err
	}
	fmt.Println("migrations aplicadas")
	return nil
}

func stats(ctx context.Context, db *sql.DB) error {
	var products, prices, semEAN int
	if err := db.QueryRowContext(ctx, `select count(*) from products`).Scan(&products); err != nil {
		return err
	}
	if err := db.QueryRowContext(ctx, `select count(*) from prices`).Scan(&prices); err != nil {
		return err
	}
	if err := db.QueryRowContext(ctx, `select count(*) from products where ean is null`).Scan(&semEAN); err != nil {
		return err
	}
	var orfaos int
	if err := db.QueryRowContext(ctx, orphansCount).Scan(&orfaos); err != nil {
		return err
	}
	fmt.Printf("produtos: %d (%d sem EAN)\nprecos:   %d\nórfãos:   %d\n\n", products, semEAN, prices, orfaos)

	if err := statsPorLoja(ctx, db); err != nil {
		return err
	}

	rows, err := db.QueryContext(ctx,
		`select store_id, status, items_found, requests_made, coalesce(to_char(finished_at,'YYYY-MM-DD HH24:MI'),'-')
		   from collection_runs order by id desc limit 5`)
	if err != nil {
		return err
	}
	defer rows.Close()

	fmt.Println("últimas execuções:")
	for rows.Next() {
		var store, status, when string
		var items, reqs int
		if err := rows.Scan(&store, &status, &items, &reqs, &when); err != nil {
			return err
		}
		fmt.Printf("  %-18s %-8s itens=%-6d req=%-4d %s\n", store, status, items, reqs, when)
	}
	return rows.Err()
}

// Um produto fica órfão quando uma regra de identidade muda: o upsert de prices
// reaponta o product_id para a linha nova e a antiga deixa de ter preço.
const orphansCount = `select count(*) from products p
	where not exists (select 1 from prices where prices.product_id = p.id)`

// prune apaga produtos que nenhum preço referencia. É idempotente e só remove o
// que não aparece em nenhum mercado.
func prune(ctx context.Context, db *sql.DB) error {
	res, err := db.ExecContext(ctx, `delete from products p
		where not exists (select 1 from prices where prices.product_id = p.id)`)
	if err != nil {
		return fmt.Errorf("apagar órfãos: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	fmt.Printf("órfãos apagados: %d\n", n)
	return nil
}

// statsPorLoja mostra quanto cada mercado tem e quantos produtos aparecem em
// mais de um — que é o número que diz se a comparação funciona.
func statsPorLoja(ctx context.Context, db *sql.DB) error {
	rows, err := db.QueryContext(ctx,
		`select s.id, count(p.id) from stores s left join prices p on p.store_id = s.id group by s.id order by s.id`)
	if err != nil {
		return err
	}
	defer rows.Close()

	fmt.Println("preços por loja:")
	for rows.Next() {
		var store string
		var n int
		if err := rows.Scan(&store, &n); err != nil {
			return err
		}
		fmt.Printf("  %-18s %d\n", store, n)
	}
	if err := rows.Err(); err != nil {
		return err
	}

	var emComum int
	if err := db.QueryRowContext(ctx,
		`select count(*) from (select product_id from prices group by product_id
		  having count(distinct store_id) > 1) x`).Scan(&emComum); err != nil {
		return err
	}
	fmt.Printf("em mais de uma loja: %d\n\n", emComum)
	return nil
}
