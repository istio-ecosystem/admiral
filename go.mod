module github.com/istio-ecosystem/admiral

go 1.21

require (
	github.com/argoproj/argo-rollouts v1.2.1
	github.com/cenkalti/backoff v2.2.1+incompatible
	github.com/go-openapi/swag v0.22.3 // indirect
	github.com/golang/protobuf v1.5.3
	github.com/google/go-cmp v0.6.0
	github.com/gorilla/mux v1.8.0
	github.com/imdario/mergo v0.3.12 // indirect
	github.com/mailru/easyjson v0.7.7 // indirect
	github.com/onsi/gomega v1.30.0
	github.com/prometheus/client_golang v1.19.1
	github.com/prometheus/client_model v0.6.1
	github.com/sirupsen/logrus v1.8.1
	github.com/spf13/cobra v1.5.0
	github.com/stretchr/testify v1.9.0
	golang.org/x/net v0.20.0 // indirect
	golang.org/x/time v0.0.0-20220210224613-90d013bbcef8 // indirect
	gopkg.in/yaml.v2 v2.4.0
	istio.io/api v1.19.6
	istio.io/client-go v1.14.0
	k8s.io/api v0.28.0
	k8s.io/apimachinery v0.28.0
	k8s.io/client-go v0.24.2
	sigs.k8s.io/yaml v1.3.0 // indirect
)

require (
	github.com/prometheus/common v0.53.0
	go.opentelemetry.io/otel v1.27.0
	go.opentelemetry.io/otel/exporters/prometheus v0.49.0
	go.opentelemetry.io/otel/metric v1.27.0
	go.opentelemetry.io/otel/sdk/metric v1.27.0
	google.golang.org/protobuf v1.34.1
	gopkg.in/natefinch/lumberjack.v2 v2.2.1
)

require (
	github.com/cheekybits/is v0.0.0-20150225183255-68e9c0620927 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/google/pprof v0.0.0-20211214055906-6f57359322fd // indirect
	github.com/rogpeppe/go-internal v1.12.0 // indirect
	go.opentelemetry.io/otel/sdk v1.27.0 // indirect
	go.opentelemetry.io/otel/trace v1.27.0 // indirect
	google.golang.org/genproto v0.0.0-20231002182017-d307bd883b97 // indirect
	google.golang.org/genproto/googleapis/api v0.0.0-20230920204549-e6e6cdab5c13 // indirect
)

require (
	github.com/alecthomas/template v0.0.0-20190718012654-fb15b899a751 // indirect
	github.com/alecthomas/units v0.0.0-20190924025748-f65c72e2690d // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/emicklei/go-restful/v3 v3.10.1 // indirect
	github.com/evanphx/json-patch v4.12.0+incompatible // indirect
	github.com/go-logr/logr v1.4.1 // indirect
	github.com/go-openapi/jsonpointer v0.19.6 // indirect
	github.com/go-openapi/jsonreference v0.20.2 // indirect
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/google/gnostic v0.6.9 // indirect
	github.com/google/gofuzz v1.2.0 // indirect
	github.com/google/uuid v1.3.0
	github.com/inconshreveable/mousetrap v1.0.0 // indirect
	github.com/josharian/intern v1.0.0 // indirect
	github.com/json-iterator/go v1.1.12 // indirect
	github.com/matryer/resync v0.0.0-20161211202428-d39c09a11215
	github.com/matttproud/golang_protobuf_extensions v1.0.2-0.20181231171920-c182affec369 // indirect
	github.com/modern-go/concurrent v0.0.0-20180306012644-bacd9c7ef1dd // indirect
	github.com/modern-go/reflect2 v1.0.2 // indirect
	github.com/munnerz/goautoneg v0.0.0-20191010083416-a7dc8b61c822 // indirect
	github.com/onsi/ginkgo/v2 v2.13.2 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/prometheus/procfs v0.15.0 // indirect
	github.com/spf13/pflag v1.0.5 // indirect
	github.com/stretchr/objx v0.5.2 // indirect
	golang.org/x/oauth2 v0.16.0 // indirect
	golang.org/x/sys v0.20.0 // indirect
	golang.org/x/term v0.16.0 // indirect
	golang.org/x/text v0.14.0 // indirect
	google.golang.org/appengine v1.6.7 // indirect
	gopkg.in/alecthomas/kingpin.v2 v2.2.6 // indirect
	gopkg.in/inf.v0 v0.9.1 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
	k8s.io/klog/v2 v2.100.1 // indirect
	k8s.io/kube-openapi v0.0.0-20230210211930-4b0756abdef5 // indirect
	k8s.io/utils v0.0.0-20230406110748-d93618cff8a2 // indirect
	sigs.k8s.io/json v0.0.0-20221116044647-bc3834ca7abd // indirect
	sigs.k8s.io/structured-merge-diff/v4 v4.2.3 // indirect
)

replace (
	github.com/fsnotify/fsnotify => github.com/fsnotify/fsnotify v1.5.1
	github.com/go-check/check => github.com/go-check/check v0.0.0-20180628173108-788fd7840127
	github.com/prometheus/common => github.com/prometheus/common v0.26.0
	k8s.io/api => k8s.io/api v0.24.2
	k8s.io/apiextensions-apiserver => k8s.io/apiextensions-apiserver v0.24.2
	k8s.io/apimachinery => k8s.io/apimachinery v0.24.2
	k8s.io/apiserver => k8s.io/apiserver v0.24.2
	k8s.io/cli-runtime => k8s.io/cli-runtime v0.24.2
	k8s.io/client-go => k8s.io/client-go v0.24.2
	k8s.io/cloud-provider => k8s.io/cloud-provider v0.24.2
	k8s.io/cluster-bootstrap => k8s.io/cluster-bootstrap v0.24.2
	k8s.io/code-generator => k8s.io/code-generator v0.24.2
	k8s.io/component-base => k8s.io/component-base v0.24.2
	k8s.io/component-helpers => k8s.io/component-helpers v0.24.2
	k8s.io/controller-manager => k8s.io/controller-manager v0.24.2
	k8s.io/cri-api => k8s.io/cri-api v0.24.2
	k8s.io/csi-translation-lib => k8s.io/csi-translation-lib v0.24.2
	k8s.io/kube-aggregator => k8s.io/kube-aggregator v0.24.2
	k8s.io/kube-controller-manager => k8s.io/kube-controller-manager v0.24.2
	k8s.io/kube-proxy => k8s.io/kube-proxy v0.24.2
	k8s.io/kube-scheduler => k8s.io/kube-scheduler v0.24.2
	k8s.io/kubectl => k8s.io/kubectl v0.24.2
	k8s.io/kubelet => k8s.io/kubelet v0.24.2
	k8s.io/kubernetes => k8s.io/kubernetes v1.23.1
	k8s.io/legacy-cloud-providers => k8s.io/legacy-cloud-providers v0.24.2
	k8s.io/metrics => k8s.io/metrics v0.24.2
	k8s.io/mount-utils => k8s.io/mount-utils v0.24.2
	k8s.io/pod-security-admission => k8s.io/pod-security-admission v0.24.2
	k8s.io/sample-apiserver => k8s.io/sample-apiserver v0.24.2
)

exclude (
	github.com/elazarl/goproxy v0.0.0-20180725130230-947c36da3153
	github.com/emicklei/go-restful v0.0.0-20170410110728-ff4f55a20633
	github.com/sassoftware/go-rpmutils v0.0.0-20190420191620-a8f1baeba37b
	golang.org/x/crypto v0.0.0-20181029021203-45a5f77698d3
	golang.org/x/crypto v0.0.0-20190611184440-5c40567a22f8
	golang.org/x/net v0.0.0-20180724234803-3673e40ba225
	golang.org/x/net v0.0.0-20190108225652-1e06a53dbb7e
	golang.org/x/text v0.0.0-20170915032832-14c0d48ead0c
	gopkg.in/yaml.v3 v3.0.0-20200313102051-9f266ea9e77c
	gopkg.in/yaml.v3 v3.0.0-20210107192922-496545a6307b
)
