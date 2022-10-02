package kommons

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/flanksource/commons/logger"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

type Specs []Spec
type Spec struct {
	Path  string
	Items []*unstructured.Unstructured
}

// Walk iterates recursively over each file in path and
// returns all of the objects contained
func Walk(path string) (Specs, error) {
	specs := Specs{}
	err := filepath.Walk(path, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if strings.HasSuffix(path, "kustomization.yaml") || strings.HasSuffix(path, "kustomization.yml") {
			return nil
		}
		if !strings.HasSuffix(path, ".yml") && !strings.HasSuffix(path, ".yaml") {
			return nil
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		items, err := GetUnstructuredObjects(data)
		if err != nil {
			logger.Errorf("Error decoding %s: %v", path, err)
			return nil
		}
		items, err = Unwrap(items)
		if err != nil {
			return nil
		}

		specs = append(specs, Spec{
			Path:  path,
			Items: items,
		})
		return nil
	})

	return specs, err
}

func Unwrap(list []*unstructured.Unstructured) ([]*unstructured.Unstructured, error) {
	var items []*unstructured.Unstructured
	for _, item := range list {
		if item.IsList() {

			children, err := item.ToList()
			if err != nil {
				return nil, err
			}
			for _, child := range children.Items {
				items = append(items, &child)
			}
		} else {
			items = append(items, item)
		}
	}
	return items, nil
}

func (specs Specs) FilterBy(kind string) []*unstructured.Unstructured {
	items := []*unstructured.Unstructured{}
	for _, spec := range specs {
		for _, item := range spec.Items {
			if strings.EqualFold(item.GetKind(), kind) {
				items = append(items, item)
			}
		}
	}
	return items
}
