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
