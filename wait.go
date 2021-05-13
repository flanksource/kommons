package kommons

import (
	"context"
	"fmt"
	"strings"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
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

func IsServiceReady(item *unstructured.Unstructured, client *Client) (bool, string) {
	serviceType := item.Object["spec"].(map[string]interface{})["type"]
	if serviceType == "LoadBalancer" {
		ingress, found, _ := unstructured.NestedSlice(item.Object, "status", "loadBalancer", "ingress")
		if !found || len(ingress) == 0 {
			return false, "⏳ waiting for LoadBalancerIP"
		}
		return true, ""
	} else {
		item, _ := client.GetByKind("Endpoints", item.GetNamespace(), item.GetName())
		if item == nil {
			return false, "⏳ waiting for the corresponding Endpoint"
		}
		return true, ""
	}
}

func IsDataContainerReady(item *unstructured.Unstructured) (bool, string) {
	data, found, _ := unstructured.NestedMap(item.Object, "data")
	if found && len(data) > 0 {
		return true, ""
	} else {
		return false, "⏳ waiting for data"
	}
}

func (c *Client) IsReady(item *unstructured.Unstructured) (bool, string) {
	if c.ApplyDryRun {
		return true, ""
	}
	if item == nil {
		return false, "⏳ waiting to be created"
	}
	c.Debugf("[%s] checking readiness", GetName(item))

	switch {
	case IsSecret(item) || IsConfigMap(item):
		return IsDataContainerReady(item)
	case IsService(item):
		return IsServiceReady(item, c)
	case IsApp(item):
		return IsAppReady(item)
	case IsElasticsearch(item):
		return c.IsElasticsearchReady(item)
	case IsKibana(item):
		return c.IsKibanaReady(item)
	case IsRedisFailover(item):
		return c.IsRedisFailoverReady(item)
	case IsPostgresqlDB(item):
		return c.IsPostgresqlDBReady(item)
	case IsPostgresql(item):
		return c.IsPostgresqlReady(item)
	case IsConstraintTemplate(item):
		return c.IsConstraintTemplateReady(item)
	case IsMongoDB(item):
		return c.IsMongoDBReady(item)
	case IsKafka(item):
		return c.IsConditionReadyTrue(item)
	case IsBuilder(item):
		return IsBuilderReady(item)
	case IsImage(item):
		return IsImageReady(item)
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

func IsBuilderReady(item *unstructured.Unstructured) (bool, string) {
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
	if status["latestImage"] == nil {
		return false, "⏳ waiting to become ready"
	}
	image := status["latestImage"].(string)
	if len(image) == 0 {
		return false, "⏳ waiting to become ready"
	}
	return true, ""
}

func IsImageReady(item *unstructured.Unstructured) (bool, string) {
	if item.Object["status"] == nil {
		return false, "⏳ waiting to become ready"
	}
	status := item.Object["status"].(map[string]interface{})
	if _, found := status["conditions"]; !found {
		return false, "⏳ waiting to become ready"
	}
	if status["latestImage"] == nil {
		return false, "⏳ waiting to become ready"
	}
	image := status["latestImage"].(string)
	if len(image) == 0 {
		return false, "⏳ waiting to become ready"
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

func (c *Client) IsMongoDBReady(item *unstructured.Unstructured) (bool, string) {
	status := item.Object["status"]

	if status == nil {
		return false, "⏳ waiting to become ready"
	}

	state := item.Object["status"].(map[string]interface{})["state"]
	if state != "ready" {
		return false, "⏳ waiting to become ready"
	}

	return true, ""
}

func (c *Client) IsConditionReadyTrue(item *unstructured.Unstructured) (bool, string) {
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
		if condition["type"] == "Ready" && condition["status"] == "True" {
			return true, ""
		}
	}
	return false, "⏳ waiting to become ready"
}

func (c *Client) IsElasticsearchReady(item *unstructured.Unstructured) (bool, string) {
	name := item.GetName()
	namespace := item.GetNamespace()

	stsName := fmt.Sprintf("%s-es-default", name)

	clientset, err := c.GetClientset()
	if err != nil {
		return false, fmt.Sprintf("failed to get clientset: %v", err)
	}

	sts, err := clientset.AppsV1().StatefulSets(namespace).Get(context.TODO(), stsName, metav1.GetOptions{})
	if err != nil {
		return false, fmt.Sprintf("failed to get sts %s: %v", stsName, err)
	}

	return IsStatefulSetReady(sts)
}

func (c *Client) IsKibanaReady(item *unstructured.Unstructured) (bool, string) {
	name := item.GetName()
	namespace := item.GetNamespace()

	kbName := fmt.Sprintf("%s-kb", name)

	clientset, err := c.GetClientset()
	if err != nil {
		return false, fmt.Sprintf("failed to get clientset: %v", err)
	}

	kb, err := clientset.AppsV1().Deployments(namespace).Get(context.TODO(), kbName, metav1.GetOptions{})
	if err != nil {
		return false, fmt.Sprintf("failed to get deployment %s: %v", kbName, err)
	}

	return IsDeploymentReady(kb)
}

func (c *Client) IsRedisFailoverReady(item *unstructured.Unstructured) (bool, string) {
	name := item.GetName()
	namespace := item.GetNamespace()

	stsName := fmt.Sprintf("rfr-%s", name)
	deplName := fmt.Sprintf("rfs-%s", name)

	clientset, err := c.GetClientset()
	if err != nil {
		return false, fmt.Sprintf("failed to get clientset: %v", err)
	}

	sts, err := clientset.AppsV1().StatefulSets(namespace).Get(context.TODO(), stsName, metav1.GetOptions{})
	if err != nil {
		return false, fmt.Sprintf("failed to get sts %s: %v", stsName, err)
	}

	depl, err := clientset.AppsV1().Deployments(namespace).Get(context.TODO(), deplName, metav1.GetOptions{})
	if err != nil {
		return false, fmt.Sprintf("failed to get deployment %s: %v", deplName, err)
	}

	stsReady, stsMsg := IsStatefulSetReady(sts)
	deplReady, deplMsg := IsDeploymentReady(depl)

	msgs := []string{}
	if !stsReady {
		msgs = append(msgs, stsMsg)
	}
	if !deplReady {
		msgs = append(msgs, deplMsg)
	}
	msg := strings.Join(msgs, "; ")
	return stsReady && deplReady, msg
}

func (c *Client) IsPostgresqlDBReady(item *unstructured.Unstructured) (bool, string) {
	// PostgresqlDB instances are backed by zalando postgres instances
	return c.isPostgresqlReady(item.GetNamespace(), "postgres-"+item.GetName())
}

func (c *Client) IsPostgresqlReady(item *unstructured.Unstructured) (bool, string) {
	return c.isPostgresqlReady(item.GetNamespace(), item.GetName())
}

func (c *Client) isPostgresqlReady(namespace, name string) (bool, string) {
	clientset, err := c.GetClientset()
	if err != nil {
		return false, fmt.Sprintf("failed to get clientset: %v", err)
	}

	// zalando postgres instances are backed by a stateful set
	sts, err := clientset.AppsV1().StatefulSets(namespace).Get(context.TODO(), name, metav1.GetOptions{})
	if err != nil {
		return false, fmt.Sprintf("⏳ waiting for statefulset")
	}

	if ready, msg := IsStatefulSetReady(sts); ready {
		// once the sts is up, check that the postgres instance is up and serving queries
		if err := c.WaitForPodCommand(namespace, name+"-0", "postgres", 30*time.Second, "su", "postgres", "-c", "psql -c 'SELECT 1;'"); err == nil {
			return true, ""
		} else {
			return false, "⏳ waiting for postgres to be running: " + err.Error()
		}
	} else {
		return ready, msg
	}
}

func IsStatefulSetReady(sts *appsv1.StatefulSet) (bool, string) {
	if *sts.Spec.Replicas == sts.Status.ReadyReplicas {
		return true, ""
	} else {
		return false, fmt.Sprintf("⏳ waiting for replicas to become ready %v/%v", sts.Status.ReadyReplicas, sts.Status.Replicas)
	}
}

func IsDeploymentReady(d *appsv1.Deployment) (bool, string) {
	if *d.Spec.Replicas == d.Status.ReadyReplicas {
		return true, ""
	} else {
		return false, fmt.Sprintf("⏳ waiting for replicas to become ready %v/%v", d.Status.ReadyReplicas, d.Status.Replicas)
	}
}

// WaitForJob waits for a job to finish (the condition type "Complete" has status of "True"), or returns an error if the timeout is exceeded
func (c *Client) WaitForJob(ns, name string, timeout time.Duration) error {
	if c.ApplyDryRun {
		return nil
	}
	client, err := c.GetClientset()
	if err != nil {
		return fmt.Errorf("waitForJob: Failed to get clientset: %v", err)
	}
	jobs := client.BatchV1().Jobs(ns)
	start := time.Now()
	for {
		job, _ := jobs.Get(context.TODO(), name, metav1.GetOptions{})
		if start.Add(timeout).Before(time.Now()) {
			return fmt.Errorf("timeout exceeded waiting for Job to finish")
		}
		if conditions := job.Status.Conditions; conditions != nil {
			for _, condition := range conditions {
				if condition.Type == batchv1.JobComplete && condition.Status == v1.ConditionTrue {
					return nil
				}
			}
		}
		time.Sleep(2 * time.Second)
	}
}

// WaitForPod waits for a pod to be in the specified phase, or returns an
// error if the timeout is exceeded
func (c *Client) WaitForPodByLabel(ns, label string, timeout time.Duration, phases ...v1.PodPhase) (*v1.Pod, error) {
	if c.ApplyDryRun {
		return &v1.Pod{}, nil
	}
	client, err := c.GetClientset()
	if err != nil {
		return nil, err
	}
	pods := client.CoreV1().Pods(ns)
	id := Name{Kind: "Pod", Namespace: ns, Name: label}
	start := time.Now()
	msg := false
	for {
		items, _ := pods.List(context.TODO(), metav1.ListOptions{LabelSelector: label})
		if items != nil && len(items.Items) > 0 {
			return &items.Items[0], nil
		}
		if start.Add(timeout).Before(time.Now()) {
			return nil, fmt.Errorf("timeout exceeded waiting for pod %s", id)
		}

		if !msg {
			c.Infof("%s ⏳ waiting for pod", id)
			msg = true
		}

		time.Sleep(2 * time.Second)
	}
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

// WaitForPodCommand waits for a command executed in pod to succeed with an exit code of 0
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
			return fmt.Errorf("timeout exceeded waiting for %s: %v, stdout: %s, stderr: %s", name, command, stdout, stderr)
		}
		time.Sleep(5 * time.Second)
	}
}
