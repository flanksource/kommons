package kommons

import (
	"context"
	"fmt"
	"time"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func (c *Client) WaitForResource(kind, namespace, name string, timeout time.Duration) error {
	client, err := c.GetClientByKind(kind)
	if err != nil {
		return err
	}
	start := time.Now()
	for {
		item, err := client.Namespace(namespace).Get(context.TODO(), name, metav1.GetOptions{})

		if errors.IsNotFound(err) {
			return err
		}

		if start.Add(timeout).Before(time.Now()) {
			return fmt.Errorf("timeout exceeded waiting for %s/%s is %s, error: %v", kind, name, "", err)
		}

		if err != nil {
			c.Debugf("Unable to get %s/%s: %v", kind, name, err)
			c.Infof("Waiting for %s/%s/%s", kind, namespace, name)
			continue
		}
		if item.Object["status"] == nil {
			c.Infof("Waiting for %s/%s/%s", kind, namespace, name)
			continue
		}

		conditions := item.Object["status"].(map[string]interface{})["conditions"].([]interface{})

		for _, raw := range conditions {
			condition := raw.(map[string]interface{})
			c.Debugf("%s/%s is %s/%s: %s", namespace, name, condition["type"], condition["status"], condition["message"])
			if condition["type"] == "Ready" && condition["status"] == "True" {
				return nil
			}
		}
		c.Infof("Waiting for %s/%s/%s", kind, namespace, name)
		time.Sleep(1 * time.Second)
	}
}

// WaitForPod waits for a pod to be in the specified phase, or returns an
// error if the timeout is exceeded
func (c *Client) WaitForPod(ns, name string, timeout time.Duration, phases ...v1.PodPhase) error {
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
	client, err := c.GetClientset()
	if err != nil {
		return err
	}
	deployments := client.AppsV1().Deployments(ns)
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
			c.Infof("Waiting for %s/%s to have 1 ready replica", ns, name)
			msg = true
		}

		time.Sleep(2 * time.Second)
	}
}

// WaitForStatefulSet waits for a statefulset to have at least 1 ready replica, or returns an
// error if the timeout is exceeded
func (c *Client) WaitForStatefulSet(ns, name string, timeout time.Duration) error {
	client, err := c.GetClientset()
	if err != nil {
		return err
	}
	statefulsets := client.AppsV1().StatefulSets(ns)
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
			c.Infof("Waiting for %s/%s to have 1 ready replica", ns, name)
			msg = true
		}

		time.Sleep(2 * time.Second)
	}
}

// WaitForNode waits for a pod to be in the specified phase, or returns an
// error if the timeout is exceeded
func (c *Client) WaitForNode(name string, timeout time.Duration, condition v1.NodeConditionType, statii ...v1.ConditionStatus) (map[v1.NodeConditionType]v1.ConditionStatus, error) {
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
