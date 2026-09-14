// Package collect é a espinha comum dos workers de coleta: pipeline, guarda de
// sanidade, dump bruto, cliente HTTP e linha de comando.
//
// Um worker implementa Source — descobrir seções e paginar cada uma, já
// mapeando para domain.Item — e chama Main. Tudo que não depende do mercado, e
// que custou caro acertar, mora aqui uma vez só.
package collect

import (
	"context"

	"github.com/MatheusMeloAntiquera/precos-core/domain"
)

// Section é uma divisão do catálogo que a fonte sabe paginar: uma seção no
// Confiança, um departamento na VipCommerce.
type Section struct {
	ID   string
	Name string
}

// Page é uma resposta da API do mercado, com o corpo exato preservado para o
// dump e os itens já mapeados.
type Page struct {
	// Key identifica a página no dump: "<seção>-<n>". O pipeline prefixa a data
	// e acrescenta .json.gz. Precisa ser determinística, para que re-executar no
	// mesmo dia sobrescreva em vez de acumular.
	Key string

	// Raw é o corpo da resposta como veio, antes de qualquer normalização.
	Raw []byte

	// Items são os itens mapeados desta página.
	Items []domain.Item

	// Skipped conta itens descartados de propósito (ex.: pesáveis).
	Skipped int
}

// Source é o que cada mercado implementa.
type Source interface {
	// Sections devolve as divisões do catálogo a varrer.
	Sections(ctx context.Context) ([]Section, error)

	// IterPages pagina uma seção inteira, chamando fn para cada página.
	IterPages(ctx context.Context, s Section, fn func(Page) error) error

	// Requests conta as requisições feitas, para o registro da execução.
	Requests() int
}
