package collect

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/MatheusMeloAntiquera/precos-core/domain"
	"github.com/MatheusMeloAntiquera/precos-core/ingest"
)

// Deps são as dependências de uma execução.
type Deps struct {
	Source   Source
	Raw      Store
	Ingester *ingest.Ingester // nil em dry-run
	Log      *slog.Logger
	StoreID  string
	DryRun   bool

	// Categories restringe a coleta. Vazio significa todas as seções.
	Categories []string
}

// Summary é o resultado de uma execução.
type Summary struct {
	Sections   int
	Requests   int
	ItemsRaw   int // antes do dedupe entre seções
	Items      int // depois do dedupe
	Skipped    int // descartados de propósito pela fonte
	WithoutEAN int // itens cujo código não é GTIN válido
	Products   int64
	Prices     int64
	Duration   time.Duration
	Sample     []domain.Item
}

// Run executa a coleta inteira: seções → páginas → dump → dedupe → sanidade →
// gravação. A run é aberta antes e sempre fechada, inclusive em erro.
func Run(ctx context.Context, d Deps) (Summary, error) {
	start := time.Now()

	var runID int64
	if !d.DryRun {
		id, err := d.Ingester.StartRun(ctx, d.StoreID, len(d.Categories) > 0)
		if err != nil {
			return Summary{}, err
		}
		runID = id
	}

	sum, err := d.collect(ctx)
	sum.Requests = d.Source.Requests()
	sum.Duration = time.Since(start)

	if d.DryRun {
		return sum, err
	}

	// A run precisa fechar mesmo em erro: run "running" órfã atrapalha a guarda
	// de sanidade da próxima execução. WithoutCancel porque, num Ctrl+C, o ctx
	// da coleta já está cancelado e o update seria recusado.
	closeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
	defer cancel()
	if ferr := d.Ingester.FinishRun(closeCtx, runID, sum.Items, sum.Requests, err); ferr != nil && err == nil {
		return sum, ferr
	}
	return sum, err
}

func (d Deps) collect(ctx context.Context) (Summary, error) {
	var sum Summary

	sections, err := d.sections(ctx)
	if err != nil {
		return sum, err
	}
	sum.Sections = len(sections)
	d.Log.Info("seções a coletar", "total", len(sections))

	// O mesmo produto aparece em várias seções. O dedupe é por SKU, e a ordem de
	// chegada é guardada para a amostra sair determinística.
	deduped := make(map[string]domain.Item, 16000)
	order := make([]string, 0, 16000)
	today := time.Now().Format("2006-01-02")

	for _, section := range sections {
		if err := ctx.Err(); err != nil {
			return sum, err
		}

		sectionStart := time.Now()
		before := len(deduped)
		var sectionItems, sectionSkipped, pages int

		err := d.Source.IterPages(ctx, section, func(p Page) error {
			pages++
			key := today + "/" + p.Key + ".json.gz"
			if err := d.Raw.Put(ctx, key, bytes.NewReader(p.Raw)); err != nil {
				// Perder o backup do dia é ruim; perder a coleta é pior.
				d.Log.Warn("falha ao gravar dump bruto", "key", key, "erro", err)
			}

			sectionSkipped += p.Skipped
			for _, it := range p.Items {
				sectionItems++
				if _, seen := deduped[it.StoreSKUKey]; !seen {
					order = append(order, it.StoreSKUKey)
				}
				deduped[it.StoreSKUKey] = it
			}
			return nil
		})
		if err != nil {
			return sum, fmt.Errorf("seção %s: %w", section.ID, err)
		}

		sum.ItemsRaw += sectionItems
		sum.Skipped += sectionSkipped
		d.Log.Info("seção coletada",
			"secao", section.ID,
			"nome", section.Name,
			"paginas", pages,
			"itens", sectionItems,
			"novos", len(deduped)-before,
			"descartados", sectionSkipped,
			"duracao", time.Since(sectionStart).Round(time.Millisecond).String())
	}

	items := make([]domain.Item, 0, len(order))
	for _, key := range order {
		it := deduped[key]
		items = append(items, it)
		if domain.NormalizeEAN(it.EAN) == "" {
			sum.WithoutEAN++
		}
	}
	sum.Items = len(items)
	sum.Sample = sampleOf(items, 5)

	if err := d.checkSanity(ctx, sum.Items); err != nil {
		return sum, err
	}

	if d.DryRun {
		d.Log.Info("dry-run: nada gravado", "itens", sum.Items)
		return sum, nil
	}

	res, err := d.Ingester.Submit(ctx, d.StoreID, items)
	if err != nil {
		return sum, err
	}
	sum.Products, sum.Prices = res.Products, res.Prices
	return sum, nil
}

func (d Deps) sections(ctx context.Context) ([]Section, error) {
	if len(d.Categories) > 0 {
		sections := make([]Section, 0, len(d.Categories))
		for _, id := range d.Categories {
			sections = append(sections, Section{ID: id, Name: id})
		}
		return sections, nil
	}
	return d.Source.Sections(ctx)
}

func sampleOf(items []domain.Item, n int) []domain.Item {
	if len(items) < n {
		n = len(items)
	}
	return append([]domain.Item(nil), items[:n]...)
}
