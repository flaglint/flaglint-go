package fixtures

import (
	"time"

	ld "github.com/launchdarkly/go-server-sdk/v7"
)

// Mirrors weaviate/weaviate's actual FeatureFlag[T] shape: a generic
// struct whose method receiver type is *ast.IndexExpr, not the plain
// *ast.StarExpr/*ast.Ident simpleTypeName originally handled — a method
// on a generic type silently failed to resolve its own receiver's type at
// all before this fixture's underlying fix.
type GenericSupportedTypes interface {
	bool | string
}

type GenericIntegration struct {
	ldClient *ld.LDClient
}

func newGenericIntegration() *GenericIntegration {
	client, _ := ld.MakeClient("sdk-key", 5*time.Second)
	return &GenericIntegration{ldClient: client}
}

type GenericFeatureFlag[T GenericSupportedTypes] struct {
	integ *GenericIntegration
}

func (f *GenericFeatureFlag[T]) Evaluate() bool {
	v, _ := f.integ.ldClient.BoolVariation("generic-receiver-flag", nil, false)
	return v
}
