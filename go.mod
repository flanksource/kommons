module github.com/flanksource/kommons

go 1.16

require (
	github.com/AlekSi/pointer v1.1.0
	github.com/docker/docker v0.7.3-0.20190327010347-be7ac8be2ae0 // indirect
	github.com/flanksource/commons v1.5.1
	github.com/globalsign/mgo v0.0.0-20181015135952-eeefdecb41b8 // indirect
	github.com/go-openapi/analysis v0.17.2 // indirect
	github.com/go-openapi/errors v0.17.2 // indirect
	github.com/go-openapi/loads v0.17.2 // indirect
	github.com/go-openapi/runtime v0.17.2 // indirect
	github.com/go-openapi/validate v0.18.0 // indirect
	github.com/go-test/deep v1.0.7
	github.com/hairyhenderson/gomplate/v3 v3.6.0
	github.com/mitchellh/mapstructure v1.3.3
	github.com/natefinch/lumberjack v2.0.0+incompatible // indirect
	github.com/pkg/errors v0.9.1
	github.com/sergi/go-diff v1.0.0 // indirect
	github.com/sirupsen/logrus v1.7.0
	github.com/tidwall/gjson v1.6.7
	go.etcd.io/etcd v0.5.0-alpha.5.0.20200910180754-dd1b699fc489
	gonum.org/v1/netlib v0.0.0-20190331212654-76723241ea4e // indirect
	google.golang.org/grpc v1.27.1
	gopkg.in/flanksource/yaml.v3 v3.1.1
	gopkg.in/yaml.v1 v1.0.0-20140924161607-9f9df34309c0 // indirect
	k8s.io/api v0.20.4
	k8s.io/apiextensions-apiserver v0.20.4 // indirect
	k8s.io/apimachinery v0.20.4
	k8s.io/cli-runtime v0.20.4
	k8s.io/client-go v11.0.0+incompatible
	k8s.io/klog v1.0.0
	sigs.k8s.io/kustomize v2.0.3+incompatible
	sigs.k8s.io/structured-merge-diff v0.0.0-20190302045857-e85c7b244fd2 // indirect
	sigs.k8s.io/yaml v1.2.0
)

replace (
	github.com/Azure/go-autorest => github.com/Azure/go-autorest v14.2.0+incompatible
	k8s.io/client-go => k8s.io/client-go v0.20.4
)
