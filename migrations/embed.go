package migrations

import "embed"

// FS carrega as migrations no binário, para que o precosctl seja autocontido.
//
//go:embed *.sql
var FS embed.FS
