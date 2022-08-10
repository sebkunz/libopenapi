package v3

import (
	"github.com/pb33f/libopenapi/datamodel/low"
)

type Document struct {
	Version      low.NodeReference[string]
	Info         low.NodeReference[*Info]
	Servers      low.NodeReference[[]low.ValueReference[*Server]]
	Paths        low.NodeReference[*Paths]
	Components   low.NodeReference[*Components]
	Security     low.NodeReference[*SecurityRequirement]
	Tags         low.NodeReference[[]low.ValueReference[*Tag]]
	ExternalDocs low.NodeReference[*ExternalDoc]
	Extensions   map[low.NodeReference[string]]low.NodeReference[any]
}
