package ingest

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// Status de uma execução de coleta.
const (
	StatusRunning = "running"
	StatusSuccess = "success"
	StatusFailed  = "failed"
)

// StartRun abre uma execução. Toda run precisa ser fechada com FinishRun,
// inclusive em caso de erro: run "running" órfã atrapalha a guarda de sanidade
// da próxima execução.
func (in *Ingester) StartRun(ctx context.Context, storeID string) (int64, error) {
	var id int64
	err := in.pool.QueryRow(ctx,
		`insert into collection_runs (store_id, status) values ($1, $2) returning id`,
		storeID, StatusRunning).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("abrir run: %w", err)
	}
	return id, nil
}

// FinishRun fecha a execução. runErr nil marca sucesso.
func (in *Ingester) FinishRun(ctx context.Context, runID int64, itemsFound, requestsMade int, runErr error) error {
	status := StatusSuccess
	var msg *string
	if runErr != nil {
		status = StatusFailed
		s := runErr.Error()
		msg = &s
	}
	_, err := in.pool.Exec(ctx,
		`update collection_runs
		    set finished_at = now(), status = $2, items_found = $3, requests_made = $4, error = $5
		  where id = $1`,
		runID, status, itemsFound, requestsMade, msg)
	if err != nil {
		return fmt.Errorf("fechar run: %w", err)
	}
	return nil
}

// LastSuccessfulItemCount devolve quantos itens a última coleta bem-sucedida
// daquele mercado encontrou. O segundo retorno é false na primeira execução,
// quando não há histórico com que comparar.
func (in *Ingester) LastSuccessfulItemCount(ctx context.Context, storeID string) (int, bool, error) {
	var n int
	err := in.pool.QueryRow(ctx,
		`select items_found from collection_runs
		  where store_id = $1 and status = $2
		  order by finished_at desc limit 1`,
		storeID, StatusSuccess).Scan(&n)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return 0, false, nil
	case err != nil:
		return 0, false, fmt.Errorf("consultar última run: %w", err)
	}
	return n, true, nil
}
