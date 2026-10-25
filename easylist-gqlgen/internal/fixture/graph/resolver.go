// Package graph is the fixture project's gqlgen package.
package graph

//go:generate go run ../cmd/generate

import (
	"context"

	easylist "github.com/mgpnd/easylist-go/easylist-core"
	easylistgql "github.com/mgpnd/easylist-go/easylist-gqlgen"
	"github.com/mgpnd/easylist-go/easylist-gqlgen/internal/fixture/graph/easylistgen"
	"github.com/mgpnd/easylist-go/easylist-gqlgen/internal/fixture/graph/model"
)

// Resolver is the root resolver. Each list resolver body is one call.
type Resolver struct {
	Runtime *easylist.Runtime
}

func (r *Resolver) Query() QueryResolver       { return queryResolver{r} }
func (r *Resolver) Mutation() MutationResolver { return mutationResolver{r} }

type queryResolver struct{ *Resolver }

func (r queryResolver) People(ctx context.Context, query easylistgen.UsersListInput) (*easylistgen.UsersPage, error) {
	return easylistgen.ResolveUsers(ctx, r.Runtime, query)
}

func (r queryResolver) OrderItems(ctx context.Context, query easylistgen.OrderItemsListInput) (*easylistgen.OrderItemsPage, error) {
	return easylistgen.ResolveOrderItems(ctx, r.Runtime, query)
}

func (r queryResolver) Health(context.Context) (*model.Health, error) {
	return &model.Health{Ok: true}, nil
}

type mutationResolver struct{ *Resolver }

func (r mutationResolver) ExportPeople(ctx context.Context, input easylistgen.UsersExportInput) (*easylistgql.ExportJob, error) {
	return easylistgen.ExportUsers(ctx, r.Runtime, input)
}
