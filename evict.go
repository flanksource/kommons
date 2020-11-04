package kommons

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/flanksource/kommons/drain"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/util/wait"
)

func (c *Client) getDrainHelper() (*drain.Helper, error) {
	client, err := c.GetClientset()
	if err != nil {
		return nil, err
	}
	return &drain.Helper{
		Ctx:                 context.Background(),
		ErrOut:              os.Stderr,
		Out:                 os.Stdout,
		Client:              client,
		DeleteLocalData:     true,
		IgnoreAllDaemonSets: true,
		Timeout:             120 * time.Second,
	}, nil
}

func (c *Client) EvictPod(pod v1.Pod) error {
	if IsPodDaemonSet(pod) || IsPodFinished(pod) || IsDeleted(&pod) || IsStaticPod(pod) {
		return nil
	}
	client, err := c.GetClientset()
	if err != nil {
		return err
	}
	drainer, err := c.getDrainHelper()
	if err != nil {
		return err
	}
	replicas, err := c.GetPodReplicas(pod)
	if err != nil {
		return err
	}
	if replicas == 1 {
		if err := c.ScalePod(pod, int32(2)); err != nil {
			return err
		}
		defer func() {
			if err := c.ScalePod(pod, int32(1)); err != nil {
				c.Warnf("Failed to scale back pod: %v", err)
			}
		}()
	}

	if pod.ObjectMeta.Labels["spilo-role"] == "master" {
		c.Infof("Conducting failover of %s", pod.Name)
		var stdout, stderr string
		if stdout, stderr, err = c.ExecutePodf(pod.Namespace, pod.Name, "postgres", "curl", "-s", "http://localhost:8008/switchover", "-XPOST", fmt.Sprintf("-d {\"leader\":\"%s\"}", pod.Name)); err != nil {
			return fmt.Errorf("failed to failover instance, aborting: %v %s %s", err, stderr, stdout)
		}
		c.Infof("Failed over: %s %s", stdout, stderr)
	}
	if err := drainer.DeleteOrEvictPods(pod); err != nil {
		return err
	}

	pvcs := client.CoreV1().PersistentVolumeClaims(pod.Namespace)
	for _, vol := range pod.Spec.Volumes {
		if vol.PersistentVolumeClaim != nil {
			pvc, err := pvcs.Get(context.TODO(), vol.PersistentVolumeClaim.ClaimName, metav1.GetOptions{})
			if err != nil {
				return err
			}
			if pvc != nil && pvc.Spec.StorageClassName == nil || strings.Contains(*pvc.Spec.StorageClassName, "local") {
				c.Infof("[%s] deleting", pvc.Name)
				if err := pvcs.Delete(context.TODO(), pvc.Name, metav1.DeleteOptions{}); err != nil {
					return err
				}
				//nolint: errcheck
				wait.PollImmediate(1*time.Second, 2*time.Minute, func() (bool, error) {
					_, err := pvcs.Get(context.TODO(), pvc.Name, metav1.GetOptions{})
					return errors.IsNotFound(err), nil
				})
				pvc.ObjectMeta.SetAnnotations(nil)
				pvc.SetFinalizers([]string{})
				pvc.SetSelfLink("")
				pvc.SetResourceVersion("")
				pvc.Spec.VolumeName = ""
				new, err := pvcs.Create(context.TODO(), pvc, metav1.CreateOptions{})
				if err != nil {
					return err
				}
				c.Infof("Created new PVC %s -> %s", pvc.UID, new.UID)
			}
		}
	}
	return nil
}

func (c *Client) EvictNode(nodeName string) error {
	client, err := c.GetClientset()
	if err != nil {
		return nil
	}

	pods, err := client.CoreV1().Pods(metav1.NamespaceAll).List(context.TODO(), metav1.ListOptions{
		FieldSelector: fields.SelectorFromSet(fields.Set{"spec.nodeName": nodeName}).String(),
	})

	if err != nil {
		return err
	}

	for _, pod := range pods.Items {
		if err := c.EvictPod(pod); err != nil {
			return err
		}
	}
	return nil
}

func (c *Client) Cordon(nodeName string) error {
	c.Infof("[%s] cordoning", nodeName)

	client, err := c.GetClientset()
	if err != nil {
		return nil
	}
	nodes := client.CoreV1().Nodes()
	node, err := nodes.Get(context.TODO(), nodeName, metav1.GetOptions{})
	if err != nil {
		return err
	}
	node.Spec.Unschedulable = true
	_, err = nodes.Update(context.TODO(), node, metav1.UpdateOptions{})
	return err
}

func (c *Client) Uncordon(nodeName string) error {
	c.Infof("[%s] uncordoning", nodeName)
	client, err := c.GetClientset()
	if err != nil {
		return nil
	}
	nodes := client.CoreV1().Nodes()
	node, err := nodes.Get(context.TODO(), nodeName, metav1.GetOptions{})
	if err != nil {
		return err
	}
	node.Spec.Unschedulable = false
	_, err = nodes.Update(context.TODO(), node, metav1.UpdateOptions{})
	return err
}

func (c *Client) Drain(nodeName string, timeout time.Duration) error {
	c.Infof("[%s] draining", nodeName)
	if err := c.Cordon(nodeName); err != nil {
		return fmt.Errorf("error cordoning %s: %v", nodeName, err)
	}
	return c.EvictNode(nodeName)
}
