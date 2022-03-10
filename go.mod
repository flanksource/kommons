module github.com/flanksource/kommons

go 1.16

require (
	github.com/AlekSi/pointer v1.1.0
	github.com/TomOnTime/utfutil v0.0.0-20210710122150-437f72b26edf
	github.com/flanksource/commons v1.5.14
	github.com/gomarkdown/markdown v0.0.0-20210820032736-385812cbea76
	github.com/hairyhenderson/gomplate/v3 v3.6.0
	github.com/mitchellh/mapstructure v1.3.3
	github.com/mitchellh/reflectwalk v1.0.0
	github.com/patrickmn/go-cache v2.1.0+incompatible
	github.com/pkg/errors v0.9.1
	github.com/sergi/go-diff v1.0.0
	github.com/sirupsen/logrus v1.7.0
	github.com/stretchr/testify v1.7.0
	github.com/tidwall/gjson v1.6.7
	go.etcd.io/etcd v0.5.0-alpha.5.0.20200910180754-dd1b699fc489
	golang.org/x/text v0.3.6
	google.golang.org/grpc v1.27.1
	gopkg.in/flanksource/yaml.v3 v3.1.1
	k8s.io/api v0.20.4
	k8s.io/apiextensions-apiserver v0.20.4
	k8s.io/apimachinery v0.20.4
	k8s.io/cli-runtime v0.20.4
	k8s.io/client-go v11.0.0+incompatible
	k8s.io/klog v1.0.0
	sigs.k8s.io/kustomize v2.0.3+incompatible
	sigs.k8s.io/yaml v1.2.0
)

replace (
	github.com/Azure/go-autorest => github.com/Azure/go-autorest v14.2.0+incompatible
	github.com/go-logr/logr => github.com/go-logr/logr v0.4.0
	k8s.io/client-go => k8s.io/client-go v0.20.4
)
