package ktemplate

import (
	"encoding/json"
	"strings"

	"github.com/flanksource/commons/logger"
	"github.com/tidwall/gjson"
)

func (f *Functions) KGet(path, jsonpath string) string {
	parts := strings.Split(path, "/")
	if len(parts) != 3 {
		logger.Errorf("invalid call to kget: expected path to contain kind/namespace/name")
		return ""
	}

	kind := parts[0]
	namespace := parts[1]
	name := parts[2]

	if kind == "configmap" || kind == "cm" || kind == "ConfigMap" {
		configMapData := f.clientset.GetConfigMap(namespace, name)
		if configMapData == nil {
			logger.Errorf("failed to read configmap name %s namespace %s: %v", name, namespace)
			return ""
		}

		data := *configMapData
		return data[jsonpath]
	} else if kind == "secret" || kind == "Secret" {
		secretData := f.clientset.GetSecret(namespace, name)
		if secretData == nil {
			logger.Errorf("failed to read secret name %s namespace %s: %v", name, namespace)
			return ""
		}
		data := *secretData
		return string(data[jsonpath])
	} else if kind == "service" || kind == "svc" {
		svc, err := f.clientset.GetByKind("Service", namespace, name)
		if svc == nil || err != nil {
			logger.Errorf("failed to read service name %s namespace %s: %v", name, namespace, err)
			return ""
		}

		encodedJSON, err := json.Marshal(svc.Object)
		if err != nil {
			logger.Errorf("failed to encode json name %s namespace %s: %v", name, namespace, err)
			return ""
		}
		value := gjson.Get(string(encodedJSON), jsonpath)
		return value.String()
	} else if kind != "" {
		object, err := f.clientset.GetByKind(kind, namespace, name)
		if object == nil || err != nil {
			logger.Errorf("failed to read %s name %s namespace %s: %v", kind, name, namespace, err)
			return ""
		}
		encodedJSON, err := json.Marshal(object.Object)
		if err != nil {
			logger.Errorf("failed to encode json name %s namespace %s: %v", name, namespace, err)
			return ""
		}
		value := gjson.Get(string(encodedJSON), jsonpath)
		return value.String()
	}

	return ""
}
