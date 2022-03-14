module github.com/istio-ecosystem/admiral

go 1.12

require (
	github.com/alecthomas/units v0.0.0-20190924025748-f65c72e2690d // indirect
	github.com/argoproj/argo-rollouts v0.8.3
	github.com/cenkalti/backoff v2.2.1+incompatible
	github.com/emicklei/go-restful v2.11.2+incompatible // indirect
	github.com/go-openapi/spec v0.19.6 // indirect
	github.com/go-openapi/swag v0.19.7 // indirect
	github.com/gogo/protobuf v1.3.1
	github.com/golang/groupcache v0.0.0-20191002201903-404acd9df4cc // indirect
	github.com/golang/protobuf v1.3.2
	github.com/google/go-cmp v0.4.0
	github.com/gorilla/mux v1.8.0
	github.com/imdario/mergo v0.3.8 // indirect
	github.com/konsorten/go-windows-terminal-sequences v1.0.2 // indirect
	github.com/mailru/easyjson v0.7.1 // indirect
	github.com/onsi/ginkgo v1.10.2 // indirect
	github.com/onsi/gomega v1.7.0
	github.com/prometheus/common v0.9.1
	github.com/sirupsen/logrus v1.4.2
	github.com/spf13/cobra v0.0.5
	github.com/stretchr/testify v1.7.0
	golang.org/x/crypto v0.0.0-20191108234033-bd318be0434a // indirect
	golang.org/x/net v0.0.0-20200301022130-244492dfa37a // indirect
	golang.org/x/time v0.0.0-20191024005414-555d28b269f0 // indirect
	google.golang.org/genproto v0.0.0-20191108220845-16a3f7862a1a // indirect
	google.golang.org/grpc v1.25.1 // indirect
	gopkg.in/yaml.v2 v2.2.8
	istio.io/api v0.0.0-20200226024546-cca495b82b03
	istio.io/client-go v0.0.0-20200226182959-cde3e69bd9dd
	istio.io/gogo-genproto v0.0.0-20191024203824-d079cc8b1d55 // indirect
	k8s.io/api v0.17.3
	k8s.io/apimachinery v0.17.3
	k8s.io/client-go v0.17.3
	k8s.io/kube-openapi v0.0.0-20200204173128-addea2498afe // indirect
	sigs.k8s.io/yaml v1.2.0 // indirect
)

replace k8s.io/api => k8s.io/api v0.17.3

replace k8s.io/apiextensions-apiserver => k8s.io/apiextensions-apiserver v0.17.3

replace k8s.io/apimachinery => k8s.io/apimachinery v0.17.5-beta.0

replace k8s.io/apiserver => k8s.io/apiserver v0.17.3

replace k8s.io/cli-runtime => k8s.io/cli-runtime v0.17.3

replace k8s.io/client-go => k8s.io/client-go v0.17.3

replace k8s.io/cloud-provider => k8s.io/cloud-provider v0.17.3

replace k8s.io/cluster-bootstrap => k8s.io/cluster-bootstrap v0.17.3

replace k8s.io/code-generator => k8s.io/code-generator v0.17.5-beta.0

replace k8s.io/component-base => k8s.io/component-base v0.17.3

replace k8s.io/cri-api => k8s.io/cri-api v0.17.18-rc.0

replace k8s.io/csi-translation-lib => k8s.io/csi-translation-lib v0.17.3

replace k8s.io/kube-aggregator => k8s.io/kube-aggregator v0.17.3

replace k8s.io/kube-controller-manager => k8s.io/kube-controller-manager v0.17.3

replace k8s.io/kube-proxy => k8s.io/kube-proxy v0.17.3

replace k8s.io/kube-scheduler => k8s.io/kube-scheduler v0.17.3

replace k8s.io/kubectl => k8s.io/kubectl v0.17.3

replace k8s.io/kubelet => k8s.io/kubelet v0.17.3

replace k8s.io/legacy-cloud-providers => k8s.io/legacy-cloud-providers v0.17.3

replace k8s.io/metrics => k8s.io/metrics v0.17.3

replace k8s.io/node-api => k8s.io/node-api v0.17.3

replace k8s.io/sample-apiserver => k8s.io/sample-apiserver v0.17.3

replace k8s.io/sample-cli-plugin => k8s.io/sample-cli-plugin v0.17.3

replace k8s.io/sample-controller => k8s.io/sample-controller v0.17.3
