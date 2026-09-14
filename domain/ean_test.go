package domain

import "testing"

func TestNormalizeEAN(t *testing.T) {
	cases := []struct {
		nome, in, want string
	}{
		{"EAN-13", "7891149104932", "7891149104932"},
		{"EAN-8", "78905351", "78905351"},
		{"UPC-A vira EAN-13", "012345678905", "0012345678905"},
		{"UPC-A sem o zero à esquerda", "12345678905", "0012345678905"},
		{"Tabasco no Confiança, sem o zero", "11210006508", "0011210006508"},
		{"11 dígitos com verificador errado", "12345678904", ""},
		{"GTIN-14 com zero vira EAN-13", "00012345678905", "0012345678905"},
		{"espaços em volta", " 7891149104932 ", "7891149104932"},
		{"PLU de pesável do Barracão", "1113", ""},
		{"dígito verificador errado", "7891149104933", ""},
		{"letras", "78911491049AB", ""},
		{"vazio", "", ""},
	}
	for _, c := range cases {
		t.Run(c.nome, func(t *testing.T) {
			if got := NormalizeEAN(c.in); got != c.want {
				t.Errorf("NormalizeEAN(%q) = %q, esperava %q", c.in, got, c.want)
			}
		})
	}
}
