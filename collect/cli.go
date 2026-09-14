package collect

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/MatheusMeloAntiquera/precos-core/ingest"
)

// Builder monta a fonte do mercado a partir da configuração.
//
// Deve ser barato e não fazer requisições: conexões e logins pertencem a
// Sections, para que uma falha ali fique registrada como run failed.
type Builder func(ctx context.Context, base Base, log *slog.Logger) (Source, error)

// Main é o ponto de entrada de um worker. Sai com código != 0 quando a
// execução falha — é o que permite agendar sem que falha passe despercebida.
func Main(name, usage, defaultStoreID string, build Builder) {
	if err := run(os.Args[1:], name, usage, defaultStoreID, build); err != nil {
		fmt.Fprintln(os.Stderr, "erro:", err)
		os.Exit(1)
	}
}

func run(args []string, name, usage, defaultStoreID string, build Builder) error {
	if len(args) == 0 {
		fmt.Println(usage)
		return nil
	}
	switch args[0] {
	case "run":
		return cmdRun(args[1:], name, defaultStoreID, build)
	case "-h", "--help", "help":
		fmt.Println(usage)
		return nil
	default:
		fmt.Println(usage)
		return fmt.Errorf("subcomando desconhecido: %s", args[0])
	}
}

func cmdRun(args []string, name, defaultStoreID string, build Builder) error {
	fs := flag.NewFlagSet("run", flag.ExitOnError)
	dryRun := fs.Bool("dry-run", false, "coleta e valida sem gravar no banco")
	category := fs.String("category", "", "coleta apenas estas seções (separadas por vírgula)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	base, err := LoadBase(defaultStoreID)
	if err != nil {
		return err
	}

	// Ctrl+C deve encerrar limpo, com a run fechada como failed.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Logs em JSON no stderr; o resumo legível vai para o stdout.
	logger := slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo})).
		With("worker", name, "loja", base.StoreID)

	deps := Deps{
		Raw:        Discard{},
		Log:        logger,
		StoreID:    base.StoreID,
		DryRun:     *dryRun,
		Categories: SplitList(*category),
	}

	// O banco é conferido antes de montar a fonte: descobrir que o Postgres está
	// fora depois de centenas de requisições é desperdiçar a carga no site.
	if !*dryRun {
		if err := base.RequireDatabase(); err != nil {
			return err
		}
		pool, err := pgxpool.New(ctx, base.DatabaseURL)
		if err != nil {
			return fmt.Errorf("conectar no Postgres: %w", err)
		}
		defer pool.Close()
		if err := pool.Ping(ctx); err != nil {
			return fmt.Errorf("conectar no Postgres: %w", err)
		}
		deps.Ingester = ingest.New(pool)
		deps.Raw = NewLocal(base.RawDir)
	}

	src, err := build(ctx, base, logger)
	if err != nil {
		return err
	}
	deps.Source = src

	logger.Info("iniciando coleta", "dry_run", *dryRun, "delay", base.RequestDelay.String())

	sum, runErr := Run(ctx, deps)
	printSummary(sum, *dryRun)
	return runErr
}

// SplitList separa uma lista por vírgulas, ignorando espaços e itens vazios.
func SplitList(s string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func printSummary(s Summary, dryRun bool) {
	fmt.Printf("\nseções:      %d\n", s.Sections)
	fmt.Printf("requisições: %d\n", s.Requests)
	fmt.Printf("itens:       %d (%d com repetição entre seções)\n", s.Items, s.ItemsRaw)
	fmt.Printf("descartados: %d\n", s.Skipped)
	fmt.Printf("sem EAN:     %d\n", s.WithoutEAN)
	fmt.Printf("duração:     %s\n", s.Duration.Round(time.Millisecond))

	if dryRun {
		fmt.Println("\ndry-run: nada gravado")
	} else {
		fmt.Printf("produtos:    %d gravados\nprecos:      %d gravados\n", s.Products, s.Prices)
	}

	if len(s.Sample) > 0 {
		fmt.Println("\namostra:")
		for _, it := range s.Sample {
			ean := it.EAN
			if ean == "" {
				ean = "(sem EAN)"
			}
			fmt.Printf("  %-14s %-52s R$ %6.2f  %s\n",
				ean, truncate(it.Name, 52), float64(it.Price.Cents)/100, it.ImageURL)
		}
	}
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}
