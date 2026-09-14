package collect

import (
	"context"
	"errors"
	"fmt"
)

// maxDropRatio: uma queda maior que isto frente à última execução bem-sucedida
// reprova a coleta. Protege contra mudança silenciosa na API, que não devolve
// erro — devolve menos dados.
const maxDropRatio = 0.20

// checkSanity reprova a coleta antes de gravar qualquer coisa.
func (d Deps) checkSanity(ctx context.Context, items int) error {
	if d.DryRun || d.Ingester == nil || len(d.Categories) > 0 {
		// Coleta parcial não é comparável com a última coleta completa.
		return checkDrop(items, 0)
	}
	last, _, err := d.Ingester.LastSuccessfulItemCount(ctx, d.StoreID)
	if err != nil {
		return err
	}
	return checkDrop(items, last)
}

// checkDrop é a regra de sanidade, isolada do banco para poder ser testada.
//
// Uma mudança silenciosa na API não devolve erro: devolve menos dados. Sem
// esta checagem um catálogo pela metade sobrescreve o bom, e a falha só
// aparece semanas depois.
func checkDrop(items, last int) error {
	if items == 0 {
		return errors.New("coleta trouxe 0 itens")
	}
	if last <= 0 {
		return nil // primeira execução: não há com o que comparar
	}
	if float64(items) < float64(last)*(1-maxDropRatio) {
		return fmt.Errorf("coleta trouxe %d itens, queda de mais de %.0f%% frente aos %d da última execução",
			items, maxDropRatio*100, last)
	}
	return nil
}
