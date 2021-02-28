package kommons

import (
	"bytes"
	"context"
	"fmt"
	"io/ioutil"
	"reflect"
	"time"

	perrors "github.com/pkg/errors"
	"gopkg.in/flanksource/yaml.v3"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer/protobuf"
	"k8s.io/client-go/rest"
)

func (c *Client) trace(msg string, objects ...runtime.Object) {
	if !c.Trace {
		return
	}
	for _, obj := range objects {
		data, err := yaml.Marshal(obj)
		if err != nil {
			c.Errorf("Error tracing %s", err)
		} else {
			fmt.Printf("%s\n%s", msg, string(data))
		}
	}
}

func (c *Client) decodeProtobufResource(kind string, object runtime.Object, message []byte) (*protobuf.Serializer, error) {
	rm, err := c.GetRestMapper()
	if err != nil {
		return nil, perrors.Wrap(err, "failed to get rest mapper")
	}
	gvks, err := rm.KindsFor(schema.GroupVersionResource{
		Resource: kind,
	})
	if err != nil {
		return nil, perrors.Wrapf(err, "failed to get kind for %s", kind)
	}
	if len(gvks) == 0 {
		return nil, perrors.Errorf("no gvks returned for kind %s", kind)
	}

	for _, gvk := range gvks {
		runtimeScheme := runtime.NewScheme()
		runtimeScheme.AddKnownTypeWithName(gvk, object)
		protoSerializer := protobuf.NewSerializer(runtimeScheme, runtimeScheme)

		// Decode protobuf value to Go pv struct
		_, _, err = protoSerializer.Decode(message, &gvk, object)
		if err == nil {
			return protoSerializer, nil
		}
	}

	return nil, perrors.Errorf("failed to decode protobuf message into runtime object, failed to find any suitable gvk")
}

func read(req *rest.Request) string {
	stream, err := req.Stream(context.TODO())
	if err != nil {
		return fmt.Sprintf("Failed to stream logs %v", err)
	}
	data, err := ioutil.ReadAll(stream)
	if err != nil {
		return fmt.Sprintf("Failed to stream logs %v", err)
	}
	return string(data)
}

func safeString(buf *bytes.Buffer) string {
	if buf == nil || buf.Len() == 0 {
		return ""
	}
	return buf.String()
}

// templatizePatch takes a patch stream (possibly containing multiple
// YAML documents) and templatizes each.
// blank documents are skipped.
func templatizePatch(patch *[]byte) (*[]byte, error) {
	var result []byte
	remainingData := patch
	for {
		first, rest := getDocumentsFromYamlFile(*remainingData)
		remainingData = &rest
		if len(first) == 0 {
			continue
		}
		templated, err := templatizeDocument(first)
		if err != nil {
			return nil, err
		}
		if len(result) > 0 {
			result = append(result, []byte("---\n")...)
		}
		result = append(result, *templated...)
		if len(rest) == 0 {
			break
		}
	}
	return &result, nil
}

// templatizeDocument applies templating to a supplied YAML
// document via the templating functionality in
// "gopkg.in/flanksource/yaml.v3"
// NOTE: only the first YAML document in a stream will be processed.
func templatizeDocument(patch []byte) (*[]byte, error) {
	var body interface{}
	if err := yaml.Unmarshal(patch, &body); err != nil {
		return nil, err
	}
	if body == nil {
		return &[]byte{}, nil
	}
	templated, err := yaml.Marshal(body)
	if err != nil {
		return nil, err
	}
	return &templated, nil
}

// getDocumentsFromYamlFile returns the first YAML document
// from a stream and a byte slice containing the remainder of the stream.
// This is needed since yaml.v3 (and the flanksource derived yaml.v3) only
// unmarshalls the **first** document in a stream.
//
// (see https://pkg.go.dev/gopkg.in/flanksource/yaml.v3@v3.1.1?tab=doc#Unmarshal)
func getDocumentsFromYamlFile(yamlData []byte) (firstDoc []byte, rest []byte) {
	endIndex := bytes.Index(yamlData, []byte("---"))
	if endIndex == -1 {
		return yamlData, []byte{}
	}
	return yamlData[:endIndex], yamlData[endIndex+3:]
}

func decodeStringToTimeDuration(f reflect.Type, t reflect.Type, data interface{}) (interface{}, error) {
	if f.Kind() != reflect.String {
		return data, nil
	}
	if t != reflect.TypeOf(time.Duration(5)) {
		return data, nil
	}
	d, err := time.ParseDuration(data.(string))
	if err != nil {
		return data, fmt.Errorf("decodeStringToTimeDuration: Failed to parse duration: %v", err)
	}
	return d, nil
}

func decodeStringToDuration(f reflect.Type, t reflect.Type, data interface{}) (interface{}, error) {
	if f.Kind() != reflect.String {
		return data, nil
	}
	if t != reflect.TypeOf(metav1.Duration{Duration: time.Duration(5)}) {
		return data, nil
	}
	d, err := time.ParseDuration(data.(string))
	if err != nil {
		return data, fmt.Errorf("decodeStringToDuration: Failed to parse duration: %v", err)
	}
	return metav1.Duration{Duration: d}, nil
}

func decodeStringToTime(f reflect.Type, t reflect.Type, data interface{}) (interface{}, error) {
	if f.Kind() != reflect.String {
		return data, nil
	}
	if t != reflect.TypeOf(metav1.Time{Time: time.Now()}) {
		return data, nil
	}
	d, err := time.Parse(time.RFC3339, data.(string))
	if err != nil {
		return data, fmt.Errorf("decodeStringToTime: failed to decode to time: %v", err)
	}
	return metav1.Time{Time: d}, nil
}

func decodeStringToInt64(f reflect.Type, t reflect.Type, data interface{}) (interface{}, error) {
	if f.Kind() != reflect.String {
		return data, nil
	}
	if t.Kind() != reflect.Int64 {
		return data, nil
	}
	d, err := time.ParseDuration(data.(string))
	if err != nil {
		return data, fmt.Errorf("decodeStringToDuration: Failed to parse duration: %v", err)
	}
	return int64(d), nil
}
