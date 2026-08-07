// Command fakeclaude stands in for the real `claude` binary in clau's
// integration tests. It records its argv and its ANTHROPIC_* env vars to the
// file named by $CLAU_TEST_OUT, using a fixed tab-separated wire format
// that is identical on every OS (unlike a shell script, which would need a
// POSIX sh version and a separate space-separated .cmd version on Windows).
//
// Wire format, one line each:
//
//	ARGV\t<arg0>\t<arg1>\t...    (bare "ARGV" when launched with no args)
//	ENV\t<name>=<value>           (one line per ANTHROPIC_* env var)
package main

import (
	"os"
	"strings"
)

func main() {
	var b strings.Builder
	b.WriteString("ARGV")
	if len(os.Args) > 1 {
		b.WriteString("\t")
		b.WriteString(strings.Join(os.Args[1:], "\t"))
	}
	b.WriteString("\n")
	for _, kv := range os.Environ() {
		if strings.HasPrefix(kv, "ANTHROPIC_") {
			b.WriteString("ENV\t")
			b.WriteString(kv)
			b.WriteString("\n")
		}
	}
	if err := os.WriteFile(os.Getenv("CLAU_TEST_OUT"), []byte(b.String()), 0o644); err != nil {
		panic(err)
	}
}
