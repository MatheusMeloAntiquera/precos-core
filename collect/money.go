package collect

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
)

// ToCents converte o número JSON em centavos, a partir da string.
//
// Não use ParseFloat seguido de *100: 573 dos 9999 preços de dois decimais
// (5,7%) truncam errado, porque float64 não representa a fração exata. 0.29
// vira 28 centavos e 1.13 vira 112. Um centavo errado em 15 mil produtos passa
// despercebido até a comparação de preço ficar estranha.
func ToCents(n json.Number) (int64, error) {
	s := strings.TrimSpace(n.String())
	if s == "" || s == "null" {
		return 0, nil
	}

	negative := strings.HasPrefix(s, "-")
	s = strings.TrimPrefix(s, "-")

	intPart, frac, _ := strings.Cut(s, ".")
	if intPart == "" {
		intPart = "0"
	}
	if !isDigits(intPart) || !isDigits(frac) {
		return 0, fmt.Errorf("valor não numérico: %q", n.String())
	}

	var cents int64
	for _, r := range intPart {
		cents = cents*10 + int64(r-'0')
		if cents > 1<<40 {
			return 0, fmt.Errorf("valor fora de faixa: %q", n.String())
		}
	}
	cents *= 100

	switch {
	case len(frac) == 0:
	case len(frac) == 1:
		cents += int64(frac[0]-'0') * 10
	default:
		cents += int64(frac[0]-'0')*10 + int64(frac[1]-'0')
		if len(frac) > 2 && frac[2] >= '5' {
			cents++ // arredonda a terceira casa; o carry sai de graça no inteiro
		}
	}

	if negative {
		cents = -cents
	}
	return cents, nil
}

func isDigits(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// DecodeJSON desserializa mantendo os números como json.Number: transformá-los
// em float64 introduz erro de arredondamento antes da conversão para centavos.
func DecodeJSON(raw []byte, v any) error {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	return dec.Decode(v)
}
