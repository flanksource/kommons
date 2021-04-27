package kommons

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io"

	"net"
	"net/http"

	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	certs "github.com/flanksource/commons/certs"
	"github.com/flanksource/commons/files"
	"github.com/flanksource/commons/logger"
	"github.com/flanksource/commons/utils"
	"github.com/flanksource/kommons/etcd"
	"github.com/flanksource/kommons/kustomize"
	"github.com/flanksource/kommons/proxy"
	"github.com/pkg/errors"
	perrors "github.com/pkg/errors"
	batchv1 "k8s.io/api/batch/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	cliresource "k8s.io/cli-runtime/pkg/resource"
	"k8s.io/client-go/discovery/cached/disk"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/restmapper"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/transport"
)

var immutableAnnotations = []string{
	"cnrm.cloud.google.com/project-id",
	"deployment.kubernetes.io/revision",
	"flux.weave.works/sync-hwm",
}

type Client struct {
	logger.Logger
	GetKubeConfigBytes   func() ([]byte, error)
	GetRESTConfig        func() (*rest.Config, error)
	GetKustomizePatches  func() ([]string, error)
	ApplyDryRun          bool
	ApplyHook            ApplyHook
	ImmutableAnnotations []string
	Trace                bool

	client              *kubernetes.Clientset
	dynamicClient       dynamic.Interface
	restConfig          *rest.Config
	etcdClientGenerator *etcd.EtcdClientGenerator
	kustomizeManager    *kustomize.Manager
	restMapper          meta.RESTMapper
}

func NewClientFromDefaults(log logger.Logger) (*Client, error) {
	kubeconfig := os.Getenv("KUBECONFIG")
	if kubeconfig == "" {
		kubeconfig = os.ExpandEnv("$HOME/.kube/config")
	}

	if !files.Exists(kubeconfig) {
		if config, err := rest.InClusterConfig(); err == nil {
			return NewClient(config, log), nil
		} else {
			return nil, fmt.Errorf("cannot find kubeconfig")
		}
	}

	data, err := ioutil.ReadFile(kubeconfig)
	if err != nil {
		return nil, err
	}
	restConfig, err := clientcmd.RESTConfigFromKubeConfig(data)
	if err != nil {
		return nil, err
	}
	return NewClient(restConfig, log), nil
}

func NewClientFromBytes(kubeconfig []byte) (*Client, error) {
	client := &Client{
		ImmutableAnnotations: immutableAnnotations,
		Logger:               logger.StandardLogger(),
		GetKubeConfigBytes: func() ([]byte, error) {
			return kubeconfig, nil
		},
		GetKustomizePatches: func() ([]string, error) {
			return []string{}, nil
		},
	}
	client.GetRESTConfig = client.GetRESTConfigFromKubeconfig
	return client, nil
}

func NewClient(config *rest.Config, logger logger.Logger) *Client {
	return &Client{
		ImmutableAnnotations: immutableAnnotations,
		restConfig:           config,
		Logger:               logger,
		GetRESTConfig: func() (*rest.Config, error) {
			return config, nil
		},
		GetKustomizePatches: func() ([]string, error) {
			return []string{}, nil
		},
	}
}

func (c *Client) ResetConnection() {
	c.client = nil
	c.dynamicClient = nil
	c.restConfig = nil
}

func (c *Client) GetKustomize() (*kustomize.Manager, error) {
	if c.kustomizeManager != nil {
		return c.kustomizeManager, nil
	}
	dir, _ := ioutil.TempDir("", "karina-kustomize")
	patches, err := c.GetKustomizePatches()
	if err != nil {
		return nil, err
	}
	no := 1
	var (
		patchData *[]byte
		name      string
	)
	for _, patch := range patches {
		if files.Exists(patch) {
			name = filepath.Base(patch)
			patchBytes, err := ioutil.ReadFile(patch)
			if err != nil {
				return nil, err
			}
			patchData = &patchBytes
		} else {
			patchBytes := []byte(patch)
			patchData = &patchBytes
			name = fmt.Sprintf("patch-%d.yaml", no)
			no++
		}
		patchData, err = templatizePatch(patchData)
		if err != nil {
			return nil, perrors.WithMessagef(err, "syntax error when reading %s ", name)
		}
		if _, err := files.CopyFromReader(bytes.NewBuffer(*patchData), dir+"/"+name, 0644); err != nil {
			return nil, err
		}
	}
	kustomizeManager, err := kustomize.GetManager(dir)
	c.kustomizeManager = kustomizeManager
	return c.kustomizeManager, err
}

// GetDynamicClient creates a new k8s client
func (c *Client) GetDynamicClient() (dynamic.Interface, error) {
	if c.dynamicClient != nil {
		return c.dynamicClient, nil
	}
	cfg, err := c.GetRESTConfig()
	if err != nil {
		return nil, fmt.Errorf("getClientset: failed to get REST config: %v", err)
	}
	c.dynamicClient, err = dynamic.NewForConfig(cfg)
	return c.dynamicClient, err
}

// GetClientset creates a new k8s client
func (c *Client) GetClientset() (*kubernetes.Clientset, error) {
	if c.client != nil {
		return c.client, nil
	}

	cfg, err := c.GetRESTConfig()
	if err != nil {
		return nil, fmt.Errorf("getClientset: failed to get REST config: %v", err)
	}
	c.client, err = kubernetes.NewForConfig(cfg)
	return c.client, err
}

func (c *Client) GetRESTConfigFromKubeconfig() (*rest.Config, error) {
	if c.restConfig != nil {
		return c.restConfig, nil
	}
	data, err := c.GetKubeConfigBytes()
	if err != nil {
		return nil, fmt.Errorf("getRESTConfig: failed to get kubeconfig: %v", err)
	}
	if len(data) == 0 {
		return nil, fmt.Errorf("kubeConfig is empty")
	}

	c.restConfig, err = clientcmd.RESTConfigFromKubeConfig(data)
	return c.restConfig, err
}

func (c *Client) GetRESTConfigInCluster() (*rest.Config, error) {
	if c.restConfig != nil {
		return c.restConfig, nil
	}
	data, err := rest.InClusterConfig()
	if err != nil {
		return nil, fmt.Errorf("getRESTConfig: failed to get in cluster config: %v", err)
	}

	return data, nil
}

func (c *Client) GetRestMapper() (meta.RESTMapper, error) {
	if c.restMapper != nil {
		return c.restMapper, nil
	}

	config, _ := c.GetRESTConfig()

	// re-use kubectl cache
	host := config.Host
	host = strings.ReplaceAll(host, "https://", "")
	host = strings.ReplaceAll(host, "-", "_")
	host = strings.ReplaceAll(host, ":", "_")
	cacheDir := os.ExpandEnv("$HOME/.kube/cache/discovery/" + host)
	cache, err := disk.NewCachedDiscoveryClientForConfig(config, cacheDir, "", 10*time.Minute)
	if err != nil {
		return nil, err
	}
	c.restMapper = restmapper.NewDeferredDiscoveryRESTMapper(cache)
	return c.restMapper, err
}

func (c *Client) GetEtcdClientGenerator(ca *certs.Certificate) (*etcd.EtcdClientGenerator, error) {
	if c.etcdClientGenerator != nil {
		return c.etcdClientGenerator, nil
	}
	client, err := c.GetClientset()
	if err != nil {
		return nil, err
	}
	rest, _ := c.GetRESTConfig()
	caPool := x509.NewCertPool()
	caPool.AppendCertsFromPEM(ca.EncodedCertificate())
	cert, _ := tls.X509KeyPair(ca.EncodedCertificate(), ca.EncodedPrivateKey())
	return etcd.NewEtcdClientGenerator(client, rest, &tls.Config{
		RootCAs:      caPool,
		Certificates: []tls.Certificate{cert},
	}), nil
}

func (c *Client) Refresh(item *unstructured.Unstructured) (*unstructured.Unstructured, error) {
	return c.GetByKind(item.GetKind(), item.GetNamespace(), item.GetName())
}

func (c *Client) GetClientByKind(kind string) (dynamic.NamespaceableResourceInterface, error) {
	dynamicClient, err := c.GetDynamicClient()
	if err != nil {
		return nil, err
	}
	rm, _ := c.GetRestMapper()
	gvk, err := rm.KindFor(schema.GroupVersionResource{
		Resource: kind,
	})
	if err != nil {
		return nil, err
	}
	gk := schema.GroupKind{Group: gvk.Group, Kind: gvk.Kind}
	mapping, err := rm.RESTMapping(gk, gvk.Version)
	if err != nil {
		return nil, err
	}
	return dynamicClient.Resource(mapping.Resource), nil
}

func (c *Client) GetDynamicClientFor(namespace string, obj runtime.Object) (dynamic.ResourceInterface, *schema.GroupVersionResource, *unstructured.Unstructured, error) {
	if obj.GetObjectKind().GroupVersionKind().Kind == "" {
		return nil, nil, nil, fmt.Errorf("cannot apply object, missing kind: %v", obj)
	}
	dynamicClient, err := c.GetDynamicClient()
	if err != nil {
		return nil, nil, nil, perrors.Wrap(err, "failed to get dynamic client")
	}

	return c.getDynamicClientFor(dynamicClient, namespace, obj)
}

func (c *Client) GetDynamicClientForUser(namespace string, obj runtime.Object, user string) (dynamic.ResourceInterface, *schema.GroupVersionResource, *unstructured.Unstructured, error) {
	data, err := c.GetKubeConfigBytes()
	if err != nil {
		return nil, nil, nil, perrors.Wrap(err, "failed to get kubeconfig")
	}
	if len(data) == 0 {
		return nil, nil, nil, fmt.Errorf("kubeConfig is empty")
	}

	cfg, err := clientcmd.RESTConfigFromKubeConfig(data)
	if err != nil {
		return nil, nil, nil, perrors.Wrap(err, "failed to get RestConfig")
	}

	impersonate := transport.ImpersonationConfig{UserName: user}

	transportConfig, err := cfg.TransportConfig()
	if err != nil {
		return nil, nil, nil, perrors.Wrap(err, "failed to get TransportConfig")
	}
	tlsConfig, err := transport.TLSConfigFor(transportConfig)
	if err != nil {
		return nil, nil, nil, perrors.Wrap(err, "failed to get TLSConfig")
	}
	timeout := 5 * time.Second

	tr := &http.Transport{
		DialContext: (&net.Dialer{
			Timeout:   timeout,
			KeepAlive: 30 * time.Second,
			DualStack: false, // K8s do not work well with IPv6
		}).DialContext,
		TLSHandshakeTimeout:   timeout,
		ResponseHeaderTimeout: 10 * time.Second,
		MaxIdleConns:          10,
		MaxIdleConnsPerHost:   2,
		IdleConnTimeout:       20 * time.Second,
		TLSClientConfig:       tlsConfig,
	}

	cfg.Transport = transport.NewImpersonatingRoundTripper(impersonate, tr)
	cfg.TLSClientConfig = rest.TLSClientConfig{}
	dynamicClient, err := dynamic.NewForConfig(cfg)
	if err != nil {
		return nil, nil, nil, perrors.Wrap(err, "failed to get imporsonating config")
	}

	return c.getDynamicClientFor(dynamicClient, namespace, obj)
}

func (c *Client) WaitForRestMapping(obj runtime.Object, timeout time.Duration) (*meta.RESTMapping, error) {
	gvk := obj.GetObjectKind().GroupVersionKind()
	gk := schema.GroupKind{Group: gvk.Group, Kind: gvk.Kind}

	start := time.Now()
	for {
		rm, _ := c.GetRestMapper()
		mapping, err := rm.RESTMapping(gk, gvk.Version)
		if err == nil || c.ApplyDryRun {
			return mapping, err
		}
		if !meta.IsNoMatchError(err) {
			return nil, err
		}
		// flush rest mapper cache
		c.restMapper = nil
		if start.Add(2 * time.Minute).Before(time.Now()) {
			return nil, fmt.Errorf("timeout waiting for RESTMapping for group=%s kind=%s", gk.Group, gk.Kind)
		}
		time.Sleep(2 * time.Second)
	}

}

func (c *Client) getDynamicClientFor(dynamicClient dynamic.Interface, namespace string, obj runtime.Object) (dynamic.ResourceInterface, *schema.GroupVersionResource, *unstructured.Unstructured, error) {
	mapping, err := c.WaitForRestMapping(obj, 2*time.Minute)
	if err != nil {
		return nil, nil, nil, err
	}

	resource := mapping.Resource

	convertedObj, err := runtime.DefaultUnstructuredConverter.ToUnstructured(obj)
	if err != nil {
		return nil, nil, nil, perrors.Wrapf(err, "failed to convert %s", obj.GetObjectKind())
	}

	unstructuredObj := &unstructured.Unstructured{Object: convertedObj}

	if mapping.Scope == meta.RESTScopeRoot {
		return dynamicClient.Resource(mapping.Resource), &resource, unstructuredObj, nil
	}
	if namespace == "" {
		namespace = unstructuredObj.GetNamespace()
	}
	return dynamicClient.Resource(mapping.Resource).Namespace(namespace), &resource, unstructuredObj, nil
}

func (c *Client) GetRestClient(obj unstructured.Unstructured) (*cliresource.Helper, error) {
	rm, _ := c.GetRestMapper()
	restConfig, _ := c.GetRESTConfig()
	// Get some metadata needed to make the REST request.
	gvk := obj.GetObjectKind().GroupVersionKind()
	gk := schema.GroupKind{Group: gvk.Group, Kind: gvk.Kind}
	mapping, err := rm.RESTMapping(gk, gvk.Version)
	if err != nil {
		return nil, err
	}

	gv := mapping.GroupVersionKind.GroupVersion()
	restConfig.ContentConfig = cliresource.UnstructuredPlusDefaultContentConfig()
	restConfig.GroupVersion = &gv
	if len(gv.Group) == 0 {
		restConfig.APIPath = "/api"
	} else {
		restConfig.APIPath = "/apis"
	}

	restClient, err := rest.RESTClientFor(restConfig)
	if err != nil {
		return nil, err
	}

	return cliresource.NewHelper(restClient, mapping), nil
}

func (c *Client) GetProxyDialer(p proxy.Proxy) (*proxy.Dialer, error) {
	clientset, err := c.GetClientset()
	if err != nil {
		return nil, err
	}

	restConfig, err := c.GetRESTConfig()
	if err != nil {
		return nil, err
	}

	return proxy.NewDialer(p, clientset, restConfig)
}

func (c *Client) Update(namespace string, item runtime.Object) error {
	client, _, unstructuredObject, err := c.GetDynamicClientFor(namespace, item)
	if err != nil {
		return errors.Wrap(err, "failed to get dynamic client")
	}

	_, err = client.Update(context.TODO(), unstructuredObject, metav1.UpdateOptions{})
	return err
}

func (c *Client) GetEtcdClient(ctx context.Context) (*etcd.Client, error) {
	clientset, err := c.GetClientset()
	if err != nil {
		return nil, perrors.Wrap(err, "failed to get clientset")
	}
	secret, err := clientset.CoreV1().Secrets("kube-system").Get(context.TODO(), "etcd-certs", metav1.GetOptions{})
	if err != nil {
		return nil, perrors.Wrap(err, "failed to get secret etcd-certs in namespace kube-system")
	}
	cert, err := certs.DecodeCertificate(secret.Data["tls.crt"], secret.Data["tls.key"])
	if err != nil {
		return nil, perrors.Wrap(err, "failed to decode etcd certificates")
	}
	etcdClientGenerator, err := c.GetEtcdClientGenerator(cert)
	if err != nil {
		return nil, perrors.Wrap(err, "failed to get etcd client generator")
	}

	masterNode, err := c.GetMasterNode()
	if err != nil {
		return nil, perrors.Wrap(err, "failed to get master node")
	}
	etcdClient, err := etcdClientGenerator.ForNode(ctx, masterNode)
	if err != nil {
		return nil, perrors.Wrap(err, "failed to get etcd client")
	}

	return etcdClient, nil
}

func (c *Client) GetJobPod(namespace, jobName string) (string, error) {
	client, err := c.GetClientset()
	if err != nil {
		return "", err
	}
	jobs := client.BatchV1().Jobs(namespace)
	c.Debugf("Waiting for %s/%s to be running", namespace, jobName)

	start := time.Now()
	timeout := 1 * time.Minute
	for {
		job, err := jobs.Get(context.TODO(), jobName, metav1.GetOptions{})
		if err != nil {
			return "", err
		}
		controllerUid := job.Labels["controller-uid"]
		if controllerUid != "" {
			pods := client.CoreV1().Pods(namespace)
			podsByLabel, err := pods.List(context.TODO(), metav1.ListOptions{
				LabelSelector: fmt.Sprintf("controller-uid=%s", controllerUid),
			})
			if err != nil {
				return "", err
			}
			if len(podsByLabel.Items) > 0 {
				return podsByLabel.Items[0].Name, nil
			}
		}
		if start.Add(timeout).Before(time.Now()) {
			return "", fmt.Errorf("couldn't find pod of Job: %s/%s after %s", namespace, jobName, timeout)
		}
		time.Sleep(1 * time.Second)
	}

}

func (c *Client) GetPodLogs(namespace, podName, container string) (string, error) {
	client, err := c.GetClientset()
	if err != nil {
		return "", err
	}
	podLogOptions := v1.PodLogOptions{}
	if container != "" {
		podLogOptions.Container = container
	}
	req := client.CoreV1().Pods(namespace).GetLogs(podName, &podLogOptions)
	podLogs, err := req.Stream(context.TODO())
	if err != nil {
		return "", err
	}
	defer podLogs.Close()
	buf := new(bytes.Buffer)
	_, err = io.Copy(buf, podLogs)
	return buf.String(), nil
}

func (c *Client) StreamLogs(namespace, name string) error {
	client, err := c.GetClientset()
	if err != nil {
		return err
	}
	pods := client.CoreV1().Pods(namespace)
	pod, err := pods.Get(context.TODO(), name, metav1.GetOptions{})
	if err != nil {
		return err
	}
	c.Debugf("Waiting for %s/%s to be running", namespace, name)
	if err := c.WaitForPod(namespace, name, 120*time.Second, v1.PodRunning, v1.PodSucceeded); err != nil {
		return err
	}
	c.Debugf("%s/%s running, streaming logs", namespace, name)
	var wg sync.WaitGroup
	for _, container := range append(pod.Spec.Containers, pod.Spec.InitContainers...) {
		logs := pods.GetLogs(pod.Name, &v1.PodLogOptions{
			Container: container.Name,
		})

		prefix := pod.Name
		if len(pod.Spec.Containers) > 1 {
			prefix += "/" + container.Name
		}
		podLogs, err := logs.Stream(context.TODO())
		if err != nil {
			return err
		}
		wg.Add(1)
		go func() {
			defer podLogs.Close()
			defer wg.Done()

			scanner := bufio.NewScanner(podLogs)
			for scanner.Scan() {
				incoming := scanner.Bytes()
				buffer := make([]byte, len(incoming))
				copy(buffer, incoming)
				fmt.Printf("\x1b[38;5;244m[%s]\x1b[0m %s\n", prefix, string(buffer))
			}
		}()
	}
	wg.Wait()
	if err = c.WaitForPod(namespace, name, 300*time.Second, v1.PodSucceeded); err != nil {
		return err
	}
	pod, err = pods.Get(context.TODO(), name, metav1.GetOptions{})
	if err != nil {
		return err
	}
	if pod.Status.Phase == v1.PodSucceeded {
		return nil
	}
	return fmt.Errorf("pod did not finish successfully %s - %s", pod.Status.Phase, pod.Status.Message)
}

// TriggerCronJobManually creates a Job from the cronJobName passed to the function and return the created job's name
func (c *Client) TriggerCronJobManually(namespace, cronJobName string) (string, error) {
	client, err := c.GetClientset()
	if err != nil {
		return "", err
	}
	cronJob, err := client.BatchV1beta1().CronJobs(namespace).Get(context.TODO(), cronJobName, metav1.GetOptions{})
	if err != nil {
		return "", err
	}
	job, err := client.BatchV1().Jobs(namespace).Create(context.TODO(), &batchv1.Job{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Job",
			APIVersion: "batch/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: fmt.Sprintf("%s-manual-%s", cronJobName, utils.RandomString(3)),
		},
		Spec: cronJob.Spec.JobTemplate.Spec,
	}, metav1.CreateOptions{})
	if err != nil {
		return "", err
	}
	return job.Name, nil
}
