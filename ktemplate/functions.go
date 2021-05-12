package ktemplate

import (
	"github.com/flanksource/kommons"
	"text/template"

	gomplate "github.com/hairyhenderson/gomplate/v3"
)

type Functions struct {
	clientset *kommons.Client
}

func NewFunctions(clientset *kommons.Client) *Functions {
	return &Functions{clientset: clientset}
}

func (f *Functions) FuncMap() template.FuncMap {
	fm := gomplate.Funcs(nil)
	fm["kget"] = f.KGet
	fm["jsonPath"] = f.JSONPath
	return fm
}
