package main

import (
	"bufio"
	"os"
	"strings"
)

// loadDotEnv reads KEY=VALUE lines from .env into the environment.
//
// The alternative is telling a reviewer to export a variable in the shell that
// happens to be running the binary, which fails silently the first time they
// open a second terminal. A file next to the repository is the one place a key
// can live where both `make demo` and an editor's run button find it.
//
// It never overwrites a variable that is already set, so an exported key still
// wins over a stale file, and .env is in .gitignore so the key cannot be
// committed by accident.
//
// This is a deliberately small parser: no export prefixes, no interpolation,
// no multi-line values. A key is one line.
func loadDotEnv(path string) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimPrefix(line, "export ")
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		k = strings.TrimSpace(k)
		v = strings.TrimSpace(v)
		// Quotes are stripped because a key pasted from a dashboard often
		// arrives wrapped in them, and a quoted key fails authentication in a
		// way whose error message does not mention quotes.
		if len(v) >= 2 && (v[0] == '"' && v[len(v)-1] == '"' || v[0] == '\'' && v[len(v)-1] == '\'') {
			v = v[1 : len(v)-1]
		}
		if k == "" || os.Getenv(k) != "" {
			continue
		}
		_ = os.Setenv(k, v)
	}
}
