package kommons

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/flanksource/commons/console"
	apps "k8s.io/api/apps/v1"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	networking "k8s.io/api/networking/v1"
	rbac "k8s.io/api/rbac/v1"
	"k8s.io/apiextensions-apiserver/pkg/apis/apiextensions"
	apiextensionsv1beta1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	yamlutil "k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/yaml"
)

var KustomizedLabel = "kustomize/patched"

func IsKustomized(to *unstructured.Unstructured) bool {
	if _, ok := to.GetAnnotations()[KustomizedLabel]; ok {
		return true
	}
	return false
}

func ToJson(to *unstructured.Unstructured) string {
	if IsNil(to) {
		return ""
	}
	data, _ := to.MarshalJSON()
	return string(data)
}

func ToYaml(to *unstructured.Unstructured) string {
	if IsNil(to) {
		return ""
	}
	data, _ := yaml.Marshal(to)
	return string(data)
}

func AsDeployment(obj *unstructured.Unstructured) (*appsv1.Deployment, error) {
	if IsNil(obj) {
		return nil, nil
	}
	var deployment appsv1.Deployment
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(obj.Object, &deployment); err != nil {
		return nil, err
	}
	return &deployment, nil
}

func AsStatefulSet(obj *unstructured.Unstructured) (*appsv1.StatefulSet, error) {
	if IsNil(obj) {
		return nil, nil
	}
	var sts appsv1.StatefulSet
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(obj.Object, &sts); err != nil {
		return nil, err
	}
	return &sts, nil
}

func AsSecret(obj *unstructured.Unstructured) (*v1.Secret, error) {
	if IsNil(obj) {
		return nil, nil
	}
	var secret v1.Secret
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(obj.Object, &secret); err != nil {
		return nil, err
	}
	return &secret, nil
}

func AsService(obj *unstructured.Unstructured) (*v1.Service, error) {
	if IsNil(obj) {
		return nil, nil
	}
	var svc v1.Service
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(obj.Object, &svc); err != nil {
		return nil, err
	}
	return &svc, nil
}

func AsIngress(obj *unstructured.Unstructured) (*networking.Ingress, error) {
	if IsNil(obj) {
		return nil, nil
	}
	var ing networking.Ingress
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(obj.Object, &ing); err != nil {
		return nil, err
	}
	return &ing, nil
}

func AsRoleBinding(obj *unstructured.Unstructured) (*rbac.RoleBinding, error) {
	if IsNil(obj) {
		return nil, nil
	}
	var rb rbac.RoleBinding
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(obj.Object, &rb); err != nil {
		return nil, err
	}
	return &rb, nil
}

func AsClusterRoleBinding(obj *unstructured.Unstructured) (*rbac.ClusterRoleBinding, error) {
	if IsNil(obj) {
		return nil, nil
	}
	var crb rbac.ClusterRoleBinding
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(obj.Object, &crb); err != nil {
		return nil, err
	}
	return &crb, nil
}

func AsDaemonSet(obj *unstructured.Unstructured) (*appsv1.DaemonSet, error) {
	if IsNil(obj) {
		return nil, nil
	}
	var daemonset appsv1.DaemonSet
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(obj.Object, &daemonset); err != nil {
		return nil, err
	}
	return &daemonset, nil
}

func AsCustomResourceDefinition(obj *unstructured.Unstructured) (*apiextensions.CustomResourceDefinition, error) {
	if IsNil(obj) {
		return nil, nil
	}
	var crd apiextensions.CustomResourceDefinition
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(obj.Object, &crd); err != nil {
		return nil, err
	}
	return &crd, nil
}

func AsCustomResourceDefinitionV1Beta1(obj *unstructured.Unstructured) (*apiextensionsv1beta1.CustomResourceDefinition, error) {
	if IsNil(obj) {
		return nil, nil
	}
	var crd apiextensionsv1beta1.CustomResourceDefinition
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(obj.Object, &crd); err != nil {
		return nil, err
	}
	return &crd, nil
}

func AsPodTemplate(obj *unstructured.Unstructured) (*v1.PodTemplateSpec, error) {
	if IsNil(obj) {
		return nil, nil
	}
	var spec v1.PodTemplateSpec
	template, _, err := unstructured.NestedMap(obj.Object, "spec", "template")
	if err != nil {
		return nil, err
	}
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(template, &spec); err != nil {
		return nil, err
	}
	return &spec, nil
}

func UnwrapError(err error) string {
	if err == nil {
		return ""
	}
	switch err.(type) {
	case *errors.StatusError:
		return err.(*errors.StatusError).ErrStatus.Message
	}
	return err.Error()
}

func IsAPIResourceMissing(err error) bool {
	msg := UnwrapError(err)
	if msg == "" {
		return false
	}
	return strings.Contains(msg, "the server could not find the requested resource") ||
		strings.Contains(msg, "no matches for kind") ||
		(strings.Contains(msg, "is not recognized") && strings.Contains(msg, "kind"))
}

func GetValidName(name string) string {
	return strings.ReplaceAll(name, "_", "-")
}

func GetName(obj interface{}) Name {
	name := Name{}
	switch obj.(type) {
	case *unstructured.Unstructured:
		object := obj.(*unstructured.Unstructured)
		if object == nil || object.Object == nil {
			return name
		}
		name.Name = object.GetName()
		name.Namespace = object.GetNamespace()
	case metav1.ObjectMetaAccessor:
		object := obj.(metav1.ObjectMetaAccessor).GetObjectMeta()
		name.Name = object.GetName()
		name.Namespace = object.GetNamespace()
	}

	switch obj.(type) {
	case *unstructured.Unstructured:
		object := obj.(*unstructured.Unstructured)
		if object == nil || object.Object == nil {
			return name
		}
		name.Kind = object.GetKind()
	default:
		if t := reflect.TypeOf(obj); t.Kind() == reflect.Ptr {
			name.Kind = t.Elem().Name()
		} else {
			name.Kind = t.Name()
		}
	}

	return name
}

type Name struct {
	Name, Kind, Namespace string
}

func (n Name) String() string {
	if n.Namespace == "" {
		return fmt.Sprintf("%s/%s/%s", console.Bluef(n.Kind), console.Grayf("*"), console.LightWhitef(n.Name))
	}
	return fmt.Sprintf("%s/%s/%s", console.Bluef(n.Kind), console.Grayf(n.Namespace), console.LightWhitef(n.Name))
}

func (n Name) GetName() string {
	return n.Name
}
func (n Name) GetKind() string {
	return n.Kind
}

func (n Name) GetNamespace() string {
	return n.Namespace
}

type Kindable interface {
	GetKind() string
}

type Nameable interface {
	GetName() string
	GetNamespace() string
}

func Validate(object runtime.Object) error {
	switch object.(type) {
	case *unstructured.Unstructured:
		obj := object.(*unstructured.Unstructured)
		if obj == nil {
			return fmt.Errorf("empty pointer")
		}
		if obj.GetKind() == "" {
			return fmt.Errorf("%s/%s does not have a kind", obj.GetNamespace(), obj.GetName())
		}
		if obj.GetAPIVersion() == "" {
			return fmt.Errorf("%s/%s/%s does not have an apiVersion", obj.GetNamespace(), obj.GetKind(), obj.GetName())
		}

	}
	return nil
}

func IsNil(object runtime.Object) bool {
	if object == nil {
		return true
	}
	switch object.(type) {
	case *unstructured.Unstructured:
		obj := object.(*unstructured.Unstructured)
		if obj == nil {
			return true
		}
	}
	return false
}

func IsConfigMap(obj *unstructured.Unstructured) bool {
	return obj.GetKind() == "ConfigMap"
}

func IsSecret(obj *unstructured.Unstructured) bool {
	return obj.GetKind() == "Secret"
}

func IsPVC(obj *unstructured.Unstructured) bool {
	return obj.GetKind() == "PersistentVolumeClaim"
}

func IsService(obj *unstructured.Unstructured) bool {
	return obj.GetKind() == "Service"
}

func IsServiceAccount(obj *unstructured.Unstructured) bool {
	return obj.GetKind() == "ServiceAccount"
}

func IsIngress(obj *unstructured.Unstructured) bool {
	return obj.GetKind() == "Ingress"
}

// IsApp returns true if the obj is a Deployment, Statetefulset or DaemonSet
func IsApp(obj *unstructured.Unstructured) bool {
	return IsStatefulSet(obj) || IsDeployment(obj) || IsDaemonSet(obj)
}

func IsDeployment(obj *unstructured.Unstructured) bool {
	return obj.GetKind() == "Deployment"
}

func IsDaemonSet(obj *unstructured.Unstructured) bool {
	return obj.GetKind() == "DaemonSet"
}

func IsStatefulSet(obj *unstructured.Unstructured) bool {
	return obj.GetKind() == "StatefulSet"
}

func IsAnyRoleBinding(obj *unstructured.Unstructured) bool {
	return IsRoleBinding(obj) || IsClusterRoleBinding(obj)
}

func IsRoleBinding(obj *unstructured.Unstructured) bool {
	return obj.GetKind() == "RoleBinding"
}

func IsClusterRoleBinding(obj *unstructured.Unstructured) bool {
	return obj.GetKind() == "ClusterRoleBinding"
}

func IsClusterRole(obj *unstructured.Unstructured) bool {
	return obj.GetKind() == "ClusterRole"
}

func IsRole(obj *unstructured.Unstructured) bool {
	return obj.GetKind() == "Role"
}

func IsCronJob(obj *unstructured.Unstructured) bool {
	return obj.GetKind() == "CronJob"
}

func IsCanary(obj *unstructured.Unstructured) bool {
	return obj.GetKind() == "Canary"
}

func IsCustomResourceDefinitionV1Beta1(obj *unstructured.Unstructured) bool {
	return obj.GetKind() == "CustomResourceDefinition" && obj.GetAPIVersion() == "apiextensions.k8s.io/v1beta1"
}

func IsCustomResourceDefinition(obj *unstructured.Unstructured) bool {
	return obj.GetKind() == "CustomResourceDefinition"
}

func IsConstraintTemplate(obj *unstructured.Unstructured) bool {
	return obj.GetKind() == "ConstraintTemplate"
}

func IsElasticsearch(obj *unstructured.Unstructured) bool {
	return obj.GetKind() == "Elasticsearch"
}

func IsKibana(obj *unstructured.Unstructured) bool {
	return obj.GetKind() == "Kibana"
}

func IsRedisFailover(obj *unstructured.Unstructured) bool {
	return obj.GetKind() == "RedisFailover"
}

func IsPostgresql(obj *unstructured.Unstructured) bool {
	return strings.ToLower(obj.GetKind()) == "postgresql"
}

func IsPostgresqlDB(obj *unstructured.Unstructured) bool {
	return strings.ToLower(obj.GetKind()) == "postgresqldb"
}

func IsMongoDB(obj *unstructured.Unstructured) bool {
	return obj.GetKind() == "PerconaServerMongoDB"
}

func IsKafka(obj *unstructured.Unstructured) bool {
	return obj.GetKind() == "Kafka"
}

func IsBuilder(obj *unstructured.Unstructured) bool {
	return obj.GetKind() == "Builder"
}

func IsImage(obj *unstructured.Unstructured) bool {
	return obj.GetKind() == "Image"
}

func NewDeployment(ns, name, image string, labels map[string]string, port int32, args ...string) *apps.Deployment {
	if labels == nil {
		labels = make(map[string]string)
	}
	if len(labels) == 0 {
		labels["app"] = name
	}
	replicas := int32(1)

	deployment := apps.Deployment{
		ObjectMeta: NewObjectMeta(ns, name),
		Spec: apps.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: labels,
			},
			Template: v1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
				},
				Spec: v1.PodSpec{
					Containers: []v1.Container{
						{
							Name:            name,
							Image:           image,
							ImagePullPolicy: "IfNotPresent",
							Ports: []v1.ContainerPort{
								v1.ContainerPort{
									ContainerPort: port,
								},
							},
							Args:      args,
							Resources: LowResourceRequirements(),
						},
					},
				},
			},
		},
	}
	deployment.Kind = "Deployment"
	deployment.APIVersion = "apps/v1"
	return &deployment
}

func NewObjectMeta(ns, name string) metav1.ObjectMeta {
	return metav1.ObjectMeta{
		Name:      name,
		Namespace: ns,
	}
}

func LowResourceRequirements() v1.ResourceRequirements {
	return v1.ResourceRequirements{
		Limits: v1.ResourceList{
			v1.ResourceMemory: resource.MustParse("512Mi"),
			v1.ResourceCPU:    resource.MustParse("500m"),
		},
		Requests: v1.ResourceList{
			v1.ResourceMemory: resource.MustParse("128Mi"),
			v1.ResourceCPU:    resource.MustParse("10m"),
		},
	}
}

func IsPodCrashLoopBackoff(pod v1.Pod) bool {
	for _, status := range pod.Status.ContainerStatuses {
		if status.State.Waiting != nil && status.State.Waiting.Reason == "CrashLoopBackOff" {
			return true
		}
	}
	return false
}

func maxTime(t1 *time.Time, t2 time.Time) *time.Time {
	if t1 == nil {
		return &t2
	}
	if t1.Before(t2) {
		return &t2
	}
	return t1
}

func GetPodStatus(pod v1.Pod) string {
	if IsPodCrashLoopBackoff(pod) {
		return "CrashLoopBackOff"
	}
	if pod.Status.Phase == v1.PodFailed {
		return "Failed"
	}
	if pod.DeletionTimestamp != nil && !pod.DeletionTimestamp.IsZero() {
		return "Terminating"
	}
	return string(pod.Status.Phase)
}

func GetLastRestartTime(pod v1.Pod) *time.Time {
	var max *time.Time
	for _, status := range pod.Status.ContainerStatuses {
		if status.LastTerminationState.Terminated != nil {
			max = maxTime(max, status.LastTerminationState.Terminated.FinishedAt.Time)
		}
	}
	return max
}

func GetContainerStatus(pod v1.Pod) string {
	if IsPodHealthy(pod) {
		return ""
	}
	msg := ""
	for _, status := range pod.Status.ContainerStatuses {
		if status.State.Terminated != nil {
			terminated := status.State.Terminated
			msg += fmt.Sprintf("%s exit(%s):, %s %s", console.Bluef(status.Name), console.Redf("%d", terminated.ExitCode), terminated.Reason, console.DarkF(terminated.Message))
		} else if status.LastTerminationState.Terminated != nil {
			terminated := status.LastTerminationState.Terminated
			msg += fmt.Sprintf("%s exit(%s): %s %s", console.Bluef(status.Name), console.Redf("%d", terminated.ExitCode), terminated.Reason, console.DarkF(terminated.Message))
		}
	}
	return msg
}

func IsPodHealthy(pod v1.Pod) bool {
	if pod.Status.Phase == v1.PodSucceeded {
		for _, status := range pod.Status.ContainerStatuses {
			if status.State.Terminated != nil && status.State.Terminated.ExitCode != 0 {
				return false
			}
		}
		return true
	}

	if pod.Status.Phase == v1.PodFailed || IsPodCrashLoopBackoff(pod) {
		return false
	}

	for _, condition := range pod.Status.Conditions {
		if condition.Status == v1.ConditionFalse {
			return false
		}
	}

	return pod.Status.Phase == v1.PodRunning
}

func IsPodFinished(pod v1.Pod) bool {
	return pod.Status.Phase == v1.PodSucceeded || pod.Status.Phase == v1.PodFailed
}

func IsPodPending(pod v1.Pod) bool {
	return pod.Status.Phase == v1.PodPending
}

func IsPodReady(pod v1.Pod) bool {
	for _, condition := range pod.Status.Conditions {
		if condition.Type == v1.PodReady && condition.Status == v1.ConditionTrue {
			return true
		}
	}
	return false
}

func IsMasterNode(node v1.Node) bool {
	_, ok := node.Labels["node-role.kubernetes.io/master"]
	return ok
}

func IsDeleted(object metav1.Object) bool {
	return object.GetDeletionTimestamp() != nil && !object.GetDeletionTimestamp().IsZero()
}

func IsPodDaemonSet(pod v1.Pod) bool {
	controllerRef := metav1.GetControllerOf(&pod)
	return controllerRef != nil && controllerRef.Kind == apps.SchemeGroupVersion.WithKind("DaemonSet").Kind
}

// IsStaticPod returns true if the pod is static i.e. declared in /etc/kubernetes/manifests and read directly by the kubelet
func IsStaticPod(pod v1.Pod) bool {
	for _, owner := range pod.GetOwnerReferences() {
		if owner.Kind == "Node" {
			return true
		}
	}
	return false
}

func GetNodeStatus(node v1.Node) string {
	s := ""
	for _, condition := range node.Status.Conditions {
		if condition.Status == v1.ConditionFalse {
			continue
		}
		if s != "" {
			s += ", "
		}
		s += string(condition.Type)
	}
	return s
}

type Health struct {
	RunningPods, PendingPods, ErrorPods, CrashLoopBackOff int
	ReadyNodes, UnreadyNodes                              int
	Error                                                 error
}

func (h Health) GetNonReadyPods() int {
	return h.PendingPods + h.ErrorPods + h.CrashLoopBackOff
}

func (h Health) IsDegradedComparedTo(h2 Health, tolerance int) bool {
	if h.GetNonReadyPods()-h2.GetNonReadyPods() > tolerance {
		return true
	}
	if h2.RunningPods-h.RunningPods > tolerance {
		return true
	}
	if h.UnreadyNodes-h2.UnreadyNodes > 0 {
		return true
	}

	return false
}

func (h Health) String() string {
	return fmt.Sprintf("pods(running=%d, pending=%s, crashloop=%s, error=%s)  nodes(ready=%d, notready=%s)",
		h.RunningPods, console.Yellowf("%d", h.PendingPods), console.Redf("%d", h.CrashLoopBackOff), console.Redf("%d", h.ErrorPods), h.ReadyNodes, console.Redf("%d", h.UnreadyNodes))
}

func GetUnstructuredObjects(data []byte) ([]*unstructured.Unstructured, error) {
	var items []*unstructured.Unstructured
	for _, chunk := range strings.Split(string(data), "---\n") {
		if strings.TrimSpace(chunk) == "" {
			continue
		}

		decoder := yamlutil.NewYAMLOrJSONDecoder(bytes.NewReader([]byte(chunk)), 1024)
		var resource *unstructured.Unstructured

		if err := decoder.Decode(&resource); err != nil {
			return nil, fmt.Errorf("error decoding %s: %s", chunk, err)
		}
		if resource != nil {
			items = append(items, resource)
		}
	}
	return items, nil
}

func GetUnstructuredObjectsFromJson(data []byte) ([]*unstructured.Unstructured, error) {
	var items []*unstructured.Unstructured
	if err := json.Unmarshal(data, &items); err != nil {
		return nil, err
	}
	return items, nil
}

// GetCurrentClusterNameFrom returns the name of the cluster associated with the currentContext of the
// specified kubeconfig file
func GetCurrentClusterNameFrom(kubeConfigPath string) string {
	config, err := clientcmd.LoadFromFile(kubeConfigPath)
	if err != nil {
		return err.Error()
	}
	ctx, ok := config.Contexts[config.CurrentContext]
	if !ok {
		return fmt.Sprintf("invalid context name: %s", config.CurrentContext)
	}
	// we strip the prefix that kind automatically adds to cluster names
	return strings.Replace(ctx.Cluster, "kind-", "", 1)
}

func RemoveTaint(taints []v1.Taint, name string) []v1.Taint {
	list := []v1.Taint{}
	for _, taint := range taints {
		if taint.Key != name {
			list = append(list, taint)
		}
	}
	return list
}

func HasTaint(node v1.Node, name string) bool {
	for _, taint := range node.Spec.Taints {
		if taint.Key == name {
			return true
		}
	}
	return false
}
