package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
)

func newFlagSet(name string) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	return fs
}

func outOK(data any) {
	enc := json.NewEncoder(os.Stdout)
	_ = enc.Encode(map[string]any{"ok": true, "version": Version, "data": data})
	os.Exit(0)
}

func fail(code int, typ, msg, suggestion string) {
	rec := code >= 100 && code < 110
	err := map[string]any{
		"code":        code,
		"type":        typ,
		"message":     msg,
		"recoverable": rec,
	}
	if suggestion != "" {
		err["suggestions"] = []string{suggestion}
	}
	b, _ := json.Marshal(map[string]any{"ok": false, "error": err})
	fmt.Fprintln(os.Stderr, string(b))
	os.Exit(code)
}
