package kommons

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"time"

	perrors "github.com/pkg/errors"
	v1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
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

// Undelete an object by removing terminationTimestamp and gracePeriod
func (c *Client) Undelete(kind, name, namespace string, object RuntimeObjectWithMetadata) error {
	ctx := context.Background()

	apiResource, err := c.GetAPIResource(kind)
	if err != nil {
		return perrors.Wrap(err, "failed to get api resource")
	}

	etcdClient, err := c.GetEtcdClient(ctx)
	if err != nil {
		return perrors.Wrap(err, "failed to get etcd client")
	}

	var key string
	if apiResource.Namespaced {
		key = fmt.Sprintf("/registry/%s/%s/%s", apiResource.Name, namespace, name)
	} else {
		key = fmt.Sprintf("/registry/%s/%s", apiResource.Name, name)
	}
	resp, err := etcdClient.EtcdClient.Get(ctx, key)
	if err != nil {
		return err
	}
	if len(resp.Kvs) < 1 {
		return perrors.Errorf("no results found for key %s", key)
	}

	protoSerializer, err := c.decodeProtobufResource(kind, object, resp.Kvs[0].Value)
	if err != nil {
		return perrors.Wrap(err, "failed to decode protobuf resource")
	}

	objectMeta := object.GetObjectMeta()
	if objectMeta.GetDeletionTimestamp() == nil {
		return fmt.Errorf("%s [%s] is not in terminating status", apiResource.Kind, name)
	}

	objectMeta.SetDeletionTimestamp(nil)
	objectMeta.SetDeletionGracePeriodSeconds(nil)

	var fixedResource bytes.Buffer
	// Encode fixed resource to protobuf value
	err = protoSerializer.Encode(object, &fixedResource)
	if err != nil {
		return perrors.Wrap(err, "failed to encode protobuf")
	}

	_, err = etcdClient.EtcdClient.Put(ctx, key, fixedResource.String())
	if err != nil {
		return perrors.Wrap(err, "failed to update resource in etcd")
	}

	return nil
}

// Undelete an object by removing terminationTimestamp and gracePeriod
func (c *Client) UndeleteCRD(kind, name, namespace string) error {
	ctx := context.Background()

	apiResource, err := c.GetAPIResource(kind)
	if err != nil {
		return perrors.Wrap(err, "failed to get api resource")
	}

	etcdClient, err := c.GetEtcdClient(ctx)
	if err != nil {
		return perrors.Wrap(err, "failed to get etcd client")
	}

	var key string
	if apiResource.Namespaced {
		key = fmt.Sprintf("/registry/%s/%s/%s/%s", apiResource.Group, apiResource.Name, namespace, name)
	} else {
		key = fmt.Sprintf("/registry/%s/%s/%s", apiResource.Group, apiResource.Name, name)
	}
	resp, err := etcdClient.EtcdClient.Get(ctx, key)
	if err != nil {
		return err
	}
	if len(resp.Kvs) < 1 {
		return perrors.Errorf("no results found for key %s", key)
	}

	object := &unstructured.Unstructured{}
	if err := json.Unmarshal(resp.Kvs[0].Value, &object.Object); err != nil {
		return perrors.Wrap(err, "failed to unmarshal json crd")
	}

	if object.GetDeletionTimestamp() == nil {
		return fmt.Errorf("%s [%s] is not in terminating status", apiResource.Kind, name)
	}

	object.SetDeletionTimestamp(nil)
	object.SetDeletionGracePeriodSeconds(nil)

	fixedResource, err := json.Marshal(object.Object)
	if err != nil {
		return perrors.Wrap(err, "failed to encode json object")
	}
	_, err = etcdClient.EtcdClient.Put(ctx, key, string(fixedResource))
	if err != nil {
		return perrors.Wrap(err, "failed to update resource in etcd")
	}

	return nil
}

// Orphan an object by removing ownerReferences
func (c *Client) Orphan(kind, name, namespace string, object RuntimeObjectWithMetadata) error {
	ctx := context.Background()

	apiResource, err := c.GetAPIResource(kind)
	if err != nil {
		return perrors.Wrap(err, "failed to get api resource")
	}

	etcdClient, err := c.GetEtcdClient(ctx)
	if err != nil {
		return perrors.Wrap(err, "failed to get etcd client")
	}

	var key string
	if apiResource.Namespaced {
		key = fmt.Sprintf("/registry/%s/%s/%s", apiResource.Name, namespace, name)
	} else {
		key = fmt.Sprintf("/registry/%s/%s", apiResource.Name, name)
	}
	resp, err := etcdClient.EtcdClient.Get(ctx, key)
	if err != nil {
		return err
	}
	if len(resp.Kvs) < 1 {
		return perrors.Errorf("no results found for key %s", key)
	}

	protoSerializer, err := c.decodeProtobufResource(kind, object, resp.Kvs[0].Value)
	if err != nil {
		return perrors.Wrap(err, "failed to decode protobuf resource")
	}

	objectMeta := object.GetObjectMeta()
	ownerReferences := objectMeta.GetOwnerReferences()
	if len(ownerReferences) == 0 {
		return fmt.Errorf("%s [%s] has no ownerReferences", apiResource.Kind, name)
	}

	objectMeta.SetOwnerReferences([]metav1.OwnerReference{})

	var fixedResource bytes.Buffer
	// Encode fixed resource to protobuf value
	err = protoSerializer.Encode(object, &fixedResource)
	if err != nil {
		return perrors.Wrap(err, "failed to encode protobuf")
	}

	_, err = etcdClient.EtcdClient.Put(ctx, key, fixedResource.String())
	if err != nil {
		return perrors.Wrap(err, "failed to update resource in etcd")
	}

	return nil
}

// Orphan an object by removing ownerReferences
func (c *Client) OrphanCRD(kind, name, namespace string) error {
	ctx := context.Background()

	apiResource, err := c.GetAPIResource(kind)
	if err != nil {
		return perrors.Wrap(err, "failed to get api resource")
	}

	etcdClient, err := c.GetEtcdClient(ctx)
	if err != nil {
		return perrors.Wrap(err, "failed to get etcd client")
	}

	var key string
	if apiResource.Namespaced {
		key = fmt.Sprintf("/registry/%s/%s/%s/%s", apiResource.Group, apiResource.Name, namespace, name)
	} else {
		key = fmt.Sprintf("/registry/%s/%s/%s", apiResource.Group, apiResource.Name, name)
	}
	resp, err := etcdClient.EtcdClient.Get(ctx, key)
	if err != nil {
		return err
	}
	if len(resp.Kvs) < 1 {
		return perrors.Errorf("no results found for key %s", key)
	}

	object := &unstructured.Unstructured{}
	if err := json.Unmarshal(resp.Kvs[0].Value, &object.Object); err != nil {
		return perrors.Wrap(err, "failed to unmarshal json crd")
	}

	if len(object.GetOwnerReferences()) == 0 {
		return fmt.Errorf("%s [%s] has no ownerReferences", apiResource.Kind, name)
	}

	object.SetOwnerReferences([]metav1.OwnerReference{})

	fixedResource, err := json.Marshal(object.Object)
	if err != nil {
		return perrors.Wrap(err, "failed to encode json object")
	}
	_, err = etcdClient.EtcdClient.Put(ctx, key, string(fixedResource))
	if err != nil {
		return perrors.Wrap(err, "failed to update resource in etcd")
	}

	return nil
}
