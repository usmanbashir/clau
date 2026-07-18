// Command fakeclaude stands in for the real `claude` binary in clau's
// integration tests. It records its argv and a couple of env vars to the
// file named by $CLAU_TEST_OUT, using a fixed tab-separated wire format
// that is identical on every OS (unlike a shell script, which would need a
// POSIX sh version and a separate space-separated .cmd version on Windows).
//
// Wire format, one line each:
//
//	ARGV\t<arg0>\t<arg1>\t...
//	ENV\tANTHROPIC_BASE_URL=<value>
package main

import (
	"os"
	"strings"
)

func main() {
	var b strings.Builder
	b.WriteString("ARGV\t")
	b.WriteString(strings.Join(os.Args[1:], "\t"))
	b.WriteString("\n")
	b.WriteString("ENV\tANTHROPIC_BASE_URL=")
	b.WriteString(os.Getenv("ANTHROPIC_BASE_URL"))
	b.WriteString("\n")
	if err := os.WriteFile(os.Getenv("CLAU_TEST_OUT"), []byte(b.String()), 0o644); err != nil {
		panic(err)
	}
}
