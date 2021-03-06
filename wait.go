package kommons

import (
	"context"
	"fmt"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"strings"
	"time"
)

type WaitFN func(*unstructured.Unstructured) (bool, string)

func (c *Client) WaitForNamespace(ns string, timeout time.Duration) error {
	if c.ApplyDryRun {
		return nil
	}
	start := time.Now()
	msg := true
	for {
		ready, message := c.IsNamespaceReady(ns)
		if start.Add(timeout).Before(time.Now()) {
			return fmt.Errorf(message)
		}
		if ready {
			return nil
		}
		if !msg {
			c.Infof(message)
			msg = true
		}
		time.Sleep(2 * time.Second)
	}
	return nil
}

func (c *Client) IsNamespaceReady(ns string) (bool, string) {
	if c.ApplyDryRun {
		return true, ""
	}
	client, err := c.GetClientset()
	if err != nil {
		return false, err.Error()
	}
	pods := client.CoreV1().Pods(ns)

	ready := 0
	pending := 0
	list, _ := pods.List(context.TODO(), metav1.ListOptions{})
	for _, pod := range list.Items {
		conditions := true
		for _, condition := range pod.Status.Conditions {
			if condition.Status == v1.ConditionFalse {
				conditions = false
			}
		}
		if conditions && (pod.Status.Phase == v1.PodRunning || pod.Status.Phase == v1.PodSucceeded) {
			ready++
		} else {
			pending++
		}
	}
	if ready > 0 && pending == 0 {
		return true, ""
	}
	return false, fmt.Sprintf("%s ⏳ waiting for ready=%d, pending=%d", Name{Kind: "Namespace", Name: ns}, ready, pending)
}

func (c *Client) WaitFor(obj runtime.Object, timeout time.Duration) (*unstructured.Unstructured, error) {
	id := GetName(obj)
	return c.WaitForResource(id.Kind, id.Namespace, id.Name, timeout)
}

func (c *Client) WaitForResource(kind, namespace, name string, timeout time.Duration) (*unstructured.Unstructured, error) {
	return c.waitForResource(kind, namespace, name, timeout, c.IsReady)
}

func (c *Client) WaitForCRD(kind, namespace, name string, timeout time.Duration, waitFN WaitFN) (*unstructured.Unstructured, error) {
	return c.waitForResource(kind, namespace, name, timeout, waitFN)
}

func (c *Client) waitForResource(kind, namespace, name string, timeout time.Duration, waitFN WaitFN) (*unstructured.Unstructured, error) {
	if c.ApplyDryRun {
		return nil, nil
	}
	client, err := c.GetClientByKind(kind)
	if err != nil {
		return nil, err
	}
	start := time.Now()
	var msg string
	id := Name{Kind: kind, Namespace: namespace, Name: name}

	for {

		item, _ := client.Namespace(namespace).Get(context.TODO(), name, metav1.GetOptions{})

		if start.Add(timeout).Before(time.Now()) {
			return nil, fmt.Errorf("timeout exceeded waiting for %s/%s is %s, error: %v", kind, name, "", err)
		}
		ready, message := waitFN(item)
		if ready {
			return item, nil
		}

		if !ready && message != msg {
			c.Infof("%s %s", id, message)
			msg = message
		}
		time.Sleep(1 * time.Second)
	}
}

func (c *Client) WaitForAPIResource(group, name string, timeout time.Duration) error {
	if c.ApplyDryRun {
		return nil
	}

	start := time.Now()
	var msg string
	id := Name{Kind: "CustomResourceDefinition", Namespace: "*", Name: name}

	for {
		if start.Add(timeout).Before(time.Now()) {
			return fmt.Errorf("timeout exceeded")
		}
		ready, message := c.IsCRDReady(group, name)
		if ready {
			return nil
		}
		if !ready && message != msg {
			c.Infof("%s %s", id, message)
			msg = message
		}
		time.Sleep(1 * time.Second)
	}
}

func (c *Client) IsCRDReady(group, name string) (bool, string) {
	client, err := c.GetClientset()
	if err != nil {
		return false, "cannot connect to api"
	}
	_, resources, err := client.ServerGroupsAndResources()
	if err != nil {
		return false, "⏳ waiting for API resources"
	}

	for _, list := range resources {
		if !strings.HasPrefix(list.GroupVersion, group) {
			continue
		}
		for _, res := range list.APIResources {
			if res.Name == strings.ToLower(name) {
				return true, ""
			}
		}
	}
	return false, "⏳ waiting for API resource"
}

func (c *Client) IsConstraintTemplateReady(item *unstructured.Unstructured) (bool, string) {
	if item.Object["status"] == nil {
		return false, "⏳ waiting to become ready"
	}
	if fmt.Sprintf("%v", item.Object["status"].(map[string]interface{})["created"]) == "true" {
		return true, ""
	}

	return false, "⏳ waiting to be created"
}

func IsAppReady(item *unstructured.Unstructured) (bool, string) {
	if item.Object["status"] == nil {
		return false, "⏳ waiting to become ready"
	}

	status := item.Object["status"].(map[string]interface{})
	if status["replicas"] != "" && status["replicas"] == status["readyReplicas"] {
		return true, ""
	}
	return false, fmt.Sprintf("⏳ waiting for replicas to become ready %v/%v", status["readyReplicas"], status["replicas"])
}

func (c *Client) IsReady(item *unstructured.Unstructured) (bool, string) {
	if c.ApplyDryRun {
		return true, ""
	}
	if item == nil {
		return false, "⏳ waiting to be created"
	}

	if IsSecret(item) {
		data, found, _ := unstructured.NestedMap(item.Object, "data")
		if found && len(data) > 0 {
			return true, ""
		} else {
			return false, "⏳ waiting for data"
		}
	}

	if IsApp(item) {
		return IsAppReady(item)
	}

	if IsConstraintTemplate(item) {
		return c.IsConstraintTemplateReady(item)
	}

	if item.Object["status"] == nil {
		return false, "⏳ waiting to become ready"
	}

	status := item.Object["status"].(map[string]interface{})

	if _, found := status["conditions"]; !found {
		return false, "⏳ waiting to become ready"
	}

	conditions := status["conditions"].([]interface{})
	if len(conditions) == 0 {
		return false, "⏳ waiting to become ready"
	}
	for _, raw := range conditions {
		condition := raw.(map[string]interface{})
		if condition["type"] != "Ready" && condition["status"] != "True" {
			return false, fmt.Sprintf("⏳ waiting for %s/%s: %s", condition["type"], condition["status"], condition["message"])
		}
	}
	return true, ""
}

func IsElasticReady(item *unstructured.Unstructured) (bool, string) {
	if item == nil {
		return false, "⏳ waiting to be created"
	}

	if item.Object["status"] == nil {
		return false, "⏳ waiting to become ready"
	}

	status := item.Object["status"].(map[string]interface{})
	phase, found := status["phase"]
	if !found {
		return false, "⏳ waiting to become ready"
	}
	if phase != "Ready" {
		return false, "⏳ waiting to become ready"
	}

	return true, ""
}

// WaitForPod waits for a pod to be in the specified phase, or returns an
// error if the timeout is exceeded
func (c *Client) WaitForPod(ns, name string, timeout time.Duration, phases ...v1.PodPhase) error {
	if c.ApplyDryRun {
		return nil
	}
	client, err := c.GetClientset()
	if err != nil {
		return fmt.Errorf("waitForPod: Failed to get clientset: %v", err)
	}
	pods := client.CoreV1().Pods(ns)
	start := time.Now()
	for {
		pod, err := pods.Get(context.TODO(), name, metav1.GetOptions{})
		if start.Add(timeout).Before(time.Now()) {
			return fmt.Errorf("timeout exceeded waiting for %s is %s, error: %v", name, pod.Status.Phase, err)
		}

		if pod == nil || pod.Status.Phase == v1.PodPending {
			time.Sleep(5 * time.Second)
			continue
		}
		if pod.Status.Phase == v1.PodFailed {
			return nil
		}

		for _, phase := range phases {
			if pod.Status.Phase == phase {
				return nil
			}
		}
	}
}

// WaitForDeployment waits for a deployment to have at least 1 ready replica, or returns an
// error if the timeout is exceeded
func (c *Client) WaitForDeployment(ns, name string, timeout time.Duration) error {
	if c.ApplyDryRun {
		return nil
	}
	client, err := c.GetClientset()
	if err != nil {
		return err
	}
	deployments := client.AppsV1().Deployments(ns)
	id := Name{Kind: "Deployment", Namespace: ns, Name: name}
	start := time.Now()
	msg := false
	for {
		deployment, _ := deployments.Get(context.TODO(), name, metav1.GetOptions{})
		if start.Add(timeout).Before(time.Now()) {
			return fmt.Errorf("timeout exceeded waiting for deployment to become ready %s", name)
		}
		if deployment != nil && deployment.Status.ReadyReplicas >= 1 {
			return nil
		}

		if !msg {
			c.Infof("%s ⏳ waiting for at least 1 pod", id)
			msg = true
		}

		time.Sleep(2 * time.Second)
	}
}

// WaitForStatefulSet waits for a statefulset to have at least 1 ready replica, or returns an
// error if the timeout is exceeded
func (c *Client) WaitForStatefulSet(ns, name string, timeout time.Duration) error {
	if c.ApplyDryRun {
		return nil
	}
	client, err := c.GetClientset()
	if err != nil {
		return err
	}
	statefulsets := client.AppsV1().StatefulSets(ns)
	id := Name{Kind: "Statefulset", Namespace: ns, Name: name}
	start := time.Now()
	msg := false
	for {
		statefulset, _ := statefulsets.Get(context.TODO(), name, metav1.GetOptions{})
		if start.Add(timeout).Before(time.Now()) {
			return fmt.Errorf("timeout exceeded waiting for statefulset to become ready %s", name)
		}
		if statefulset != nil && statefulset.Status.ReadyReplicas >= 1 {
			return nil
		}

		if !msg {
			c.Infof("%s ⏳ waiting for at least 1 pod", id)
			msg = true
		}

		time.Sleep(2 * time.Second)
	}
}

// WaitForDaemonSet waits for a statefulset to have at least 1 ready replica, or returns an
// error if the timeout is exceeded
func (c *Client) WaitForDaemonSet(ns, name string, timeout time.Duration) error {
	if c.ApplyDryRun {
		return nil
	}
	client, err := c.GetClientset()
	if err != nil {
		return err
	}
	daemonsets := client.AppsV1().DaemonSets(ns)
	id := Name{Kind: "Daemonset", Name: name, Namespace: ns}
	start := time.Now()
	msg := false
	for {
		daemonset, err := daemonsets.Get(context.TODO(), name, metav1.GetOptions{})
		if err != nil {
			return err
		}
		if start.Add(timeout).Before(time.Now()) {
			return fmt.Errorf("%s timeout waiting for daemonset to become ready", id)
		}

		if daemonset != nil && daemonset.Status.NumberReady >= 1 {
			return nil
		}

		if !msg {
			c.Infof("%s ⏳ waiting for at least 1 pod", id)
			msg = true
		}

		time.Sleep(2 * time.Second)
	}
}

// WaitForNode waits for a pod to be in the specified phase, or returns an
// error if the timeout is exceeded
func (c *Client) WaitForNode(name string, timeout time.Duration, condition v1.NodeConditionType, statii ...v1.ConditionStatus) (map[v1.NodeConditionType]v1.ConditionStatus, error) {
	if c.ApplyDryRun {
		return nil, nil
	}
	start := time.Now()
	for {
		conditions, err := c.GetConditionsForNode(name)
		if start.Add(timeout).Before(time.Now()) {
			return conditions, fmt.Errorf("timeout exceeded waiting for %s is %s, error: %v", name, conditions, err)
		}

		for _, status := range statii {
			if conditions[condition] == status {
				return conditions, nil
			}
		}
		time.Sleep(2 * time.Second)
	}
}

// WaitForNode waits for a pod to be in the specified phase, or returns an
// error if the timeout is exceeded
func (c *Client) WaitForTaintRemoval(name string, timeout time.Duration, taintKey string) error {
	if c.ApplyDryRun {
		return nil
	}
	start := time.Now()
outerLoop:
	for {
		if time.Since(start) > timeout {
			return fmt.Errorf("timeout exceeded waiting for %s to not have %s", name, taintKey)
		}

		client, err := c.GetClientset()
		if err != nil {
			return err
		}
		node, err := client.CoreV1().Nodes().Get(context.TODO(), name, metav1.GetOptions{})
		if err != nil {
			return err
		}

		for _, taint := range node.Spec.Taints {
			if taint.Key == taintKey {
				time.Sleep(2 * time.Second)
				continue outerLoop
			}
		}
		// taint not found
		return nil
	}
}

// WaitForPodCommand waits for a command executed in pod to succeed with an exit code of 9
// error if the timeout is exceeded
func (c *Client) WaitForPodCommand(ns, name string, container string, timeout time.Duration, command ...string) error {
	if c.ApplyDryRun {
		return nil
	}
	start := time.Now()
	for {
		stdout, stderr, err := c.ExecutePodf(ns, name, container, command...)
		if err == nil {
			return nil
		}
		if start.Add(timeout).Before(time.Now()) {
			return fmt.Errorf("timeout exceeded waiting for %s stdout: %s, stderr: %s", name, stdout, stderr)
		}
		time.Sleep(5 * time.Second)
	}
}
