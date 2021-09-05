package ktemplate

import (
	"text/template"

	gomplate "github.com/hairyhenderson/gomplate/v3"
	"k8s.io/client-go/kubernetes"
)

type Functions struct {
	clientset             *kubernetes.Clientset
	RightDelim, LeftDelim string
	Custom                template.FuncMap
}

func NewFunctions(clientset *kubernetes.Clientset) *Functions {
	return &Functions{clientset: clientset}
}

func (f *Functions) FuncMap() template.FuncMap {
	fm := gomplate.Funcs(nil)
	fm["kget"] = f.KGet
	fm["jsonPath"] = f.JSONPath
	fm["parseMarkdownTables"] = f.ParseMarkdownTables
	for k, v := range f.Custom {
		fm[k] = v
	}
	return fm
}
