/*
Copyright 2019 The Kubernetes Authors.
Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at
    http://www.apache.org/licenses/LICENSE-2.0
Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Package kustomize contains helpers for working with embedded kustomize commands
package kustomize

import (
	"bytes"
	"fmt"
	"path/filepath"
	"regexp"
	osruntime "runtime"
	"strings"

	"github.com/TomOnTime/utfutil"
	"github.com/flanksource/commons/logger"
	"gopkg.in/yaml.v3"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	yamlutil "k8s.io/apimachinery/pkg/util/yaml"
	"sigs.k8s.io/kustomize/api/krusty"
	"sigs.k8s.io/kustomize/pkg/types"

	"sigs.k8s.io/kustomize/kyaml/filesys"
)

// Manager define a manager that allow access to kustomize capabilities
type Manager struct {
	filesys.FileSystem
	kustomizeDir          string
	kustomizationFile     *types.Kustomization
	strategicMergePatches strategicMergeSlice
	json6902Patches       json6902Slice
}

func (km *Manager) AddPatch(name string, data []byte) error {
	return km.WriteFile(name, data)
}

var KustomizationFileNames = []string{
	"kustomization.yaml",
	"kustomization.yml",
	"Kustomization",
}

// Kustomize apply a set of patches to a resource.
// Portions of the kustomize logic in this function are taken from the kubernetes-sigs/kind project
func (km *Manager) KustomizeRaw(namespace string, data []byte) ([]runtime.Object, error) {
	raw, err := GetUnstructuredObjects(data)
	if err != nil {
		return nil, err
	}
	return km.Kustomize(namespace, raw...)
}

func setAnnotation(obj *unstructured.Unstructured, key string, value string) {
	annotations := obj.GetAnnotations()
	if annotations == nil {
		annotations = make(map[string]string)
	}
	annotations[key] = value
	obj.SetAnnotations(annotations)
}

// Kustomize apply a set of patches to a resource.
// Portions of the kustomize logic in this function are taken from the kubernetes-sigs/kind project
func (km *Manager) Kustomize(namespace string, objects ...runtime.Object) ([]runtime.Object, error) {
	var kustomized []runtime.Object
	for _, _resource := range objects {
		resource := _resource.(*unstructured.Unstructured)

		if resource == nil {
			continue
		}

		// get patches corresponding to this resource
		//strategicMerge := km.strategicMergePatches.filterByResource(namespace, resource)
		//json6902 := km.json6902Patches.filterByResource(namespace, resource)

		// if there are no patches, for the target resources, exit
		//patchesCnt := len(strategicMerge) + len(json6902)

		//if patchesCnt == 0 {
		//kustomized = append(kustomized, resource)
		//continue
		//}

		setAnnotation(resource, "kustomize/patched", "true")

		// create an in memory fs to use for the kustomization
		fakeDir := "/"
		// for Windows we need this to be a drive because kustomize uses filepath.Abs()
		// which will add a drive letter if there is none. which drive letter is
		// unimportant as the path is on the fake filesystem anyhow
		if osruntime.GOOS == "windows" {
			fakeDir = `C:\`
		}

		memFS := filesys.MakeFsInMemory()

		// writes the resource to a file in the temp file system
		b, err := yaml.Marshal(resource.Object)
		if err != nil {
			return nil, err
		}
		name := "resource.yaml"
		memFS.WriteFile(filepath.Join(fakeDir, name), b) // nolint: errcheck

		// writes strategic merge patches to files in the temp file system
		// km.kustomizationFile.PatchesStrategicMerge = []types.PatchStrategicMerge{}
		// for i, p := range strategicMerge {
		// 	b, err := yaml.Marshal(p)
		// 	if err != nil {
		// 		return nil, err
		// 	}
		// 	name := fmt.Sprintf("patch-%d.yaml", i)
		// 	memFS.WriteFile(filepath.Join(fakeDir, name), b) // nolint: errcheck

		// 	km.kustomizationFile.PatchesStrategicMerge = append(km.kustomizationFile.PatchesStrategicMerge, types.PatchStrategicMerge(name))
		// }

		// // writes json6902 patches to files in the temp file system
		// km.kustomizationFile.PatchesJson6902 = []types.Patch
		// for i, p := range json6902 {
		// 	name := fmt.Sprintf("patchjson-%d.yaml", i)
		// 	memFS.WriteFile(filepath.Join(fakeDir, name), []byte(p.Patch)) // nolint: errcheck

		// 	km.kustomizationFile.PatchesJson6902 = append(km.kustomizationFile.PatchesJson6902, types.Patch{Target: p.Target, Path: name})
		// }

		// writes the kustomization file to the temp file system
		km.kustomizationFile = &types.Kustomization{Resources: []string{name}}
		kbytes, err := yaml.Marshal(km.kustomizationFile)
		if err != nil {
			return nil, err
		}
		memFS.WriteFile(filepath.Join(fakeDir, "kustomization.yaml"), kbytes) // nolint: errcheck

		kresult, err := krusty.MakeKustomizer(krusty.MakeDefaultOptions()).Run(memFS, fakeDir)
		if err != nil {
			return nil, err
		}
		for _, r := range kresult.Resources() {
			y, _ := r.AsYAML()
			objects, err := GetUnstructuredObjects(y)
			if err != nil {
				return nil, err
			}
			kustomized = append(kustomized, objects...)
		}
	}
	return kustomized, nil
}

func GetUnstructuredObjects(data []byte) ([]runtime.Object, error) {
	utfData, err := BytesToUtf8Lf(data)
	if err != nil {
		return nil, fmt.Errorf("error converting to UTF %v", err)
	}
	var items []runtime.Object
	re := regexp.MustCompile(`(?m)^---\n`)
	for _, chunk := range re.Split(utfData, -1) {
		if strings.TrimSpace(chunk) == "" {
			continue
		}
		decoder := yamlutil.NewYAMLOrJSONDecoder(bytes.NewReader([]byte(chunk)), 1024)
		var resource *unstructured.Unstructured

		if err := decoder.Decode(&resource); err != nil {
			return nil, fmt.Errorf("error decoding %s: %s", chunk, err)
		}
		if resource != nil {
			items = append(items, resource)
		}
	}

	return items, nil
}

func BytesToUtf8Lf(file []byte) (string, error) {
	decoded := utfutil.BytesReader(file, utfutil.UTF8)
	buf := new(bytes.Buffer)
	_, err := buf.ReadFrom(decoded)
	if err != nil {
		logger.Errorf("error reading from buffer: %v", err)
		return "", err
	}
	val := buf.Bytes()
	// replace \r with \n -> solves for Mac but leaves \n\n for Windows
	val = bytes.ReplaceAll(val, []byte{13}, []byte{10})
	// replace \n\n with \n
	val = bytes.ReplaceAll(val, []byte{10, 10}, []byte{10})
	return string(val), nil
}
