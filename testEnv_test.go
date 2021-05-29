package kommons

import (
	"context"
	"github.com/flanksource/commons/files"
	"github.com/flanksource/commons/logger"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"os"
	"strings"
	"testing"
)

func TestInstallTestBin(t *testing.T) {
	type args struct {
		version string
	}
	tests := []struct {
		name    string
		args    args
		want    string
		wantErr bool
	}{
		{name: "base-working-1.19.2", args: args{version: "1.19.2"}, want: "/tmp/kubebuilder", wantErr: false},
		{name: "base-working-1.20.2", args: args{version: "1.20.2"}, want: "/tmp/kubebuilder", wantErr: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := InstallTestBin(tt.args.version)
			if (err != nil) != tt.wantErr {
				t.Errorf("SetupTestEnv() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if err != nil {
				return
			}
			defer os.RemoveAll(got)
			if !strings.HasPrefix(got, tt.want) {
				t.Errorf("SetupTestEnv() got = %v, want %v", got, tt.want)
				return
			}
			for _, bin := range []string{"etcd", "kube-apiserver", "kubectl"} {
				path := strings.Join([]string{got, bin}, "/")
				if !files.Exists(path) {
					t.Errorf("SetupTestEnv(), %v binary not installed", path)
				} else {
					t.Logf("File %v found", path)
				}
			}
		})
	}
}

func TestStartTestEnv(t *testing.T) {
	type args struct {
		version string
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		//Only ever have one case here, otherwise test will fail with multiple api servers trying to start
		{"1.19.2 Env", args{version: "1.19.2"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, bindir, err := StartTestEnv(tt.args.version)
			if (err != nil) != tt.wantErr {
				t.Errorf("StartTestEnv() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			defer os.RemoveAll(bindir)
			if err != nil {
				return
			}
			t.Logf("connecting to kube-apiserver at %v/%v", got.Host, got.APIPath)
			cfg := NewClient(got, logger.GetZapLogger())
			client, err := cfg.GetClientset()
			if err != nil || client == nil {
				t.Errorf("Could not create k8s client from rest config: %v", err)
				return
			}
			namespacelist, err := client.CoreV1().Namespaces().List(context.Background(), metav1.ListOptions{})
			if err != nil {
				t.Errorf("Failed to retrieve namespace list")
				return
			}
			if len(namespacelist.Items) == 0 {
				t.Errorf("No namespaces detected in test evironment")
				return
			}
			for _, ns := range namespacelist.Items {
				t.Logf("Found namespace: %v", ns.Name)
			}
		})
	}
}
