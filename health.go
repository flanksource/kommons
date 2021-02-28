package kommons

import (
	"context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// PingMaster attempts to connect to the API server and list nodes and services
// to ensure the API server is ready to accept any traffic
func (c *Client) PingMaster() bool {
	client, err := c.GetClientset()
	if err != nil {
		c.Tracef("pingMaster: Failed to get clientset: %v", err)
		return false
	}

	nodes, err := client.CoreV1().Nodes().List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		c.Tracef("pingMaster: Failed to get nodes list: %v", err)
		return false
	}
	if nodes == nil && len(nodes.Items) == 0 {
		return false
	}

	_, err = client.CoreV1().ServiceAccounts("kube-system").Get(context.TODO(), "default", metav1.GetOptions{})
	if err != nil {
		c.Tracef("pingMaster: Failed to get service account: %v", err)
		return false
	}
	return true
}

func (c *Client) GetHealth() Health {
	health := Health{}
	client, err := c.GetClientset()
	if err != nil {
		return Health{Error: err}
	}
	pods, err := client.CoreV1().Pods("").List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return Health{Error: err}
	}

	for _, pod := range pods.Items {
		if IsDeleted(&pod) {
			continue
		}
		if pod.Spec.Priority != nil && *pod.Spec.Priority < 0 {
			continue
		}

		if IsPodCrashLoopBackoff(pod) {
			health.CrashLoopBackOff++
		} else if IsPodHealthy(pod) {
			health.RunningPods++
		} else if IsPodPending(pod) {
			health.PendingPods++
		} else {
			health.ErrorPods++
		}
	}
	return health
}
