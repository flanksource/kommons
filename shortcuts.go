package kommons

import (
	"context"
	"fmt"
	"strings"

	"github.com/AlekSi/pointer"
	"github.com/mitchellh/mapstructure"
	perrors "github.com/pkg/errors"
	v1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
)

func (c *Client) CreateOrUpdateConfigMap(name, ns string, data map[string]string) error {
	if c.ApplyDryRun {
		c.Debugf("[dry-run] configmaps/%s/%s created/configured", ns, name)
		return nil
	}
	return c.Apply(ns, &v1.ConfigMap{
		TypeMeta:   metav1.TypeMeta{APIVersion: "v1", Kind: "ConfigMap"},
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
		Data:       data})
}

func (c *Client) CreateOrUpdateNamespace(name string, labels, annotations map[string]string) error {
	k8s, err := c.GetClientset()
	if err != nil {
		return err
	}

	ns := k8s.CoreV1().Namespaces()
	cm, err := ns.Get(context.TODO(), name, metav1.GetOptions{})

	if cm == nil || err != nil {
		cm = &v1.Namespace{
			TypeMeta: metav1.TypeMeta{
				Kind:       "Namespace",
				APIVersion: "v1",
			},
		}
		cm.Name = name
		cm.Labels = labels
		cm.Annotations = annotations

		if !c.ApplyDryRun {
			return c.Apply("", cm)
		}
	} else {
		// update incoming and current labels
		if cm.ObjectMeta.Labels != nil {
			for k, v := range labels {
				cm.ObjectMeta.Labels[k] = v
			}
			labels = cm.ObjectMeta.Labels
		}

		// update incoming and current annotations
		if cm.ObjectMeta.Annotations != nil && annotations != nil {
			for k, v := range annotations {
				cm.ObjectMeta.Annotations[k] = v
			}
			annotations = cm.ObjectMeta.Annotations
		}
	}
	(*cm).Name = name
	(*cm).Labels = labels
	(*cm).Annotations = annotations
	(*cm).TypeMeta = metav1.TypeMeta{
		Kind:       "Namespace",
		APIVersion: "v1",
	}
	if !c.ApplyDryRun {
		return c.Apply("", cm)
	}
	return nil
}

func (c *Client) CreateOrUpdateSecret(name, ns string, data map[string][]byte) error {
	if c.ApplyDryRun {
		c.Debugf("[dry-run] secrets/%s/%s created/configured", ns, name)
		return nil
	}
	return c.Apply(ns, &v1.Secret{
		TypeMeta:   metav1.TypeMeta{APIVersion: "v1", Kind: "Secret"},
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
		Data:       data,
	})
}

func (c *Client) ExposeIngress(namespace, service string, domain string, port int, annotations map[string]string) error {
	k8s, err := c.GetClientset()
	if err != nil {
		return fmt.Errorf("exposeIngress: failed to get client set: %v", err)
	}
	ingresses := k8s.NetworkingV1().Ingresses(namespace)
	ingress, err := ingresses.Get(context.TODO(), service, metav1.GetOptions{})
	if ingress == nil || err != nil {
		ingress = &networkingv1.Ingress{
			ObjectMeta: metav1.ObjectMeta{
				Name:        service,
				Namespace:   namespace,
				Annotations: annotations,
			},
			Spec: networkingv1.IngressSpec{
				TLS: []networkingv1.IngressTLS{
					{
						Hosts: []string{domain},
					},
				},
				Rules: []networkingv1.IngressRule{
					{
						Host: domain,
						IngressRuleValue: networkingv1.IngressRuleValue{
							HTTP: &networkingv1.HTTPIngressRuleValue{
								Paths: []networkingv1.HTTPIngressPath{
									{
										Backend: networkingv1.IngressBackend{
											Service: &networkingv1.IngressServiceBackend{
												Name: service,
												Port: networkingv1.ServiceBackendPort{Number: int32(port)},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		}
		c.Infof("Creating %s/ingress/%s", namespace, service)
		if !c.ApplyDryRun {
			if _, err := ingresses.Create(context.TODO(), ingress, metav1.CreateOptions{}); err != nil {
				return fmt.Errorf("exposeIngress: failed to create ingress: %v", err)
			}
		}
	}
	return nil
}

func (c *Client) Get(namespace string, name string, obj runtime.Object) error {
	client, _, _, err := c.GetDynamicClientFor(namespace, obj)
	if err != nil {
		return err
	}
	unstructuredObj, err := client.Get(context.TODO(), name, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("get: failed to get client: %v", err)
	}

	err = runtime.DefaultUnstructuredConverter.
		FromUnstructured(unstructuredObj.Object, obj)
	if err == nil {
		return nil
	}
	// if c.IsLevelEnabled(logger.TraceLevel) {
	// 	spew.Dump(unstructuredObj.Object)
	// }

	// FIXME(moshloop) getting the zalando operationconfiguration fails with "unrecognized type: int64" so we fall back to brute-force
	c.Warnf("Using mapstructure to decode %s: %v", obj.GetObjectKind().GroupVersionKind().Kind, err)
	config := &mapstructure.DecoderConfig{
		WeaklyTypedInput: true,
		TagName:          "json",
		DecodeHook:       mapstructure.ComposeDecodeHookFunc(decodeStringToTime, decodeStringToDuration, decodeStringToTimeDuration, decodeStringToInt64),
		Result:           obj,
	}

	decoder, err := mapstructure.NewDecoder(config)
	if err != nil {
		return fmt.Errorf("get: failed to decode config: %v", err)
	}
	return decoder.Decode(unstructuredObj.Object)
}

func (c *Client) GetByKind(kind, namespace, name string) (*unstructured.Unstructured, error) {
	client, err := c.GetClientByKind(kind)
	if err != nil {
		return nil, err
	}
	item, err := client.Namespace(namespace).Get(context.TODO(), name, metav1.GetOptions{})

	if errors.IsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return item, nil
}

func (c *Client) GetAPIResource(name string) (*metav1.APIResource, error) {
	clientset, err := c.GetClientset()
	if err != nil {
		return nil, perrors.Wrap(err, "failed to get clientset")
	}

	rm, err := c.GetRestMapper()
	if err != nil {
		return nil, perrors.Wrap(err, "failed to get rest mapper")
	}

	resources, err := clientset.ServerResources()
	if err != nil {
		return nil, perrors.Wrap(err, "failed to get server resources")
	}

	for _, list := range resources {
		for _, resource := range list.APIResources {
			var singularName string
			if resource.Name != name {
				singularName, err = rm.ResourceSingularizer(resource.Name)
				if err != nil {
					continue
				}
			}

			if resource.Name == name || singularName == name {
				parts := strings.Split(list.GroupVersion, "/")
				if len(parts) >= 2 {
					resource.Group = parts[0]
					resource.Version = parts[1]
				} else {
					resource.Group = ""
					resource.Version = parts[0]
				}
				return &resource, nil
			}
		}
	}

	return nil, perrors.Errorf("no resource with name %s found", name)
}

func (c *Client) GetOrCreateSecret(name, ns string, data map[string][]byte) error {
	if c.HasSecret(name, ns) {
		return nil
	}
	return c.CreateOrUpdateSecret(name, ns, data)
}

func (c *Client) GetOrCreatePVC(namespace, name, size, class string) error {
	client, err := c.GetClientset()
	if err != nil {
		return fmt.Errorf("getOrCreatePVC: failed to get client set: %v", err)
	}
	qty, err := resource.ParseQuantity(size)
	if err != nil {
		return fmt.Errorf("getOrCreatePVC: failed to parse quantity: %v", err)
	}
	pvcs := client.CoreV1().PersistentVolumeClaims(namespace)

	existing, err := pvcs.Get(context.TODO(), name, metav1.GetOptions{})
	if err != nil && errors.IsNotFound(err) {
		c.Tracef("GetOrCreatePVC: failed to get PVC: %s", err)
		c.Infof("Creating PVC %s/%s (%s %s)\n", namespace, name, size, class)
		_, err = pvcs.Create(context.TODO(), &v1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{
				Name: name,
			},
			Spec: v1.PersistentVolumeClaimSpec{
				StorageClassName: &class,
				AccessModes: []v1.PersistentVolumeAccessMode{
					v1.ReadWriteOnce,
				},
				Resources: v1.ResourceRequirements{
					Requests: v1.ResourceList{
						v1.ResourceStorage: qty,
					},
				},
			},
		}, metav1.CreateOptions{})
	} else if err != nil {
		return fmt.Errorf("getOrCreatePVC: failed to create PVC: %v", err)
	} else {
		c.Infof("Found existing PVC %s/%s (%s %s) ==> %s\n", namespace, name, size, class, existing.UID)
		return nil
	}
	return err
}

func (c *Client) GetPodReplicas(pod v1.Pod) (int, error) {
	client, err := c.GetClientset()
	if err != nil {
		return 0, err
	}

	for _, owner := range pod.GetOwnerReferences() {
		if owner.Kind == "ReplicaSet" {
			replicasets := client.AppsV1().ReplicaSets(pod.Namespace)
			rs, err := replicasets.Get(context.TODO(), owner.Name, metav1.GetOptions{})
			if err != nil {
				return 0, err
			}
			return int(*rs.Spec.Replicas), nil
		}
	}
	return 1, nil
}

// GetSecret returns the data of a secret or nil for any error
func (c *Client) GetSecret(namespace, name string) *map[string][]byte {
	k8s, err := c.GetClientset()
	if err != nil {
		c.Tracef("failed to get client %v", err)
		return nil
	}
	secret, err := k8s.CoreV1().Secrets(namespace).Get(context.TODO(), name, metav1.GetOptions{})
	if err != nil {
		c.Tracef("failed to get secret %s/%s: %v\n", namespace, name, err)
		return nil
	}
	return &secret.Data
}

// GetConfigMap returns the data of a secret or nil for any error
func (c *Client) GetConfigMap(namespace, name string) *map[string]string {
	k8s, err := c.GetClientset()
	if err != nil {
		c.Tracef("failed to get client %v", err)
		return nil
	}
	cm, err := k8s.CoreV1().ConfigMaps(namespace).Get(context.TODO(), name, metav1.GetOptions{})
	if err != nil {
		c.Tracef("failed to get secret %s/%s: %v\n", namespace, name, err)
		return nil
	}
	return &cm.Data
}

func (c *Client) GetEnvValue(input EnvVar, namespace string) (string, string, error) {
	if input.Value != "" {
		return input.Name, input.Value, nil
	}
	if input.ValueFrom != nil {
		if input.ValueFrom.SecretKeyRef != nil {
			secret := c.GetSecret(namespace, input.ValueFrom.SecretKeyRef.Name)
			if secret == nil {
				return "", "", perrors.New(fmt.Sprintf("Could not get contents of secret %v from namespace %v", input.ValueFrom.SecretKeyRef.Name, namespace))
			}

			value, ok := (*secret)[input.ValueFrom.SecretKeyRef.Key]
			if !ok {
				return input.Name, "", perrors.New(fmt.Sprintf("Could not find key %v in secret %v", input.ValueFrom.SecretKeyRef.Key, input.ValueFrom.SecretKeyRef.Name))
			}
			return input.Name, string(value), nil
		}
		if input.ValueFrom.ConfigMapKeyRef != nil {
			cm := c.GetConfigMap(namespace, input.ValueFrom.ConfigMapKeyRef.Name)
			if cm == nil {
				return "", "", perrors.New(fmt.Sprintf("Could not get contents of configmap %v from namespace %v", input.ValueFrom.ConfigMapKeyRef.Name, namespace))
			}
			value, ok := (*cm)[input.ValueFrom.ConfigMapKeyRef.Key]
			if !ok {
				return input.Name, "", perrors.New(fmt.Sprintf("Could not find key %v in configmap %v", input.ValueFrom.ConfigMapKeyRef.Key, input.ValueFrom.ConfigMapKeyRef.Name))
			}
			return input.Name, value, nil
		}
	}
	return "", "", perrors.New("must specify either value or valueFrom")
}

func (c *Client) GetConditionsForNode(name string) (map[v1.NodeConditionType]v1.ConditionStatus, error) {
	client, err := c.GetClientset()
	if err != nil {
		return nil, err
	}
	node, err := client.CoreV1().Nodes().Get(context.TODO(), name, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}

	if node == nil {
		return nil, nil
	}

	var out = make(map[v1.NodeConditionType]v1.ConditionStatus)
	for _, condition := range node.Status.Conditions {
		out[condition.Type] = condition.Status
	}
	return out, nil
}

// GetMasterNode returns the name of the first node found labelled as a master
func (c *Client) GetMasterNode() (string, error) {
	client, err := c.GetClientset()
	if err != nil {
		return "", fmt.Errorf("GetMasterNode: Failed to get clientset: %v", err)
	}

	nodes, err := client.CoreV1().Nodes().List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return "", err
	}

	for _, node := range nodes.Items {
		if IsMasterNode(node) {
			return node.Name, nil
		}
	}
	return "", fmt.Errorf("no master nodes found")
}

// GetMasterNode returns a list of all master nodes
func (c *Client) GetMasterNodes() ([]string, error) {
	client, err := c.GetClientset()
	if err != nil {
		return nil, nil
	}

	nodes, err := client.CoreV1().Nodes().List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return nil, nil
	}

	var nodeNames []string
	for _, node := range nodes.Items {
		if IsMasterNode(node) {
			nodeNames = append(nodeNames, node.Name)
		}
	}
	return nodeNames, nil
}

// Returns the first pod found by label
func (c *Client) GetFirstPodByLabelSelector(namespace string, labelSelector string) (*v1.Pod, error) {
	client, err := c.GetClientset()
	if err != nil {
		return nil, fmt.Errorf("GetFirstPodByLabelSelector: Failed to get clientset: %v", err)
	}

	pods, err := client.CoreV1().Pods(namespace).List(context.TODO(), metav1.ListOptions{
		LabelSelector: labelSelector,
	})
	if err != nil {
		return nil, fmt.Errorf("GetFirstPodByLabelSelector: Failed to query for %v in namespace %v: %v", labelSelector, namespace, err)
	}

	if (pods != nil && len(pods.Items) < 1) || pods == nil {
		return nil, fmt.Errorf("GetFirstPodByLabelSelector: No pods found for query for %v in namespace %v: %v", labelSelector, namespace, err)
	}

	return &pods.Items[0], nil
}

func (c *Client) GetEventsFor(kind string, object metav1.Object) ([]v1.Event, error) {
	client, err := c.GetClientset()
	if err != nil {
		return nil, err
	}
	selector := client.CoreV1().Events(object.GetNamespace()).GetFieldSelector(
		pointer.ToString(object.GetName()),
		pointer.ToString(object.GetNamespace()),
		&kind,
		pointer.ToString(string(object.GetUID())))
	events, err := client.CoreV1().Events(object.GetNamespace()).List(context.TODO(), metav1.ListOptions{
		FieldSelector: selector.String(),
	})
	if err != nil {
		return nil, err
	}
	return events.Items, nil
}

func (c *Client) ScalePod(pod v1.Pod, replicas int32) error {
	client, err := c.GetClientset()
	if err != nil {
		return err
	}

	for _, owner := range pod.GetOwnerReferences() {
		if owner.Kind == "ReplicaSet" {
			replicasets := client.AppsV1().ReplicaSets(pod.Namespace)
			rs, err := replicasets.Get(context.TODO(), owner.Name, metav1.GetOptions{})
			if err != nil {
				return err
			}
			if *rs.Spec.Replicas != replicas {
				c.Infof("Scaling %s/%s => %d", pod.Namespace, owner.Name, replicas)
				rs.Spec.Replicas = &replicas
				_, err := replicasets.Update(context.TODO(), rs, metav1.UpdateOptions{})
				if err != nil {
					return err
				}
			} else {
				c.Infof("Scaling %s/%s => %d (no-op)", pod.Namespace, owner.Name, replicas)
			}
		}
	}
	return nil
}

func (c *Client) HasSecret(ns, name string) bool {
	client, err := c.GetClientset()
	if err != nil {
		c.Tracef("hasSecret: failed to get client set: %v", err)
		return false
	}
	secrets := client.CoreV1().Secrets(ns)
	cm, err := secrets.Get(context.TODO(), name, metav1.GetOptions{})
	return cm != nil && err == nil
}

func (c *Client) HasConfigMap(ns, name string) bool {
	client, err := c.GetClientset()
	if err != nil {
		c.Tracef("hasConfigMap: failed to get client set: %v", err)
		return false
	}
	configmaps := client.CoreV1().ConfigMaps(ns)
	cm, err := configmaps.Get(context.TODO(), name, metav1.GetOptions{})
	return cm != nil && err == nil
}
