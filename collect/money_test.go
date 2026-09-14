package collect

import (
	"encoding/json"
	"testing"
)

func TestToCents(t *testing.T) {
	cases := []struct {
		in   string
		want int64
	}{
		{"1.97", 197},
		{"0.29", 29},  // ParseFloat*100 truncado daria 28
		{"1.13", 113}, // idem: daria 112
		{"1.9", 190},
		{"12", 1200},
		{"0", 0},
		{"", 0}, // salePrice null vira string vazia
		{"0.05", 5},
		{"29.9", 2990},
		{"1.999", 200}, // arredonda a terceira casa, com carry
		{"9.995", 1000},
		{"-1.50", -150},
	}
	for _, c := range cases {
		got, err := ToCents(json.Number(c.in))
		if err != nil {
			t.Errorf("ToCents(%q): %v", c.in, err)
			continue
		}
		if got != c.want {
			t.Errorf("ToCents(%q) = %d, esperava %d", c.in, got, c.want)
		}
	}

	if _, err := ToCents(json.Number("abc")); err == nil {
		t.Error("esperava erro para valor não numérico")
	}
}

// A VipCommerce manda preco como string ("9.99") e preco_original como número
// (0). Com UseNumber os dois viram json.Number e convertem igual.
func TestDecodeJSONAceitaStringENumero(t *testing.T) {
	var v struct {
		Preco         json.Number `json:"preco"`
		PrecoOriginal json.Number `json:"preco_original"`
	}
	if err := DecodeJSON([]byte(`{"preco":"9.99","preco_original":0}`), &v); err != nil {
		t.Fatal(err)
	}
	if c, _ := ToCents(v.Preco); c != 999 {
		t.Errorf("preco = %d centavos, esperava 999", c)
	}
	if c, _ := ToCents(v.PrecoOriginal); c != 0 {
		t.Errorf("preco_original = %d centavos, esperava 0", c)
	}
}
