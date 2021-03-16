package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +kubebuilder:object:generate=true

type Condition struct {
	Type   string `json:"type" description:"type of common condition"`
	Status string `json:"status" description:"status of the condition, one of Ready, NotReady, Unknown"`

	// +optional
	Reason *string `json:"reason,omitempty" description:"one-word CamelCase reason for the condition's last transition"`
	// +optional
	Message *string `json:"message,omitempty" description:"human-readable message indicating details about last transition"`

	// +optional
	LastHeartbeatTime *metav1.Time `json:"lastHeartbeatTime,omitempty" description:"last time we got an update on a given condition"`
	// +optional
	LastTransitionTime *metav1.Time `json:"lastTransitionTime,omitempty" description:"last time the condition transit from one status to another"`
}

// +kubebuilder:object:generate=true

type ConditionList []Condition
