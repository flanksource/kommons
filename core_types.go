package kommons

import (
	"strings"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

var CoreAPIGroups = []string{
	"admissionregistration.k8s.io",
	"apiextensions.k8s.io",
	"apiregistration.k8s.io",
	"apps",
	"authentication.k8s.io",
	"authorization.k8s.io",
	"autoscaling",
	"batch",
	"certificates.k8s.io",
	"coordination.k8s.io",
	"discovery.k8s.io",
	"events.k8s.io",
	"extensions",
	"flowcontrol.apiserver.k8s.io",
	"networking.k8s.io",
	"node.k8s.io",
	"policy",
	"rbac.authorization.k8s.io",
	"scheduling.k8s.io",
	"storage.k8s.io",
}

func (c *Client) IsCoreType(object *unstructured.Unstructured) bool {
	apiVersion := object.GetAPIVersion()
	parts := strings.Split(apiVersion, "/")
	if len(parts) == 1 { // "core" api group
		return true
	}
	apiGroup := parts[0]
	for _, ag := range CoreAPIGroups {
		if ag == apiGroup {
			return true
		}
	}
	return false
}
