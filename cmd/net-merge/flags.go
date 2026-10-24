package main

import (
	"flag"
	"fmt"
	"net-merge/internal/i18n"
)

func newFlags(name string, handling flag.ErrorHandling) *flag.FlagSet {
	fs := flag.NewFlagSet(name, handling)
	fs.Usage = func() { fmt.Fprintf(fs.Output(), i18n.T("Usage: net-merge %s [options]\n"), name); fs.PrintDefaults() }
	return fs
}
