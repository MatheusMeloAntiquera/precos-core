package domain

import "strings"

// NormalizeEAN devolve o GTIN canônico, ou "" quando o código não é GTIN válido.
//
// Mercados mandam no campo de código de barras coisas que não são EAN: o
// Barracão manda o PLU interno dos pesáveis ("1113", "6088"). Aceitos como EAN,
// viram match_key compartilhada e fundem produtos diferentes de mercados
// diferentes na mesma linha. Por isso a validação mora no core, e não em cada
// worker.
//
// Códigos de 11 dígitos são UPC-A que perdeu o zero à esquerda em algum sistema
// que guardou o código como número: em 13/09/2026, 223 de 223 deles (143 do
// Confiança, 80 do Barracão) passavam no dígito verificador com o zero de volta
// — ao acaso seriam ~10%. Os prefixos são de marcas importadas (011210 é
// Tabasco, 080432 é Diageo).
//
// GTIN-12 (UPC-A) ganha um zero à esquerda e GTIN-14 que começa com zero perde
// o zero, para que UPC e EAN do mesmo item casem. Zeros à esquerda não mudam o
// dígito verificador.
func NormalizeEAN(s string) string {
	s = strings.TrimSpace(s)
	if len(s) == 11 {
		s = "0" + s
	}
	switch len(s) {
	case 8, 12, 13, 14:
	default:
		return ""
	}
	if !validCheckDigit(s) {
		return ""
	}
	switch {
	case len(s) == 12:
		return "0" + s
	case len(s) == 14 && s[0] == '0':
		return s[1:]
	}
	return s
}

// validCheckDigit confere o dígito verificador GTIN: da direita para a
// esquerda, sem o último dígito, pesos alternados 3, 1, 3, 1...
func validCheckDigit(s string) bool {
	sum, weight := 0, 3
	for i := len(s) - 2; i >= 0; i-- {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
		sum += int(s[i]-'0') * weight
		weight = 4 - weight
	}
	last := s[len(s)-1]
	if last < '0' || last > '9' {
		return false
	}
	return (10-sum%10)%10 == int(last-'0')
}
