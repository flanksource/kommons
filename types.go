package kommons

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

type CRD struct {
	Kind       string                 `yaml:"kind,omitempty"`
	APIVersion string                 `yaml:"apiVersion,omitempty"`
	Metadata   Metadata               `yaml:"metadata,omitempty"`
	Spec       map[string]interface{} `yaml:"spec,omitempty"`
}

type Metadata struct {
	Name        string            `yaml:"name,omitempty"`
	Namespace   string            `yaml:"namespace,omitempty"`
	Labels      map[string]string `yaml:"labels,omitempty"`
	Annotations map[string]string `yaml:"annotations,omitempty"`
}

type DynamicKind struct {
	APIVersion, Kind string
}

func (dk DynamicKind) SetGroupVersionKind(gvk schema.GroupVersionKind) {}

func (dk DynamicKind) GroupVersionKind() schema.GroupVersionKind {
	return schema.FromAPIVersionAndKind(dk.APIVersion, dk.Kind)
}

type RuntimeObjectWithMetadata interface {
	GetObjectMeta() metav1.Object
	GetObjectKind() schema.ObjectKind
	DeepCopyObject() runtime.Object
}

// +kubebuilder:object:generate=true
type EnvVar struct {
	Name      string        `json:"name,omitempty" yaml:"name,omitempty" protobuf:"bytes,1,opt,name=name"`
	Value     string        `json:"value,omitempty" yaml:"value,omitempty" protobuf:"bytes,2,opt,name=value"`
	ValueFrom *EnvVarSource `json:"valueFrom,omitempty" yaml:"valueFrom,omitempty" protobuf:"bytes,3,opt,name=valueFrom"`
}

func (e EnvVar) IsEmpty() bool {
	return e.Value == "" && e.ValueFrom == nil
}

// +kubebuilder:object:generate=true
type EnvVarSource struct {
	ConfigMapKeyRef *ConfigMapKeySelector `json:"configMapKeyRef,omitempty" yaml:"configMapKeyRef,omitempty" protobuf:"bytes,3,opt,name=configMapKeyRef"`
	SecretKeyRef    *SecretKeySelector    `json:"secretKeyRef,omitempty" yaml:"secretKeyRef,omitempty" protobuf:"bytes,4,opt,name=secretKeyRef"`
}

// +kubebuilder:object:generate=true
type ConfigMapKeySelector struct {
	LocalObjectReference `json:",inline" yaml:",inline" protobuf:"bytes,1,opt,name=localObjectReference"`
	Key                  string `json:"key" yaml:"key" protobuf:"bytes,2,opt,name=key"`
	Optional             *bool  `json:"optional,omitempty" yaml:"optional,omitempty" protobuf:"varint,3,opt,name=optional"`
}

// +kubebuilder:object:generate=true
type SecretKeySelector struct {
	LocalObjectReference `json:",inline" yaml:",inline" protobuf:"bytes,1,opt,name=localObjectReference"`
	Key                  string `json:"key" yaml:"key" protobuf:"bytes,2,opt,name=key"`
	Optional             *bool  `json:"optional,omitempty" yaml:"optional,omitempty" protobuf:"varint,3,opt,name=optional"`
}

// +kubebuilder:object:generate=true
type LocalObjectReference struct {
	Name string `json:"name,omitempty" yaml:"name,omitempty" protobuf:"bytes,1,opt,name=name"`
}
