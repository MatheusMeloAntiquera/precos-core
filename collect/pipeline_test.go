package collect

import (
	"context"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/MatheusMeloAntiquera/precos-core/domain"
)

type fakeSource struct {
	sections       []Section
	pages          map[string][]Page
	sectionsCalled bool
}

func (f *fakeSource) Sections(context.Context) ([]Section, error) {
	f.sectionsCalled = true
	return f.sections, nil
}

func (f *fakeSource) IterPages(_ context.Context, s Section, fn func(Page) error) error {
	for _, p := range f.pages[s.ID] {
		if err := fn(p); err != nil {
			return err
		}
	}
	return nil
}

func (f *fakeSource) Requests() int { return 7 }

type recordingStore struct{ keys []string }

func (r *recordingStore) Put(_ context.Context, key string, _ io.Reader) error {
	r.keys = append(r.keys, key)
	return nil
}

func item(sku, ean string) domain.Item {
	return domain.Item{StoreSKUKey: sku, EAN: ean, Name: "Produto " + sku, Price: domain.Price{Cents: 100}}
}

func quietLog() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func TestRunDryRunDedupeEDump(t *testing.T) {
	src := &fakeSource{
		sections: []Section{{ID: "bebidas"}, {ID: "ofertas"}},
		pages: map[string][]Page{
			"bebidas": {
				{Key: "bebidas-1", Raw: []byte(`{}`), Items: []domain.Item{item("1", "7891149104932"), item("2", "1113")}},
				{Key: "bebidas-2", Raw: []byte(`{}`), Items: []domain.Item{item("3", "")}, Skipped: 2},
			},
			// O mesmo SKU em duas seções conta duas vezes antes do dedupe e uma depois.
			"ofertas": {
				{Key: "ofertas-1", Raw: []byte(`{}`), Items: []domain.Item{item("1", "7891149104932")}, Skipped: 1},
			},
		},
	}
	raw := &recordingStore{}

	// Dry-run com Ingester nil: tocar no banco causaria panic, e é essa a asserção.
	sum, err := Run(context.Background(), Deps{Source: src, Raw: raw, Log: quietLog(), StoreID: "loja", DryRun: true})
	if err != nil {
		t.Fatal(err)
	}

	if sum.Sections != 2 || sum.ItemsRaw != 4 || sum.Items != 3 {
		t.Errorf("seções=%d brutos=%d itens=%d, esperava 2/4/3", sum.Sections, sum.ItemsRaw, sum.Items)
	}
	if sum.Skipped != 3 {
		t.Errorf("descartados = %d, esperava 3", sum.Skipped)
	}
	// "1113" é PLU e "" é vazio: os dois contam como sem EAN.
	if sum.WithoutEAN != 2 {
		t.Errorf("sem EAN = %d, esperava 2", sum.WithoutEAN)
	}
	if sum.Requests != 7 {
		t.Errorf("requisições = %d, esperava as 7 da fonte", sum.Requests)
	}
	if sum.Sample[0].StoreSKUKey != "1" {
		t.Errorf("amostra deveria seguir a ordem de chegada, veio %q primeiro", sum.Sample[0].StoreSKUKey)
	}

	today := time.Now().Format("2006-01-02")
	want := []string{today + "/bebidas-1.json.gz", today + "/bebidas-2.json.gz", today + "/ofertas-1.json.gz"}
	if strings.Join(raw.keys, ",") != strings.Join(want, ",") {
		t.Errorf("chaves do dump = %v, esperava %v", raw.keys, want)
	}
}

func TestRunReprovaColetaVazia(t *testing.T) {
	src := &fakeSource{
		sections: []Section{{ID: "a"}},
		pages:    map[string][]Page{"a": {{Key: "a-1", Skipped: 20}}},
	}
	_, err := Run(context.Background(), Deps{Source: src, Raw: Discard{}, Log: quietLog(), StoreID: "loja", DryRun: true})
	if err == nil || !strings.Contains(err.Error(), "0 itens") {
		t.Errorf("esperava reprovar coleta vazia, veio %v", err)
	}
}

func TestRunCategoriasNaoDescobreSecoes(t *testing.T) {
	src := &fakeSource{
		sections: []Section{{ID: "nao-usar"}},
		pages:    map[string][]Page{"7": {{Key: "7-1", Items: []domain.Item{item("1", "")}}}},
	}
	sum, err := Run(context.Background(), Deps{
		Source: src, Raw: Discard{}, Log: quietLog(), StoreID: "loja", DryRun: true, Categories: []string{"7"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if src.sectionsCalled {
		t.Error("com --category, Sections() não deveria ser chamado")
	}
	if sum.Sections != 1 || sum.Items != 1 {
		t.Errorf("seções=%d itens=%d, esperava 1/1", sum.Sections, sum.Items)
	}
}

func TestSplitList(t *testing.T) {
	if got := SplitList(" 7, ,90 ,"); strings.Join(got, "|") != "7|90" {
		t.Errorf("SplitList = %q", got)
	}
	if SplitList("  ") != nil {
		t.Error("lista em branco deveria dar nil")
	}
}
