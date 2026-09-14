package domain

import "testing"

func TestMatchKey(t *testing.T) {
	// Com EAN, a identidade é compartilhada entre mercados.
	comEAN := Item{StoreSKUKey: "966533", EAN: "78933873"}
	if got := comEAN.MatchKey("confianca-bauru"); got != "78933873" {
		t.Errorf("MatchKey = %q, esperava o EAN", got)
	}

	// Sem EAN (itens de peso), a identidade fica restrita ao mercado — que é o
	// problema de casamento adiado para o segundo worker.
	semEAN := Item{StoreSKUKey: "12345"}
	if got := semEAN.MatchKey("confianca-bauru"); got != "confianca-bauru:12345" {
		t.Errorf("MatchKey = %q, esperava confianca-bauru:12345", got)
	}

	// EAN em branco não pode virar chave compartilhada.
	branco := Item{StoreSKUKey: "12345", EAN: "   "}
	if got := branco.MatchKey("loja"); got != "loja:12345" {
		t.Errorf("MatchKey = %q com EAN em branco", got)
	}

	// PLU no campo de código de barras também não: dois mercados com o mesmo
	// PLU para produtos diferentes virariam a mesma linha.
	plu := Item{StoreSKUKey: "1826", EAN: "1113"}
	if got := plu.MatchKey("barracao-bauru"); got != "barracao-bauru:1826" {
		t.Errorf("MatchKey = %q com PLU, esperava barracao-bauru:1826", got)
	}

	// UPC-A e EAN-13 do mesmo item precisam dar a mesma chave.
	upc := Item{StoreSKUKey: "1", EAN: "012345678905"}
	if got := upc.MatchKey("loja"); got != "0012345678905" {
		t.Errorf("MatchKey = %q com UPC-A, esperava 0012345678905", got)
	}
}

func TestValidate(t *testing.T) {
	ok := Item{StoreSKUKey: "1", Name: "Produto"}
	if err := ok.Validate(); err != nil {
		t.Errorf("item válido reprovado: %v", err)
	}
	for nome, it := range map[string]Item{
		"sem sku":        {Name: "Produto"},
		"sem nome":       {StoreSKUKey: "1"},
		"preço negativo": {StoreSKUKey: "1", Name: "P", Price: Price{Cents: -1}},
	} {
		if err := it.Validate(); err == nil {
			t.Errorf("%s: esperava erro", nome)
		}
	}
}
