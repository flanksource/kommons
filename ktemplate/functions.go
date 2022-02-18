package ktemplate

import (
	"github.com/flanksource/commons/text"
	"github.com/hairyhenderson/gomplate/v3"
	"k8s.io/client-go/kubernetes"
	"text/template"
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
	commonFuncs := text.GetTemplateFuncs()
	for funcName, _ := range commonFuncs {
		if _, ok := fm[funcName]; !ok {
			fm[funcName] = commonFuncs[funcName]
		}
	}
	return fm
}
