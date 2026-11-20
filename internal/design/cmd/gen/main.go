// Command gen writes internal/design/tokens.css from tokens.json. It runs
// through go generate ./internal/design, from the package's directory.
package main

import (
	"log"
	"os"

	"github.com/nicholasjackson/tcgreplay/internal/design"
)

func main() {
	css, err := design.Generate()
	if err != nil {
		log.Fatal(err)
	}
	if err := os.WriteFile("tokens.css", css, 0o644); err != nil {
		log.Fatal(err)
	}
}
