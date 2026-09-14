package collect

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"time"
)

// Base é a configuração comum a todo worker. Variáveis específicas do mercado
// ficam no próprio worker, lidas com EnvOr.
type Base struct {
	DatabaseURL  string
	StoreID      string
	RequestDelay time.Duration
	RawDir       string
}

// LoadBase lê a configuração comum, aplicando antes o .env do diretório atual.
func LoadBase(defaultStoreID string) (Base, error) {
	LoadDotEnv(".env")

	b := Base{
		DatabaseURL: os.Getenv("DATABASE_URL"),
		StoreID:     EnvOr("STORE_ID", defaultStoreID),
		RawDir:      EnvOr("RAW_DIR", "./raw"),
	}

	delay, err := time.ParseDuration(EnvOr("REQUEST_DELAY", "1s"))
	if err != nil {
		return b, fmt.Errorf("REQUEST_DELAY inválido: %w", err)
	}
	b.RequestDelay = delay

	if b.StoreID == "" {
		return b, fmt.Errorf("STORE_ID vazio")
	}
	return b, nil
}

// RequireDatabase valida o que só é necessário quando a coleta vai gravar.
func (b Base) RequireDatabase() error {
	if b.DatabaseURL == "" {
		return fmt.Errorf("DATABASE_URL não definida")
	}
	return nil
}

// EnvOr devolve a variável de ambiente, ou fallback quando vazia.
func EnvOr(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

// LoadDotEnv aplica um arquivo .env, se existir. Variáveis já definidas no
// ambiente têm precedência sobre o arquivo.
func LoadDotEnv(path string) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		if _, exists := os.LookupEnv(key); exists {
			continue
		}
		os.Setenv(key, strings.Trim(strings.TrimSpace(value), `"'`))
	}
}
