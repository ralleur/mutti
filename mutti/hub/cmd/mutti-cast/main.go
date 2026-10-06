// SPDX-License-Identifier: GPL-2.0-or-later

// Command mutti-cast qualifies local models through the product harness with
// the frozen synthetic fixture. Development tool; not part of the packages.
package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	hub "github.com/ralleur/mutti/hub"
)

func main() {
	var o hub.CastOptions
	models := flag.String("models", "", "comma-separated catalog model ids")
	flag.StringVar(&o.Fixture, "fixture", "mutti/tests/fixtures/model-casting-v5-dev.json", "frozen fixture")
	flag.StringVar(&o.Rubric, "rubric", "docs/mutti/evidence/casting-v5-rubric.md", "frozen rubric")
	flag.StringVar(&o.Output, "output", "", "new result directory")
	flag.StringVar(&o.Store, "store", "", "engine directory containing models/ (never the user's store)")
	flag.StringVar(&o.Ollama, "ollama", "", "engine executable")
	flag.IntVar(&o.Repetitions, "repetitions", 3, "repetitions per case")
	flag.Parse()
	if *models == "" || o.Output == "" || o.Store == "" || o.Ollama == "" {
		fmt.Fprintln(os.Stderr, "models, output, store and ollama are required")
		os.Exit(2)
	}
	o.Models = strings.Split(*models, ",")
	if err := hub.RunCasting(o); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
