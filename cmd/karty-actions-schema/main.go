// karty-actions-schema exports the canonical public action schema into an SDK snapshot.
package main

import (
	"flag"
	"os"

	"github.com/karty-game/karty-sdk/format/actions"
)

func main() {
	output := flag.String("out", "", "destination for the generated schema snapshot")
	flag.Parse()
	if *output == "" {
		_, err := os.Stdout.Write(actions.Schema)
		if err != nil {
			panic(err)
		}
		return
	}
	if err := os.WriteFile(*output, actions.Schema, 0600); err != nil {
		panic(err)
	}
}
