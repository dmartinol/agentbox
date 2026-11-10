package web

import "embed"

//go:embed all:dist
var assets embed.FS

// Assets returns the compiled single-page application. The build output is
// committed so `go install` still produces a complete GUI binary.
func Assets() embed.FS { return assets }
