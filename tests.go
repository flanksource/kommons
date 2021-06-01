package kommons

import (
	"context"
	goerrors "errors"
	"fmt"
	"strings"

	"github.com/flanksource/commons/console"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/kubernetes"
	typedv1 "k8s.io/client-go/kubernetes/typed/core/v1"
)

func TestDeploy(client kubernetes.Interface, ns string, deploymentName string, t *console.TestResults) {
	if client == nil {
		t.Failf(deploymentName, "failed to get kubernetes client")
		return
	}

	deployment, err := client.AppsV1().Deployments(ns).Get(context.TODO(), deploymentName, metav1.GetOptions{})
	if errors.IsNotFound(err) {
		t.Failf(deploymentName, "deployment not found")
		return
	}
	labelMap, _ := metav1.LabelSelectorAsMap(deployment.Spec.Selector)
	TestPodsByLabels(client, deploymentName, ns, labelMap, t)
}

func TestDaemonSet(client kubernetes.Interface, ns string, name string, t *console.TestResults) {
	testName := name
	if client == nil {
		t.Failf(testName, "failed to get kubernetes client")
		return
	}
	daemonset, err := client.AppsV1().DaemonSets(ns).Get(context.TODO(), name, metav1.GetOptions{})
	if errors.IsNotFound(err) {
		t.Failf(testName, "daemonset not found")
		return
	}

	labelMap, _ := metav1.LabelSelectorAsMap(daemonset.Spec.Selector)
	TestPodsByLabels(client, testName, ns, labelMap, t)
}

func TestPodsByLabels(client kubernetes.Interface, testName string, ns string, labelMap map[string]string, t *console.TestResults) {
	if client == nil {
		t.Failf(testName, "failed to get kubernetes client")
		return
	}
	events := client.CoreV1().Events(ns)
	pods, _ := client.CoreV1().Pods(ns).List(context.TODO(), metav1.ListOptions{
		LabelSelector: labels.SelectorFromSet(labelMap).String(),
	})

	if len(pods.Items) == 0 {
		t.Failf(testName, "No pods found for %s", testName)
		return
	}
	fails := make([]error, 0)
	for _, pod := range pods.Items {
		if err := TestPod(testName, client, events, pod); err != nil {
			fails = append(fails, err)
		}
	}
	if len(fails) > 0 {
		message := fmt.Sprintf("[%s] %d of %d pods failed: ", testName, len(fails), len(pods.Items))
		for _, err := range fails {
			message += err.Error() + ". "
		}
		message = strings.TrimSuffix(message, " ")
		t.Failf(testName, message)
	}
	t.Passf(testName, "[%s] %d of %d pods passed", testName, len(pods.Items), len(pods.Items))
}

func TestPod(testName string, client kubernetes.Interface, events typedv1.EventInterface, pod v1.Pod) error {
	if client == nil {
		return fmt.Errorf("%s: failed to get kubernetes client", testName)
	}
	conditions := true
	// for _, condition := range pod.Status.Conditions {
	// 	if condition.Status == v1.ConditionFalse {
	// 		t.Failf(ns, "%s => %s: %s", pod.Name, condition.Type, condition.Message)
	// 		conditions = false
	// 	}
	// }
	if conditions && pod.Status.Phase == v1.PodRunning || pod.Status.Phase == v1.PodSucceeded {
		return nil
	} else {
		events, err := events.List(context.TODO(), metav1.ListOptions{
			FieldSelector: "involvedObject.name=" + pod.Name,
		})
		if err != nil {
			return goerrors.New(fmt.Sprintf("%s => %s, failed to get events %+v",
				pod.Name, pod.Status.Phase, err))
		}
		msg := ""
		for _, event := range events.Items {
			if event.Type == "Normal" {
				continue
			}
			msg += fmt.Sprintf("%s: %s ", event.Reason, event.Message)
		}
		if pod.Spec.NodeName == "" {
			msg += fmt.Sprintf("nodeName=<blank>: pod not scheduled yet ")
		}
		return goerrors.New(fmt.Sprintf("%s/%s=%s %s ", pod.Namespace, pod.Name, pod.Status.Phase, msg))
	}

	// check all pods running or completed with < 3 restarts
	// check unbound pvcs
	// check all pod liveness / readiness
}

func GetOwner(pod v1.Pod) string {
	return pod.GetName()
}

func TestNamespace(client kubernetes.Interface, ns string, t *console.TestResults) {
	if client == nil {
		t.Failf(ns, "failed to get kubernetes client")
		return
	}
	pods := client.CoreV1().Pods(ns)
	events := client.CoreV1().Events(ns)
	list, err := pods.List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		t.Failf(ns, "Failed to get pods for %s: %v", ns, err)
		return
	}

	if len(list.Items) == 0 {
		_, err := client.CoreV1().Namespaces().Get(context.TODO(), ns, metav1.GetOptions{})
		if errors.IsNotFound(err) {
			t.Failf(ns, "[%s] namespace not found, skipping", ns)
		} else {
			t.Failf(ns, "[%s] Expected pods but none running", ns)
		}
		return
	}
	failed := 0
	for _, pod := range list.Items {
		if err := TestPod(ns, client, events, pod); err != nil {
			t.Failf(ns, "%s %s", GetOwner(pod), err)
			failed++
		}
	}
	if failed > 0 {
		t.Failf(ns, "[%s] %d of %d pods failed: ", ns, failed, len(list.Items))
	} else {
		t.Passf(ns, "[%s] %d of %d pods passed", ns, len(list.Items), len(list.Items))
	}
}
