package kommons

import (
	"context"
	"fmt"
	"strings"

	"github.com/flanksource/commons/logger"
	"github.com/go-test/deep"
	perrors "github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
)

type ApplyHook func(namespace string, obj unstructured.Unstructured)

func SetAnnotation(obj *unstructured.Unstructured, key string, value string) {
	annotations := obj.GetAnnotations()
	if annotations == nil {
		annotations = make(map[string]string)
	}
	annotations[key] = value
	obj.SetAnnotations(annotations)
}

func (c *Client) ApplyUnstructured(namespace string, objects ...*unstructured.Unstructured) error {
	for _, unstructuredObj := range objects {
		client, err := c.GetRestClient(*unstructuredObj)
		if err != nil {
			return err
		}

		if c.ApplyHook != nil {
			c.ApplyHook(namespace, *unstructuredObj)
		}
		if c.ApplyDryRun {
			c.Debugf("[dry-run] %s/%s/%s created/configured", client.Resource, unstructuredObj, unstructuredObj.GetName())
		} else {
			_, err = client.Create(namespace, true, unstructuredObj)
			if errors.IsAlreadyExists(err) {
				existingRuntime, err := client.Get(namespace, unstructuredObj.GetName())
				existing := existingRuntime.(*unstructured.Unstructured)

				if unstructuredObj.GetKind() == "Service" {
					// Workaround for immutable spec.clusterIP error message
					spec := unstructuredObj.Object["spec"].(map[string]interface{})
					spec["clusterIP"] = existing.Object["spec"].(map[string]interface{})["clusterIP"]
				} else if unstructuredObj.GetKind() == "ServiceAccount" {
					unstructuredObj.Object["secrets"] = existing.Object["secrets"]
				} else if unstructuredObj.GetKind() == "PersistentVolumeClaim" {
					resourcesRequests, found, err := unstructured.NestedFieldCopy(unstructuredObj.Object, "spec", "resources", "requests")
					if err != nil {
						c.Errorf("Failed to get spec.resources.requests of %s/%s/%s", client.Resource, unstructuredObj.GetNamespace(), unstructuredObj.GetName())
					}
					unstructuredObj.Object["spec"] = existing.Object["spec"]
					if found {
						unstructured.SetNestedField(unstructuredObj.Object, resourcesRequests, "spec", "resources", "requests")
					}
				}
				for _, immutable := range c.ImmutableAnnotations {
					if existing, ok := existing.GetAnnotations()[immutable]; ok {
						SetAnnotation(unstructuredObj, immutable, existing)
					}
				}

				unstructuredObj.SetResourceVersion(existing.GetResourceVersion())
				unstructuredObj.SetSelfLink(existing.GetSelfLink())
				unstructuredObj.SetUID(existing.GetUID())
				unstructuredObj.SetCreationTimestamp(existing.GetCreationTimestamp())
				unstructuredObj.SetGeneration(existing.GetGeneration())

				updated, err := client.Replace(namespace, unstructuredObj.GetName(), true, unstructuredObj)
				if err != nil {
					return perrors.Wrapf(err, "error handling: %s", client.Resource)
				} else {
					updatedUnstructured := updated.(*unstructured.Unstructured)
					if updatedUnstructured.GetResourceVersion() == unstructuredObj.GetResourceVersion() {
						c.Debugf("%s/%s/%s (unchanged)", client.Resource, unstructuredObj.GetNamespace(), unstructuredObj.GetName())
					} else {
						// remove "runtime" fields from objects that woulds otherwise increase the verbosity of diffs
						unstructured.RemoveNestedField(unstructuredObj.Object, "metadata", "managedFields")
						unstructured.RemoveNestedField(unstructuredObj.Object, "metadata", "ownerReferences")
						unstructured.RemoveNestedField(unstructuredObj.Object, "metadata", "generation")
						unstructured.RemoveNestedField(unstructuredObj.Object, "metadata", "annotations", "deprecated.daemonset.template.generation")
						unstructured.RemoveNestedField(unstructuredObj.Object, "metadata", "annotations", "template-operator-owner-ref")
						unstructured.RemoveNestedField(updatedUnstructured.Object, "metadata", "managedFields")
						unstructured.RemoveNestedField(updatedUnstructured.Object, "metadata", "ownerReferences")
						unstructured.RemoveNestedField(updatedUnstructured.Object, "metadata", "generation")
						unstructured.RemoveNestedField(updatedUnstructured.Object, "metadata", "annotations", "deprecated.daemonset.template.generation")
						unstructured.RemoveNestedField(updatedUnstructured.Object, "metadata", "annotations", "template-operator-owner-ref")

						diff := deep.Equal(unstructuredObj.Object, updatedUnstructured.Object)
						if len(diff) > 0 {
							c.Debugf("Diff: %s", diff)
							c.Infof("%s/%s/%s configured %d", client.Resource, unstructuredObj.GetNamespace(), unstructuredObj.GetName(), len(diff))
						} else {
							c.Debugf("%s/%s/%s (unchanged)", client.Resource, unstructuredObj.GetNamespace(), unstructuredObj.GetName())
						}

					}
				}
			} else if err == nil {
				c.Infof("%s/%s/%s created", client.Resource, unstructuredObj.GetNamespace(), unstructuredObj.GetName())
			} else {
				return perrors.Wrapf(err, "error handling: %s", client.Resource)
			}
		}
	}
	return nil
}

func (c *Client) DeleteUnstructured(namespace string, objects ...*unstructured.Unstructured) error {
	for _, unstructuredObj := range objects {
		client, err := c.GetRestClient(*unstructuredObj)
		if err != nil {
			return err
		}

		if c.ApplyDryRun {
			c.Debugf("[dry-run] %s/%s/%s removed", namespace, client.Resource, unstructuredObj.GetName())
		} else {
			if _, err := client.Delete(namespace, unstructuredObj.GetName()); err != nil {
				return err
			}
			c.Infof("%s/%s/%s removed", namespace, client.Resource, unstructuredObj.GetName())
		}
	}
	return nil
}

func (c *Client) Apply(namespace string, objects ...runtime.Object) error {
	for _, obj := range objects {
		client, resource, unstructuredObj, err := c.GetDynamicClientFor(namespace, obj)
		if err != nil {
			if c.ApplyDryRun && strings.HasPrefix(err.Error(), "no matches for kind") {
				c.Debugf("[dry-run] failed to get dynamic client for namespace %s", namespace)
				continue
			}
			return perrors.Wrapf(err, "failed to get dynamic client from %s", obj.GetObjectKind())
		}

		if c.ApplyHook != nil {
			c.ApplyHook(namespace, *unstructuredObj)
		}
		if c.ApplyDryRun {
			c.trace("apply", unstructuredObj)
			c.Debugf("[dry-run] %s/%s created/configured", resource.Resource, unstructuredObj.GetName())
			continue
		}

		existing, _ := client.Get(context.TODO(), unstructuredObj.GetName(), metav1.GetOptions{})

		if existing == nil {
			c.trace("creating", unstructuredObj)
			_, err = client.Create(context.TODO(), unstructuredObj, metav1.CreateOptions{})
			if err != nil {
				c.Errorf("error creating: %s/%s/%s : %+v", resource.Group, resource.Version, resource.Resource, err)
			} else {
				c.Infof("%s/%s/%s created", resource.Resource, unstructuredObj.GetNamespace(), unstructuredObj.GetName())
			}
		} else {
			if unstructuredObj.GetKind() == "Service" {
				// Workaround for immutable spec.clusterIP error message
				spec := unstructuredObj.Object["spec"].(map[string]interface{})
				spec["clusterIP"] = existing.Object["spec"].(map[string]interface{})["clusterIP"]
			} else if unstructuredObj.GetKind() == "ServiceAccount" {
				unstructuredObj.Object["secrets"] = existing.Object["secrets"]
			} else if unstructuredObj.GetKind() == "PersistentVolumeClaim" {
				resourcesRequests, found, err := unstructured.NestedFieldCopy(unstructuredObj.Object, "spec", "resources", "requests")
				if err != nil {
					c.Errorf("Failed to get spec.resources.requests of %s/%s/%s", resource.Resource, unstructuredObj.GetNamespace(), unstructuredObj.GetName())
				}
				unstructuredObj.Object["spec"] = existing.Object["spec"]
				if found {
					unstructured.SetNestedField(unstructuredObj.Object, resourcesRequests, "spec", "resources", "requests")
				}
			}
			// apps/DameonSet MatchExpressions:[]v1.LabelSelectorRequirement(nil)}: field is immutable
			// webhook CA's

			newObject := unstructuredObj.DeepCopy()
			c.trace("updating", unstructuredObj)
			unstructuredObj.SetResourceVersion(existing.GetResourceVersion())
			unstructuredObj.SetSelfLink(existing.GetSelfLink())
			unstructuredObj.SetUID(existing.GetUID())
			unstructuredObj.SetCreationTimestamp(existing.GetCreationTimestamp())
			unstructuredObj.SetGeneration(existing.GetGeneration())
			if existing.GetAnnotations() != nil && existing.GetAnnotations()["deployment.kubernetes.io/revision"] != "" {
				annotations := unstructuredObj.GetAnnotations()
				if annotations == nil {
					annotations = make(map[string]string)
				}
				annotations["deployment.kubernetes.io/revision"] = existing.GetAnnotations()["deployment.kubernetes.io/revision"]
				unstructuredObj.SetAnnotations(annotations)
			}
			updated, err := client.Update(context.TODO(), unstructuredObj, metav1.UpdateOptions{})
			if err != nil {
				//Primarily cert-manager version upgrade, but should redeploy any incompatible deployments
				c.Errorf("error updating: %s/%s/%s : %+v", unstructuredObj.GetNamespace(), resource.Resource, unstructuredObj.GetName(), err)
				if (resource.Resource == "deployments" || resource.Resource == "daemonsets") && strings.Contains(fmt.Sprintf("%+v", err), "field is immutable") {
					c.Errorf("Immutable field change required in %s/%s/%s, attempting to delete", unstructuredObj.GetNamespace(), resource.Resource, unstructuredObj.GetName())
					if delerr := client.Delete(context.TODO(), existing.GetName(), metav1.DeleteOptions{}); delerr != nil {
						c.Errorf("Failed to delete %s/%s/%s: %+v", unstructuredObj.GetNamespace(), resource.Resource, unstructuredObj.GetName(), err)
						return delerr
					}
					if updated, err = client.Create(context.TODO(), newObject, metav1.CreateOptions{}); err != nil {
						c.Errorf("Failed to create new %s/%s/%s: %+v", newObject.GetNamespace(), resource.Resource, newObject.GetName(), err)
						return err
					}
				} else if (unstructuredObj.GetKind() == "RoleBinding" || unstructuredObj.GetKind() == "ClusterRoleBinding") && strings.Contains(fmt.Sprintf("%+v", err), "cannot change roleRef") {
					c.Errorf("Immutable field change required in %s/%s/%s, attempting to delete", unstructuredObj.GetNamespace(), resource.Resource, unstructuredObj.GetName())
					if delerr := client.Delete(context.TODO(), existing.GetName(), metav1.DeleteOptions{}); delerr != nil {
						c.Errorf("Failed to delete %s/%s/%s: %+v", unstructuredObj.GetNamespace(), resource.Resource, unstructuredObj.GetName(), err)
						return delerr
					}
					if updated, err = client.Create(context.TODO(), newObject, metav1.CreateOptions{}); err != nil {
						c.Errorf("Failed to create new %s/%s/%s: %+v", newObject.GetNamespace(), resource.Resource, newObject.GetName(), err)
						return err
					}
				} else {
					return err
				}
			}

			if updated.GetResourceVersion() == unstructuredObj.GetResourceVersion() {
				c.Debugf("%s/%s/%s (unchanged)", resource.Resource, unstructuredObj.GetNamespace(), unstructuredObj.GetName())
			} else {
				c.Infof("%s/%s/%s configured", resource.Resource, unstructuredObj.GetNamespace(), unstructuredObj.GetName())
				if logger.IsTraceEnabled() {
					// remove "runtime" fields from objects that woulds otherwise increase the verbosity of diffs
					unstructured.RemoveNestedField(unstructuredObj.Object, "metadata", "managedFields")
					unstructured.RemoveNestedField(unstructuredObj.Object, "metadata", "generation")
					unstructured.RemoveNestedField(unstructuredObj.Object, "metadata", "annotations", "deprecated.daemonset.template.generation")

					unstructured.RemoveNestedField(existing.Object, "metadata", "managedFields")
					unstructured.RemoveNestedField(existing.Object, "metadata", "generation")
					unstructured.RemoveNestedField(existing.Object, "metadata", "annotations", "deprecated.daemonset.template.generation")

					diff := deep.Equal(unstructuredObj.Object["metadata"], existing.Object["metadata"])
					if len(diff) > 0 {
						c.Tracef("%s", diff)
					}
				}
			}
		}
	}
	return nil
}

func (c *Client) DeleteByKind(kind, namespace, name string) error {
	c.Debugf("Deleting %s/%s/%s", kind, namespace, name)
	client, err := c.GetClientByKind(kind)
	if err != nil {
		return err
	}

	err = client.Namespace(namespace).Delete(context.TODO(), name, metav1.DeleteOptions{})
	if errors.IsNotFound(err) {
		return nil
	}
	return err
}

func (c *Client) Annotate(obj runtime.Object, annotations map[string]string) error {
	client, resource, unstructuredObj, err := c.GetDynamicClientFor("", obj)
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
		return fmt.Errorf("annotate: failed to update object: #{err}")
	}
	c.Infof("%s/%s/%s annotated", resource.Resource, unstructuredObj.GetNamespace(), unstructuredObj.GetName())
	return nil
}

func (c *Client) Label(obj runtime.Object, labels map[string]string) error {
	client, resource, unstructuredObj, err := c.GetDynamicClientFor("", obj)
	if err != nil {
		return err
	}
	existing := unstructuredObj.GetLabels()
	for k, v := range labels {
		existing[k] = v
	}
	unstructuredObj.SetLabels(existing)
	if _, err := client.Update(context.TODO(), unstructuredObj, metav1.UpdateOptions{}); err != nil {
		return fmt.Errorf("label: failed to update client: %v", err)
	}
	c.Infof("%s/%s/%s labelled", resource.Resource, unstructuredObj.GetNamespace(), unstructuredObj.GetName())
	return nil
}
