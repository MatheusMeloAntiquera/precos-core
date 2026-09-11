// Package domain define o contrato entre os workers de coleta e o banco.
// Um worker produz []Item; nada além disto ele precisa saber sobre o schema.
package domain

import (
	"fmt"
	"strings"
)

// Price guarda valores em centavos. Nunca use float para dinheiro: 5,7% dos
// preços de dois decimais truncam errado ao passar por float64 (0.29 vira 28
// centavos, 1.13 vira 112).
type Price struct {
	Cents     int64 // preço efetivo (o que o cliente paga)
	ListCents int64 // preço "de", quando existir
	ClubCents int64 // preço do clube de descontos, quando existir
}

// Item é um produto anunciado por um mercado, no momento da coleta.
type Item struct {
	StoreSKUKey string // id do SKU no site do mercado
	EAN         string // vazio quando o mercado não expõe código de barras
	Name        string // nome canônico, legível
	NameAtStore string // como aquele mercado escreve
	ImageURL    string
	ProductURL  string
	Unit        string
	Price       Price
	Available   bool
}

// MatchKey é a chave determinística de upsert em products.
//
// O EAN não serve sozinho porque itens de peso (hortifrúti, açougue, padaria)
// vêm sem código de barras. Nesses casos a identidade fica restrita ao mercado
// de origem, que é exatamente o problema de casamento entre catálogos adiado
// para quando houver um segundo mercado.
func (i Item) MatchKey(storeID string) string {
	if ean := strings.TrimSpace(i.EAN); ean != "" {
		return ean
	}
	return storeID + ":" + i.StoreSKUKey
}

// Validate rejeita itens que não podem ser persistidos de forma útil.
func (i Item) Validate() error {
	if strings.TrimSpace(i.StoreSKUKey) == "" {
		return fmt.Errorf("item sem StoreSKUKey")
	}
	if strings.TrimSpace(i.Name) == "" {
		return fmt.Errorf("item %s sem Name", i.StoreSKUKey)
	}
	if i.Price.Cents < 0 || i.Price.ListCents < 0 || i.Price.ClubCents < 0 {
		return fmt.Errorf("item %s com preço negativo", i.StoreSKUKey)
	}
	return nil
}
