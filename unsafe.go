package kommons

import (
	"context"
	"fmt"
	"time"

	v1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Remove volume attachment
func (c *Client) RemoveVolumeAttachment(va storagev1.VolumeAttachment) error {
	k8s, err := c.GetClientset()
	if err != nil {
		return fmt.Errorf("failed to get clientset: %v", err)
	}

	volumeAPI := k8s.StorageV1().VolumeAttachments()

	if len(va.Finalizers) > 0 {
		va.Finalizers = []string{}
		if _, err := volumeAPI.Update(context.TODO(), &va, metav1.UpdateOptions{}); err != nil {
			return fmt.Errorf("failed to remove finalizers from volume attachment %s: %v", va.Name, err)
		}
	}

	c.Infof("Removing volume attachment %s", va.Name)

	if err := volumeAPI.Delete(context.TODO(), va.Name, metav1.DeleteOptions{}); err != nil {
		return fmt.Errorf("failed to delete volume attachment %s: %v", va.Name, err)
	}

	return nil
}

// ForceDeleteNamespace deletes a namespace forcibly
// by overriding it's finalizers first
func (c *Client) ForceDeleteNamespace(ns string, timeout time.Duration) error {
	c.Warnf("Clearing finalizers for %v", ns)
	k8s, err := c.GetClientset()
	if err != nil {
		return fmt.Errorf("ForceDeleteNamespace: failed to get client set: %v", err)
	}

	namespace, err := k8s.CoreV1().Namespaces().Get(context.TODO(), ns, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("ForceDeleteNamespace: failed to get namespace: %v", err)
	}
	namespace.Spec.Finalizers = []v1.FinalizerName{}
	_, err = k8s.CoreV1().Namespaces().Finalize(context.TODO(), namespace, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("ForceDeleteNamespace: error removing finalisers: %v", err)
	}
	err = k8s.CoreV1().Namespaces().Delete(context.TODO(), ns, metav1.DeleteOptions{})
	if err != nil {
		return fmt.Errorf("ForceDeleteNamespace: error deleting namespace: %v", err)
	}
	return nil
}
