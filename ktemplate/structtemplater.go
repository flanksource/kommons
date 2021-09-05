package ktemplate

import (
	"reflect"
	"strings"
	"text/template"

	"github.com/mitchellh/reflectwalk"
	"k8s.io/client-go/kubernetes"
)

type StructTemplater struct {
	Values    map[string]string
	Clientset *kubernetes.Clientset
	functions *Functions
	// IgnoreFields from walking where key is field name and value is field type
	IgnoreFields map[string]string
	Funcs        template.FuncMap
	DelimSets    []Delims
	// If specified create a function for each value so that is can be accessed via {{ value }} in addition to {{ .value }}
	ValueFunctions bool
	RequiredTag    string
}

type Delims struct {
	Left, Right string
}

// this func is required to fulfil the reflectwalk.StructWalker interface
func (w StructTemplater) Struct(reflect.Value) error {
	return nil
}

func (w StructTemplater) StructField(f reflect.StructField, v reflect.Value) error {
	for key, value := range w.IgnoreFields {
		if key == f.Name && value == f.Type.String() {
			return reflectwalk.SkipEntry
		}
	}
	if w.RequiredTag != "" && f.Tag.Get(w.RequiredTag) != "true" {
		return reflectwalk.SkipEntry
	}
	if v.CanSet() && v.Kind() == reflect.String {
		val, err := w.Template(v.String())
		if err != nil {
			return err
		}
		v.SetString(val)
	}
	return nil
}

func (w StructTemplater) Walk(object interface{}) error {
	return reflectwalk.Walk(object, w)
}

func (w StructTemplater) Template(val string) (string, error) {
	if strings.TrimSpace(val) == "" {
		return val, nil
	}
	if w.functions == nil {
		w.functions = NewFunctions(w.Clientset)
		w.functions.Custom = w.Funcs

		if w.ValueFunctions {
			if w.functions.Custom == nil {
				w.functions.Custom = make(template.FuncMap)
			}
			for k, v := range w.Values {
				_v := v
				w.functions.Custom[k] = func() string {
					return _v
				}
			}
		}
	}
	if len(w.DelimSets) == 0 {
		w.DelimSets = []Delims{{Left: "{{", Right: "}}"}}
	}

	var err error

	for _, delims := range w.DelimSets {
		w.functions.LeftDelim = delims.Left
		w.functions.RightDelim = delims.Right
		val, err = w.functions.Template(val, w.Values)
		if err != nil {
			return val, err
		}
	}
	return val, nil
}
