package base

import (
	"github.com/pb33f/libopenapi/datamodel/low"
	"gopkg.in/yaml.v3"
)

type XML struct {
	Name       low.NodeReference[string]
	Namespace  low.NodeReference[string]
	Prefix     low.NodeReference[string]
	Attribute  low.NodeReference[bool]
	Wrapped    low.NodeReference[bool]
	Extensions map[low.KeyReference[string]]low.ValueReference[any]
}

func (x *XML) Build(root *yaml.Node) error {
	x.Extensions = low.ExtractExtensions(root)
	return nil
}