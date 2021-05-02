package ktemplate

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/flanksource/commons/logger"
	"github.com/tidwall/gjson"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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

	if kind == "configmap" || kind == "cm" {
		cm, err := f.clientset.CoreV1().ConfigMaps(namespace).Get(context.Background(), name, metav1.GetOptions{})
		if err != nil {
			if !errors.IsNotFound(err) {
				logger.Errorf("failed to read configmap name %s namespace %s: %v", name, namespace, err)
			}
			return ""
		}

		encodedJSON, err := json.Marshal(cm)
		if err != nil {
			logger.Errorf("failed to encode json name %s namespace %s: %v", name, namespace, err)
			return ""
		}
		value := gjson.Get(string(encodedJSON), jsonpath)
		return value.String()
	} else if kind == "secret" {
		secret, err := f.clientset.CoreV1().Secrets(namespace).Get(context.Background(), name, metav1.GetOptions{})
		if err != nil {
			if !errors.IsNotFound(err) {
				logger.Errorf("failed to read secret name %s namespace %s: %v", name, namespace, err)
			}
			return ""
		}
		return string(secret.Data[jsonpath])
	} else if kind == "service" || kind == "svc" {
		svc, err := f.clientset.CoreV1().Services(namespace).Get(context.Background(), name, metav1.GetOptions{})
		if err != nil {
			if !errors.IsNotFound(err) {
				logger.Errorf("failed to read service name %s namespace %s: %v", name, namespace, err)
			}
			return ""
		}

		encodedJSON, err := json.Marshal(svc)
		if err != nil {
			logger.Errorf("failed to encode json name %s namespace %s: %v", name, namespace, err)
			return ""
		}
		value := gjson.Get(string(encodedJSON), jsonpath)
		return value.String()
	}

	return ""
}
