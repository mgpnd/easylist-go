// Package graph is the GraphQL server the snapshot tests send requests to.
// Everything in it but this file and the schema is generated.
package graph

//go:generate go run ../cmd/generate

import (
	"context"

	"github.com/mgpnd/easylist-go/easylist-behavior/graph/easylistgen"
	easylist "github.com/mgpnd/easylist-go/easylist-core"
)

// Resolver is the root resolver. Each list resolver body is one call.
type Resolver struct {
	Runtime *easylist.Runtime
}

func (r *Resolver) Query() QueryResolver { return queryResolver{r} }

type queryResolver struct{ *Resolver }

func (r queryResolver) People(ctx context.Context, query easylistgen.PeopleListInput) (*easylistgen.PeoplePage, error) {
	return easylistgen.ResolvePeople(ctx, r.Runtime, query)
}

func (r queryResolver) Countries(ctx context.Context, query easylistgen.CountriesListInput) (*easylistgen.CountriesPage, error) {
	return easylistgen.ResolveCountries(ctx, r.Runtime, query)
}
