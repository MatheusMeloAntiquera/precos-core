package collect

import (
	"compress/gzip"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPutGravaGzipValido(t *testing.T) {
	dir := t.TempDir()
	s := NewLocal(dir)
	key := "2026-09-10/s_bebidas-000000.json.gz"
	payload := `{"totalResults":1,"items":[{"id":"1"}]}`

	if err := s.Put(context.Background(), key, strings.NewReader(payload)); err != nil {
		t.Fatal(err)
	}

	f, err := os.Open(filepath.Join(dir, key))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	// Se o Close do gzip.Writer for ignorado, o rodapé não é gravado e a
	// leitura falha aqui — que é o ponto do teste.
	zr, err := gzip.NewReader(f)
	if err != nil {
		t.Fatalf("gzip inválido: %v", err)
	}
	got, err := io.ReadAll(zr)
	if err != nil {
		t.Fatalf("ler gzip: %v", err)
	}
	if string(got) != payload {
		t.Errorf("conteúdo = %q, esperava %q", got, payload)
	}
}

// Chave determinística: re-executar no mesmo dia sobrescreve em vez de
// acumular lixo.
func TestPutSobrescreve(t *testing.T) {
	dir := t.TempDir()
	s := NewLocal(dir)
	key := "2026-09-10/s_bebidas-000000.json.gz"

	for _, payload := range []string{"primeiro", "segundo"} {
		if err := s.Put(context.Background(), key, strings.NewReader(payload)); err != nil {
			t.Fatal(err)
		}
	}

	entries, err := os.ReadDir(filepath.Join(dir, "2026-09-10"))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Errorf("%d arquivos, esperava 1", len(entries))
	}
}
