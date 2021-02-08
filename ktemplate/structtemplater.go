package ktemplate

import (
	"k8s.io/client-go/kubernetes"
	"reflect"
	"strings"
)

type StructTemplater struct {
	Values map[string]string
	Clientset *kubernetes.Clientset
	functions *Functions
}

// this func is required to fulfil the reflectwalk.StructWalker interface
func (w StructTemplater) Struct(reflect.Value) error {
	return nil
}

func (w StructTemplater) StructField(f reflect.StructField, v reflect.Value) error {
	if v.CanSet() && v.Kind() == reflect.String {
		v.SetString(w.Template(v.String()))
	}
	return nil
}

func (w StructTemplater) Template(val string) string {
	if strings.HasPrefix(val, "$") {
		key := strings.TrimRight(strings.TrimLeft(val[1:], "("), ")")
		env := w.Values[key]
		if env != "" {
			return env
		}
	} else if w.Clientset != nil {
		if w.functions == nil{
			w.functions = NewFunctions(w.Clientset)
		}
		parse, err := w.functions.Template(val, w.Values)
		if err != nil {
			return val
		}
		return parse
	}
	return val
}