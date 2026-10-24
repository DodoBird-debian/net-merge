package main

import (
	"bytes"
	"flag"
	"fmt"
	"net-merge/internal/i18n"
	"strings"
)

func newFlags(name string, handling flag.ErrorHandling) *flag.FlagSet {
	fs := flag.NewFlagSet(name, handling)
	fs.Usage = func() {
		out := fs.Output()
		fmt.Fprintf(out, i18n.T("Usage: net-merge %s [options]\n"), name)
		if !i18n.RTL() {
			fs.PrintDefaults()
			return
		}
		var defaults bytes.Buffer
		fs.SetOutput(&defaults)
		fs.PrintDefaults()
		fs.SetOutput(out)
		fmt.Fprint(out, strings.ReplaceAll(defaults.String(), "(default ", i18n.T("(default ")))
	}
	return fs
}
