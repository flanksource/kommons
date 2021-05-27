package kommons

import (
	"github.com/flanksource/commons/files"
	"github.com/flanksource/commons/logger"
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
		{"1.19.2 Env", args{version: "1.19.2"}, false},
		{"1.20.2 Env", args{version: "1.20.2"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := StartTestEnv(tt.args.version)
			if (err != nil) != tt.wantErr {
				t.Errorf("StartTestEnv() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			cfg := NewClient(got, logger.GetZapLogger())
			nsready, msg := cfg.IsNamespaceReady("default")
			if !nsready {
				t.Errorf("Default namespace not available: %v", msg)
			}
		})
	}
}