package collect

import "testing"

func TestCheckDrop(t *testing.T) {
	cases := []struct {
		nome     string
		items    int
		last     int
		querErro bool
	}{
		{"coleta vazia sempre reprova", 0, 0, true},
		{"coleta vazia reprova mesmo com histórico", 0, 15000, true},
		{"primeira execução passa", 15182, 0, false},
		{"estável passa", 15182, 15100, false},
		{"crescimento passa", 16000, 15100, false},
		{"queda pequena passa", 14000, 15000, false},
		{"queda de 20% está no limite", 12000, 15000, false},
		{"queda acima de 20% reprova", 11999, 15000, true},
		{"catálogo pela metade reprova", 7500, 15000, true},
	}

	for _, c := range cases {
		t.Run(c.nome, func(t *testing.T) {
			err := checkDrop(c.items, c.last)
			if c.querErro && err == nil {
				t.Errorf("checkDrop(%d, %d) passou, esperava reprovar", c.items, c.last)
			}
			if !c.querErro && err != nil {
				t.Errorf("checkDrop(%d, %d) reprovou: %v", c.items, c.last, err)
			}
		})
	}
}
