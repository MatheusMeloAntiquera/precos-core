package collect

import (
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// Store é o destino do JSON bruto, gravado exatamente como veio da API.
//
// Quando aparecer um bug no mapper — e vai aparecer — dá para reprocessar os
// dias já coletados em vez de perder o dado. Local hoje; S3/GCS quando o worker
// for para a nuvem, sem que o pipeline precise mudar.
type Store interface {
	Put(ctx context.Context, key string, r io.Reader) error
}

// Local grava em disco, comprimido.
type Local struct{ dir string }

func NewLocal(dir string) *Local { return &Local{dir: dir} }

// Put grava <dir>/<key>. A chave é determinística, então re-executar no mesmo
// dia sobrescreve em vez de acumular lixo.
func (l *Local) Put(ctx context.Context, key string, r io.Reader) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	path := filepath.Join(l.dir, key)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("criar diretório do dump: %w", err)
	}

	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("criar %s: %w", path, err)
	}
	defer f.Close()

	zw := gzip.NewWriter(f)
	if _, err := io.Copy(zw, r); err != nil {
		zw.Close()
		return fmt.Errorf("escrever %s: %w", path, err)
	}
	// O gzip só grava o rodapé no Close: ignorar este erro produz arquivo
	// corrompido que só aparece na hora de reprocessar.
	if err := zw.Close(); err != nil {
		return fmt.Errorf("finalizar %s: %w", path, err)
	}
	return f.Close()
}

// Discard descarta o payload. Usado no --dry-run.
type Discard struct{}

func (Discard) Put(context.Context, string, io.Reader) error { return nil }
