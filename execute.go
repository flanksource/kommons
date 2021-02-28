package kommons

import (
	"bytes"
	"context"
	"fmt"
	"time"

	utils "github.com/flanksource/commons/utils"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/remotecommand"
)

// ExecutePodf runs the specified shell command inside a container of the specified pod
func (c *Client) ExecutePodf(namespace, pod, container string, command ...string) (string, string, error) {
	client, err := c.GetClientset()
	if err != nil {
		return "", "", fmt.Errorf("executePodf: Failed to get clientset: %v", err)
	}
	c.Debugf("[%s/%s/%s] %s", namespace, pod, container, command)
	const tty = false
	req := client.CoreV1().RESTClient().Post().
		Resource("pods").
		Name(pod).
		Namespace(namespace).
		SubResource("exec").
		Param("container", container)
	req.VersionedParams(&v1.PodExecOptions{
		Container: container,
		Command:   command,
		Stdin:     false,
		Stdout:    true,
		Stderr:    true,
		TTY:       tty,
	}, scheme.ParameterCodec)

	rc, err := c.GetRESTConfig()
	if err != nil {
		return "", "", fmt.Errorf("ExecutePodf: Failed to get REST config: %v", err)
	}

	exec, err := remotecommand.NewSPDYExecutor(rc, "POST", req.URL())
	if err != nil {
		return "", "", fmt.Errorf("ExecutePodf: Failed to get SPDY Executor: %v", err)
	}
	var stdout, stderr bytes.Buffer
	err = exec.Stream(remotecommand.StreamOptions{
		Stdin:  nil,
		Stdout: &stdout,
		Stderr: &stderr,
		Tty:    tty,
	})

	_stdout := safeString(&stdout)
	_stderr := safeString(&stderr)
	if err != nil {
		return _stdout, _stderr, fmt.Errorf("exec returned an error: %+v", err)
	}

	c.Tracef("[%s/%s/%s] %s => %s %s ", namespace, pod, container, command, _stdout, _stderr)
	return _stdout, _stderr, nil
}

// Executef runs the specified shell command on a node by creating
// a pre-scheduled pod that runs in the host namespace
func (c *Client) Executef(node string, timeout time.Duration, command string, args ...interface{}) (string, error) {
	client, err := c.GetClientset()
	if err != nil {
		return "", fmt.Errorf("executef: Failed to get clientset: %v", err)
	}
	pods := client.CoreV1().Pods("kube-system")
	command = fmt.Sprintf(command, args...)
	pod, err := pods.Create(context.TODO(), &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: fmt.Sprintf("command-%s-%s", node, utils.ShortTimestamp()),
		},
		Spec: NewCommandJob(node, command),
	}, metav1.CreateOptions{})
	c.Tracef("[%s] executing '%s' in pod %s", node, command, pod.Name)
	if err != nil {
		return "", fmt.Errorf("executef: Failed to create pod: %v", err)
	}
	defer pods.Delete(context.TODO(), pod.ObjectMeta.Name, metav1.DeleteOptions{}) // nolint: errcheck

	logs := pods.GetLogs(pod.Name, &v1.PodLogOptions{
		Container: pod.Spec.Containers[0].Name,
	})

	err = c.WaitForPod("kube-system", pod.ObjectMeta.Name, timeout, v1.PodSucceeded)
	logString := read(logs)
	if err != nil {
		return logString, fmt.Errorf("failed to execute command, pod did not complete: %v", err)
	}
	c.Tracef("[%s] stdout: %s", node, logString)
	return logString, nil
}

func NewCommandJob(node, command string) v1.PodSpec {
	yes := true
	return v1.PodSpec{
		RestartPolicy: v1.RestartPolicyNever,
		NodeName:      node,
		Volumes: []v1.Volume{{
			Name: "root",
			VolumeSource: v1.VolumeSource{
				HostPath: &v1.HostPathVolumeSource{
					Path: "/",
				},
			},
		}},
		Containers: []v1.Container{{
			Name:  "shell",
			Image: "docker.io/ubuntu:18.04",
			Command: []string{
				"sh",
				"-c",
				"chroot /chroot bash -c \"" + command + "\"",
			},
			VolumeMounts: []v1.VolumeMount{{
				Name:      "root",
				MountPath: "/chroot",
			}},
			SecurityContext: &v1.SecurityContext{
				Privileged: &yes,
			},
		}},
		Tolerations: []v1.Toleration{
			{
				// tolerate all values
				Operator: "Exists",
			},
		},
		HostNetwork: true,
		HostPID:     true,
		HostIPC:     true,
	}
}
