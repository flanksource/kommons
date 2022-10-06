package kustomize

import (
	"bytes"
	"io"
	"os"
	"reflect"
	"testing"

	"github.com/TomOnTime/utfutil"
	"github.com/flanksource/commons/logger"
	"github.com/stretchr/testify/assert"
	"golang.org/x/text/encoding"
	"golang.org/x/text/encoding/unicode"
	"golang.org/x/text/transform"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/kustomize/api/filesys"
	"sigs.k8s.io/kustomize/pkg/gvk"
	"sigs.k8s.io/kustomize/pkg/patch"
)

type UTFWriteCloser interface {
	Write(p []byte) (n int, err error)
	Close() error
}

type writeCloser struct {
	file   *os.File
	writer io.Writer
}

func (u writeCloser) Write(p []byte) (n int, err error) {
	return u.writer.Write(p)
}

func (u writeCloser) Close() error {
	if u.file != nil {
		return u.file.Close()
	}
	return nil
}

func newWriter(r io.Writer, d utfutil.EncodingHint) UTFWriteCloser {
	var encoder *encoding.Encoder
	switch d {
	case utfutil.UTF8:
		encoder = unicode.UTF8.NewEncoder()
	case utfutil.UTF16LE:
		winutf := unicode.UTF16(unicode.LittleEndian, unicode.ExpectBOM)
		encoder = winutf.NewEncoder()
	case utfutil.UTF16BE:
		utf16be := unicode.UTF16(unicode.BigEndian, unicode.ExpectBOM)
		encoder = utf16be.NewEncoder()
	}

	if rc, ok := r.(writeCloser); ok {
		rc.writer = transform.NewWriter(rc.file, unicode.BOMOverride(encoder))
		return rc
	}

	return writeCloser{
		writer: transform.NewWriter(r, unicode.BOMOverride(encoder)),
	}
}

func bytesWriter(b *bytes.Buffer, d utfutil.EncodingHint) io.Writer {
	return newWriter(b, d)
}

func bytesToCrlfOrCr(file []byte, os string, hint utfutil.EncodingHint) (string, error) {
	val := file
	val = bytes.ReplaceAll(val, []byte{13}, []byte{10})
	val = bytes.ReplaceAll(val, []byte{10, 10}, []byte{10})
	switch os {
	case "macos":
		val = bytes.ReplaceAll(val, []byte{10}, []byte{13})
	default:
		val = bytes.ReplaceAll(val, []byte{10}, []byte{13, 10})
	}
	var buf bytes.Buffer
	writer := bytesWriter(&buf, hint)
	bytesWritten, err := writer.Write(val)
	if err != nil {
		logger.Errorf("error reading from buffer. bytesWritten %v. err: %v", bytesWritten, err)
		return "", err
	}
	return buf.String(), nil
}

func handleError(err error, t *testing.T) {
	if err != nil {
		t.Errorf("error setting up test TestBytesToUtf8Lf: %v", err)
	}
}

func getFileBuffer(filePath string) ([]byte, error) {
	buf, err := os.ReadFile(filePath)
	if err != nil {
		logger.Errorf("error reading file %v: %v", filePath, err)
		return nil, err
	}
	return buf, nil
}

func TestBytesToUtf8Lf(t *testing.T) {

	// If tests are breaking, first check that the file read is UTF-8 with LF endings
	data, err := getFileBuffer("../testdata/test.yaml")
	if err != nil {
		t.Errorf("error setting up test TestBytesToUtf8Lf: %v", err)
		return
	}
	dataToString := string(data)
	utf16LeCrMac, err := bytesToCrlfOrCr(data, "macos", utfutil.UTF16LE)
	if err != nil {
		t.Errorf("error setting up test TestBytesToUtf8Lf: %v", err)
		return
	}
	utf16LeCrlfDefault, err := bytesToCrlfOrCr(data, "", utfutil.UTF16LE)
	if err != nil {
		t.Errorf("error setting up test TestBytesToUtf8Lf: %v", err)
		return
	}
	utf16BeCrMac, err := bytesToCrlfOrCr(data, "macos", utfutil.UTF16BE)
	if err != nil {
		t.Errorf("error setting up test TestBytesToUtf8Lf: %v", err)
		return
	}
	utf16BeCrlfDefault, err := bytesToCrlfOrCr(data, "", utfutil.UTF16BE)
	if err != nil {
		t.Errorf("error setting up test TestBytesToUtf8Lf: %v", err)
		return
	}

	type args struct {
		file []byte
	}
	tests := []struct {
		name    string
		args    args
		want    string
		wantErr bool
	}{
		{
			name: "f(data)->dataToString",
			args: args{
				file: data,
			},
			want:    dataToString,
			wantErr: false,
		},
		{
			name: "f(utf16LeCrMac)->dataToString",
			args: args{
				file: []byte(utf16LeCrMac),
			},
			want:    dataToString,
			wantErr: false,
		},
		{
			name: "f(utf16LeCrlfDefault)->dataToString",
			args: args{
				file: []byte(utf16LeCrlfDefault),
			},
			want:    dataToString,
			wantErr: false,
		},
		{
			name: "f(utf16BeCrMac)->dataToString",
			args: args{
				file: []byte(utf16BeCrMac),
			},
			want:    dataToString,
			wantErr: false,
		},
		{
			name: "f(utf16BeCrlfDefault)->dataToString",
			args: args{
				file: []byte(utf16BeCrlfDefault),
			},
			want:    dataToString,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hasContent := len(tt.want) > 0
			assert.Equal(t, true, hasContent)
			got, err := BytesToUtf8Lf(tt.args.file)
			if (err != nil) != tt.wantErr {
				t.Errorf("BytesToUtf8Lf() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("BytesToUtf8Lf() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGetUnstructuredObjects(t *testing.T) {
	itemOne := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"data": map[string]interface{}{
				"logging.level.org.springframework":     "DEBUG",
				"logging.level.org.springframework.web": "INFO",
				"some-key":                              "value-from-spring",
			},
			"kind": "ConfigMap",
			"metadata": map[string]interface{}{
				"name":      "spring-defaults-spring",
				"namespace": "default",
			},
		},
	}

	itemTwo := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"data": map[string]interface{}{
				"application.properties": "some-key=new-value\nnew-key=diff-value\n",
			},
			"kind": "ConfigMap",
			"metadata": map[string]interface{}{
				"name":      "sample",
				"namespace": "default",
			},
		},
	}

	// If tests are breaking, first check that the file read is UTF-8 with LF endings
	data, err := getFileBuffer("../testdata/test.yaml")
	if err != nil {
		t.Errorf("error setting up test TestBytesToUtf8Lf: %v", err)
		return
	}
	utf16LeCrMac, err := bytesToCrlfOrCr(data, "macos", utfutil.UTF16LE)
	if err != nil {
		t.Errorf("error setting up test TestBytesToUtf8Lf: %v", err)
		return
	}
	utf16LeCrlfDefault, err := bytesToCrlfOrCr(data, "", utfutil.UTF16LE)
	if err != nil {
		t.Errorf("error setting up test TestBytesToUtf8Lf: %v", err)
		return
	}
	utf16BeCrMac, err := bytesToCrlfOrCr(data, "macos", utfutil.UTF16BE)
	if err != nil {
		t.Errorf("error setting up test TestBytesToUtf8Lf: %v", err)
		return
	}
	utf16BeCrlfDefault, err := bytesToCrlfOrCr(data, "", utfutil.UTF16BE)
	if err != nil {
		t.Errorf("error setting up test TestBytesToUtf8Lf: %v", err)
		return
	}

	items := []runtime.Object{itemOne, itemTwo}
	type args struct {
		data []byte
	}
	tests := []struct {
		name        string
		args        args
		want        []runtime.Object
		testIndexes []int
		wantErr     bool
	}{
		{
			name: "f(data)->[]runtime.Object{ ...items }",
			args: args{
				data: data,
			},
			want:        items,
			testIndexes: []int{0, 1},
			wantErr:     false,
		},
		{
			name: "f(utf16LeCrMac)->[]runtime.Object{ ...items }",
			args: args{
				data: []byte(utf16LeCrMac),
			},
			want:        items,
			testIndexes: []int{0, 1},
			wantErr:     false,
		},
		{
			name: "f(utf16BeCrMac)->[]runtime.Object{ ...items }",
			args: args{
				data: []byte(utf16BeCrMac),
			},
			want:        items,
			testIndexes: []int{0, 1},
			wantErr:     false,
		},
		{
			name: "f(utf16LeCrlfDefault)->[]runtime.Object{ ...items }",
			args: args{
				data: []byte(utf16LeCrlfDefault),
			},
			want:        items,
			testIndexes: []int{0, 1},
			wantErr:     false,
		},
		{
			name: "f(utf16BeCrlfDefault)->[]runtime.Object{ ...items }",
			args: args{
				data: []byte(utf16BeCrlfDefault),
			},
			want:        items,
			testIndexes: []int{0, 1},
			wantErr:     false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := GetUnstructuredObjects(tt.args.data)

			if (err != nil) != tt.wantErr {
				t.Errorf("GetUnstructuredObjects() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			for _, index := range tt.testIndexes {
				if !reflect.DeepEqual(got[index], tt.want[index]) {
					t.Errorf("GetUnstructuredObjects() got = %v, want %v", got[index], tt.want[index])
				}
			}
		})
	}
}

func TestKustomize(t *testing.T) {
	itemOne := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"data": map[string]interface{}{
				"logging.level.org.springframework":     "DEBUG",
				"logging.level.org.springframework.web": "INFO",
				"some-key":                              "value-from-spring",
			},
			"kind": "ConfigMap",
			"metadata": map[string]interface{}{
				"name":      "spring-defaults-spring",
				"namespace": "default",
			},
		},
	}
	strategicPatch := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
			"metadata": map[string]interface{}{
				"name":      "spring-defaults-spring",
				"namespace": "default",
			},
			"data": map[string]interface{}{
				"logging.level.org.springframework": "ERROR",
			},
		},
	}
	json6902Patch := json6902{
		Target: &patch.Target{
			Namespace: "default",
			Name:      "spring-defaults-spring",
			Gvk: gvk.Gvk{
				Version: "v1",
				Kind:    "ConfigMap",
			},
		},
		Patch: "[{\"op\": \"replace\", \"path\": \"/data/logging.level.org.springframework\", \"value\": \"ERROR\"}]",
	}
	patchedItem := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"data": map[string]interface{}{
				"logging.level.org.springframework":     "ERROR",
				"logging.level.org.springframework.web": "INFO",
				"some-key":                              "value-from-spring",
			},
			"kind": "ConfigMap",
			"metadata": map[string]interface{}{
				"name":      "spring-defaults-spring",
				"namespace": "default",
				"annotations": map[string]interface{}{
					"kustomize/patched": "true",
				},
			},
		},
	}

	type args struct {
		namespace             string
		object                runtime.Object
		strategicMergePatches strategicMergeSlice
		json6902Patches       json6902Slice
	}
	tests := []struct {
		name    string
		args    args
		want    runtime.Object
		wantErr bool
	}{
		{
			name: "f(default, item) => []runtime.Object{item}",
			args: args{
				namespace:             "default",
				object:                itemOne,
				strategicMergePatches: nil,
				json6902Patches:       nil,
			},
			want:    itemOne,
			wantErr: false,
		},
		{
			name: "f(default, item) => []runtime.Object{strategicMergePatchedItem}",
			args: args{
				namespace:             "default",
				object:                itemOne,
				strategicMergePatches: strategicMergeSlice{strategicPatch},
				json6902Patches:       nil,
			},
			want:    patchedItem,
			wantErr: false,
		},
		{
			name: "f(default, item) => []runtime.Object{JSON6902PatchedItem}",
			args: args{
				namespace:             "default",
				object:                itemOne,
				strategicMergePatches: nil,
				json6902Patches:       json6902Slice{&json6902Patch},
			},
			want:    patchedItem,
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			manager := Manager{
				FileSystem: filesys.MakeFsInMemory(),
			}

			if tt.args.strategicMergePatches != nil {
				manager.StrategicMergePatches = tt.args.strategicMergePatches
			}
			if tt.args.json6902Patches != nil {
				manager.JSON6902Patches = tt.args.json6902Patches
			}

			got, err := manager.Kustomize(tt.args.namespace, tt.args.object)

			if (err != nil) != tt.wantErr {
				t.Errorf("Kustomize() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			testObject := got[0].(*unstructured.Unstructured).Object
			expected := tt.want.(*unstructured.Unstructured).Object
			if !reflect.DeepEqual(testObject, expected) {
				t.Errorf("Kustomize() got = %v, want %v", testObject, expected)
			}
		})
	}
}
