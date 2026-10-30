// Command generate runs gqlgen with the easylist plugin for the module's
// GraphQL server. Run it from the graph directory: go generate ./graph
package main

import (
	"fmt"
	"os"

	"github.com/mgpnd/easylist-go/easylist-behavior/lists"
	easylistgql "github.com/mgpnd/easylist-go/easylist-gqlgen"

	"github.com/99designs/gqlgen/api"
	"github.com/99designs/gqlgen/codegen/config"
)

func main() {
	cfg, err := config.LoadConfigFromDefaultLocations()
	if err != nil {
		fmt.Fprintln(os.Stderr, "failed to load config:", err)
		os.Exit(2)
	}
	plugin := easylistgql.NewPlugin(lists.Registry, easylistgql.Options{
		Registry: "github.com/mgpnd/easylist-go/easylist-behavior/lists.Registry",
	})
	if err := api.Generate(cfg, api.PrependPlugin(plugin)); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(3)
	}
}
