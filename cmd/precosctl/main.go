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
	fmt.Printf("produtos: %d (%d sem EAN)\nprecos:   %d\n\n", products, semEAN, prices)

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
