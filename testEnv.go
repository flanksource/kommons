package kommons

import (
	"github.com/flanksource/commons/deps"
	"io/ioutil"
	"k8s.io/client-go/rest"
	"os"
	"path/filepath"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"strings"
)

var APIServerDefaultArgs = []string{
	"--advertise-address=127.0.0.1",
	"--etcd-servers={{ if .EtcdURL }}{{ .EtcdURL.String }}{{ end }}",
	"--cert-dir={{ .CertDir }}",
	"--insecure-port={{ if .URL }}{{ .URL.Port }}{{ end }}",
	"--insecure-bind-address={{ if .URL }}{{ .URL.Hostname }}{{ end }}",
	"--secure-port={{ if .SecurePort }}{{ .SecurePort }}{{ end }}",
	// we're keeping this disabled because if enabled, default SA is missing which would force all tests to create one
	// in normal apiserver operation this SA is created by controller, but that is not run in integration environment
	//"--disable-admission-plugins=ServiceAccount",
	"--service-cluster-ip-range=10.0.0.0/24",
	"--allow-privileged=true",
}

func InstallTestBin(version string) (string, error) {
	dir, err := ioutil.TempDir("", "kubebuilder-*")
	if err != nil {
		return "", err
	}
	if err = deps.InstallDependency("etcd", version, dir); err != nil {
		return "", err
	}
	return dir, nil
}

func StartTestEnv(version string) (*rest.Config, error) {
	bindir, err := InstallTestBin(version)
	if err != nil {
		return nil, err
	}
	var APIpath = strings.Join([]string{bindir,"kube-apiserver"}, "/")
	var ETCpath = strings.Join([]string{bindir,"etcd"}, "/")
	var Kpath =  strings.Join([]string{bindir,"kubectl"}, "/")

	os.Setenv("TEST_ASSET_KUBE_APISERVER", APIpath)
	os.Setenv("TEST_ASSET_ETCD", ETCpath)
	os.Setenv("TEST_ASSET_KUBECTL", Kpath)
	os.Setenv("KUBEBUILDER_CONTROLPLANE_START_TIMEOUT", "5m")
	os.Setenv("KUBEBUILDER_CONTROLPLANE_STOP_TIMEOUT", "5m")

	var testEnv = &envtest.Environment{
		CRDDirectoryPaths:  []string{filepath.Join("..", "config", "crd", "bases")},
		KubeAPIServerFlags: APIServerDefaultArgs,
	}

	return testEnv.Start()
}