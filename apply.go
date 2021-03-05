package kommons

import (
	"context"
	"fmt"
	"strings"

	"github.com/flanksource/commons/console"
	perrors "github.com/pkg/errors"
	"github.com/sergi/go-diff/diffmatchpatch"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
)

var (
	unchanged  = fmt.Sprintf("(%s)", "unchanged")
	skipping   = fmt.Sprintf("(%s)", "unchanged, skipping")
	created    = fmt.Sprintf("(%s)", console.Greenf("created"))
	deleted    = fmt.Sprintf("(%s)", console.Redf("deleted"))
	configured = fmt.Sprintf("(%s)", console.Magentaf("configured"))
	kustomized = fmt.Sprintf("+%s", console.Yellowf("kustomized"))
	diff       = diffmatchpatch.New()
)

type ApplyHook func(namespace string, obj unstructured.Unstructured)

func (c *Client) copyImmutable(from, to *unstructured.Unstructured) {
	if from == nil {
		return
	}
	if IsService(to) {
		spec := to.Object["spec"].(map[string]interface{})
		spec["clusterIP"] = from.Object["spec"].(map[string]interface{})["clusterIP"]
		spec["type"] = from.Object["spec"].(map[string]interface{})["type"]
		spec["sessionAffinity"] = from.Object["spec"].(map[string]interface{})["sessionAffinity"]

	} else if IsServiceAccount(to) {
		to.Object["secrets"] = from.Object["secrets"]
	} else if IsPVC(to) {
		resourcesRequests, found, _ := unstructured.NestedFieldCopy(to.Object, "spec", "resources", "requests")
		if found {
			to.Object["spec"] = from.Object["spec"]
			unstructured.SetNestedField(to.Object, resourcesRequests, "spec", "resources", "requests")
		}
	} else if IsSecret(to) {
		to.Object["type"] = from.Object["type"]
	} else if IsCustomResourceDefinition(to) {
		webhook, found, _ := unstructured.NestedMap(from.Object, "spec", "conversion", "webhook")
		if found {
			unstructured.SetNestedField(to.Object, webhook, "spec", "conversion", "webhook")
		}
	}

	for _, immutable := range c.ImmutableAnnotations {
		if existing, ok := from.GetAnnotations()[immutable]; ok {
			SetAnnotation(to, immutable, existing)
		}
	}

	to.SetResourceVersion(from.GetResourceVersion())
	to.SetSelfLink(from.GetSelfLink())
	to.SetUID(from.GetUID())
	to.SetCreationTimestamp(from.GetCreationTimestamp())
	to.SetGeneration(from.GetGeneration())
}

// Sanitize will remove "runtime" fields from objects that woulds otherwise increase the verbosity of diffs
func Sanitize(objects ...*unstructured.Unstructured) {
	for _, unstructuredObj := range objects {
		// unstructuredObj.SetCreationTimestamp(metav1.Time{})
		if unstructuredObj.GetAnnotations() == nil {
			unstructuredObj.SetAnnotations(make(map[string]string))
		}
		unstructured.RemoveNestedField(unstructuredObj.Object, "metadata", "creationTimestamp")
		unstructured.RemoveNestedField(unstructuredObj.Object, "creationTimestamp")
		unstructured.RemoveNestedField(unstructuredObj.Object, "metadata", "managedFields")
		unstructured.RemoveNestedField(unstructuredObj.Object, "metadata", "ownerReferences")
		unstructured.RemoveNestedField(unstructuredObj.Object, "metadata", "generation")
		unstructured.RemoveNestedField(unstructuredObj.Object, "metadata", "uid")
		unstructured.RemoveNestedField(unstructuredObj.Object, "metadata", "selfLink")
		unstructured.RemoveNestedField(unstructuredObj.Object, "metadata", "resourceVersion")
		unstructured.RemoveNestedField(unstructuredObj.Object, "metadata", "annotations", "deprecated.daemonset.template.generation")
		unstructured.RemoveNestedField(unstructuredObj.Object, "metadata", "annotations", "template-operator-owner-ref")
		unstructured.RemoveNestedField(unstructuredObj.Object, "metadata", "annotations", "deployment.kubernetes.io/revision")
		unstructured.RemoveNestedField(unstructuredObj.Object, "status")
		unstructured.RemoveNestedField(unstructuredObj.Object, "spec", "template", "metadata", "creationTimestamp")
	}
}

func (c *Client) DeleteUnstructured(namespace string, objects ...*unstructured.Unstructured) error {
	for _, unstructuredObj := range objects {
		client, err := c.GetRestClient(*unstructuredObj)
		if err != nil {
			return err
		}

		if c.ApplyDryRun {
			c.Infof("[dry-run] %s %s", GetName(unstructuredObj), deleted)
		} else {
			if _, err := client.Delete(namespace, unstructuredObj.GetName()); err != nil {
				return err
			}
			c.Infof("%s %s", GetName(unstructuredObj), deleted)
		}
	}
	return nil
}

func RequiresReplacement(obj *unstructured.Unstructured, err error) bool {
	if IsDeployment(obj) || IsDaemonSet(obj) &&
		strings.Contains(fmt.Sprintf("%+v", err), "field is immutable") {
		return true
	} else if IsAnyRoleBinding(obj) &&
		strings.Contains(fmt.Sprintf("%+v", err), "cannot change roleRef") {
		return true
	}
	return false
}

func Diff(from, to *unstructured.Unstructured) string {
	_from := from.DeepCopy()
	_to := to.DeepCopy()
	Sanitize(_from, _to)

	_fromYaml := ToYaml(_from)
	_toYaml := ToYaml(_to)
	if _fromYaml == _toYaml {
		return ""
	}
	diffs := diff.DiffMain(_fromYaml, _toYaml, false)
	for _, d := range diffs {
		if d.Type != diffmatchpatch.DiffEqual {
			return diff.DiffPrettyText(diffs)
		}
	}
	return ""
}

func (c *Client) HasChanges(from, to *unstructured.Unstructured) bool {
	// if IsCustomResourceDefinition(from) || IsCustomResourceDefinitionV1Beta1(from) {
	// 	return true
	// }
	if diff := Diff(from, to); diff != "" {
		if c.Trace {
			c.Tracef(diff)
		}
		return true
	}
	return false
}

func (c *Client) ApplyText(namespace string, specs ...string) error {
	for _, spec := range specs {
		items, err := GetUnstructuredObjects([]byte(spec))
		if err != nil {
			return err

		}
		if err := c.ApplyUnstructured(namespace, items...); err != nil {
			return err
		}
	}
	return nil
}

func (c *Client) ApplyUnstructured(namespace string, objects ...*unstructured.Unstructured) error {
	_objects := []runtime.Object{}
	for _, obj := range objects {
		_objects = append(_objects, obj)
	}
	return c.Apply(namespace, _objects...)
}

func (c *Client) Apply(namespace string, objects ...runtime.Object) error {
	kustomize, err := c.GetKustomize()
	if err != nil {
		return err
	}
	for _, obj := range objects {
		if IsNil(obj) {
			continue
		}
		client, _, unstructuredObj, err := c.GetDynamicClientFor(namespace, obj)
		// apply defaults to objects beforehand to prevent uncessary configured logs
		if unstructuredObj, err = Defaults(unstructuredObj); err != nil {
			return err
		}

		if err != nil {
			if c.ApplyDryRun && strings.HasPrefix(err.Error(), "no matches for kind") {
				c.Debugf("[dry-run] failed to get dynamic client for namespace %s", namespace)
				continue
			}
			return perrors.Wrapf(err, "failed to get dynamic client from %s", obj.GetObjectKind().GroupVersionKind())
		}

		if kustomize != nil {
			kustomized, err := kustomize.Kustomize(namespace, unstructuredObj)
			if err != nil {
				return err
			}
			if len(kustomized) != 1 {
				return fmt.Errorf("expecting 1 kustomized object back, got %d", len(kustomized))
			}
			unstructuredObj = kustomized[0].(*unstructured.Unstructured)
		}

		if c.ApplyHook != nil {
			c.ApplyHook(namespace, *unstructuredObj)
		}
		if c.ApplyDryRun {
			c.trace("apply", unstructuredObj)
			c.Debugf("[dry-run] %s created/configured", GetName(unstructuredObj))
			continue
		}

		extra := ""
		if IsKustomized(unstructuredObj) {
			extra = " " + kustomized
		}
		existing, _ := client.Get(context.TODO(), unstructuredObj.GetName(), metav1.GetOptions{})
		c.copyImmutable(existing, unstructuredObj)
		if existing == nil {
			if c.Trace {
				if IsCustomResourceDefinition(unstructuredObj) {
					c.Tracef("%s creating %s", GetName(unstructuredObj), extra)
				} else {
					c.Tracef(ToYaml(unstructuredObj))
				}
			}
			_, err = client.Create(context.TODO(), unstructuredObj, metav1.CreateOptions{})
			if err != nil {
				return perrors.Wrap(err, GetName(unstructuredObj))
			} else {
				c.Infof("%s %s%s", GetName(unstructuredObj), created, extra)
			}
		} else if !c.HasChanges(existing, unstructuredObj) {
			c.Debugf("%s %s%s", GetName(unstructuredObj), skipping, extra)
		} else {
			newObject := unstructuredObj.DeepCopy()
			updated, err := client.Update(context.TODO(), unstructuredObj, metav1.UpdateOptions{})
			if err != nil {
				if !RequiresReplacement(unstructuredObj, err) {
					return err
				}
				c.Infof("error updating: %s, attempting replacement", GetName(unstructuredObj))
				if err := client.Delete(context.TODO(), existing.GetName(), metav1.DeleteOptions{}); err != nil {
					return perrors.Wrapf(err, "failed to delete %s, during replacement", GetName(unstructuredObj))
				}
				if updated, err = client.Create(context.TODO(), newObject, metav1.CreateOptions{}); err != nil {
					return perrors.Wrapf(err, "failed to recreate %s, during replacement, neither the new or old object remain", GetName(unstructuredObj))
				}
			}

			if updated.GetResourceVersion() == unstructuredObj.GetResourceVersion() {
				c.Debugf("%s %s%s", GetName(unstructuredObj), unchanged, extra)
			} else {
				c.Infof("%s %s%s", GetName(unstructuredObj), configured, extra)
				if c.Trace {
					c.Tracef(Diff(unstructuredObj, existing))
				}
			}
		}
	}
	return nil
}

func (c *Client) DeleteByKind(kind, namespace, name string) error {
	client, err := c.GetClientByKind(kind)
	if err != nil {
		return err
	}

	err = client.Namespace(namespace).Delete(context.TODO(), name, metav1.DeleteOptions{})
	if errors.IsNotFound(err) {
		return nil
	}
	c.Infof("%s/%s/%s %s", console.Bluef(kind), console.Grayf(namespace), console.LightWhitef(name), deleted)
	return err
}

func (c *Client) Annotate(obj runtime.Object, annotations map[string]string) error {
	client, _, unstructuredObj, err := c.GetDynamicClientFor("", obj)
	if err != nil {
		return err
	}
	existing := unstructuredObj.GetAnnotations()
	for k, v := range annotations {
		existing[k] = v
	}
	unstructuredObj.SetAnnotations(existing)
	_, err = client.Update(context.TODO(), unstructuredObj, metav1.UpdateOptions{})
	if err != nil {
		return err
	}
	c.Infof("%s annotated", GetName(unstructuredObj))
	return nil
}

func (c *Client) Label(obj runtime.Object, labels map[string]string) error {
	client, _, unstructuredObj, err := c.GetDynamicClientFor("", obj)
	if err != nil {
		return err
	}
	existing := unstructuredObj.GetLabels()
	for k, v := range labels {
		existing[k] = v
	}
	unstructuredObj.SetLabels(existing)
	if _, err := client.Update(context.TODO(), unstructuredObj, metav1.UpdateOptions{}); err != nil {
		return err
	}
	c.Infof("%s labelled", GetName(unstructuredObj))
	return nil
}

func SetAnnotation(obj *unstructured.Unstructured, key string, value string) {
	annotations := obj.GetAnnotations()
	if annotations == nil {
		annotations = make(map[string]string)
	}
	annotations[key] = value
	obj.SetAnnotations(annotations)
}
