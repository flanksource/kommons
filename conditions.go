package kommons

import (
	"encoding/json"

	kommonsv1 "github.com/flanksource/kommons/api/v1"
	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

type IsType func(*unstructured.Unstructured) bool

var (
	TrivialTypes = map[string]IsType{
		"ConfigMap":             IsConfigMap,
		"ClusterRole":           IsClusterRole,
		"ClusterRoleBinding":    IsClusterRoleBinding,
		"PersistentVolumeClaim": IsPVC,
		"Role":                  IsRole,
		"RoleBinding":           IsRoleBinding,
		"Secret":                IsSecret,
	}
)

func (c *Client) IsTrivialType(item *unstructured.Unstructured) bool {
	for _, v := range TrivialTypes {
		if v(item) {
			return false
		}
	}
	return true
}

func (c *Client) GetConditions(item *unstructured.Unstructured) (kommonsv1.ConditionList, error) {
	if item == nil {
		return nil, errors.Errorf("could not get conditions for nil object")
	}

	status, ok := item.Object["status"].(map[string]interface{})
	if !ok {
		return kommonsv1.ConditionList{}, nil
	}

	conditions, ok := status["conditions"].([]interface{})
	if !ok || len(conditions) == 0 {
		return kommonsv1.ConditionList{}, nil
	}

	js, err := json.Marshal(conditions)
	if err != nil {
		return nil, errors.Wrap(err, "failed to marshal conditions")
	}

	commonConditions := kommonsv1.ConditionList{}
	if err := json.Unmarshal(js, &commonConditions); err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal conditions")
	}

	return commonConditions, nil
}

func (c *Client) SetCondition(item *unstructured.Unstructured, kind, status string) error {
	conditions, err := c.GetConditions(item)
	if err != nil {
		return errors.Wrap(err, "failed to get conditions")
	}

	found := false
	changed := false
	now := metav1.Now()

	for _, condition := range conditions {
		if condition.Type == kind {
			found = true
			if condition.Status != status {
				changed = true
				condition.Status = status
				condition.LastTransitionTime = &now
			}
			condition.LastHeartbeatTime = &now
		}
	}

	if !found {
		changed = true
		condition := kommonsv1.Condition{
			Type:               kind,
			Status:             status,
			LastHeartbeatTime:  &now,
			LastTransitionTime: &now,
		}
		conditions = append(conditions, condition)
	}

	if !changed {
		return nil
	}

	conditionsJson, err := json.Marshal(conditions)
	if err != nil {
		return errors.Wrap(err, "failed to encode conditions to json")
	}
	conditionsList := []interface{}{}
	if err := json.Unmarshal(conditionsJson, &conditionsList); err != nil {
		return errors.Wrap(err, "failed to decode conditions json")
	}

	itemStatus, ok := item.Object["status"].(map[string]interface{})
	if !ok {
		itemStatus = map[string]interface{}{}
	}
	itemStatus["conditions"] = conditionsList
	item.Object["status"] = itemStatus

	if err := c.Update(item.GetNamespace(), item); err != nil {
		return errors.Wrap(err, "failed to apply status")
	}
	return nil
}
