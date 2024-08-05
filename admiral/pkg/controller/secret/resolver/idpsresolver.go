package resolver

import (
	"fmt"
	"io/ioutil"
	"sync"

	v1 "github.com/istio-ecosystem/admiral/admiral/apis/v1"
	"github.com/istio-ecosystem/admiral/admiral/pkg/client"
	log "github.com/sirupsen/logrus"
	"gopkg.in/yaml.v2"
)

type IDPSResolver struct {
	IdpsConfig      *v1.AdmiralConfig
	IdpsClient      client.IdpsClientAPI
	KubeConfigCache *sync.Map
}

var Config = &v1.AdmiralConfig{}

func (r IDPSResolver) FetchKubeConfig(secretName string, _ []byte) ([]byte, error) {
	idpsKubeConfig, ok := r.KubeConfigCache.Load(secretName)
	if ok {
		return idpsKubeConfig.([]byte), nil
	} else {
		if r.IdpsConfig.IdpsConfig.KubeConfigSecretFolder != "" {
			secretName = r.IdpsConfig.IdpsConfig.KubeConfigSecretFolder + "/" + secretName
		}
		idpsKubeConfig, err := r.IdpsClient.GetSecret(secretName)
		if err != nil {
			log.Errorf("Failed to fetch kubeconfig with name: %v with error: %v", secretName, err)
			return nil, err
		} else {
			kubeConfig := []byte(idpsKubeConfig)
			r.KubeConfigCache.Store(secretName, kubeConfig)
			return kubeConfig, nil
		}
	}
}

func NewIDPSResolver(configFile string, clientProvider client.IdpsSdkProvider) (SecretResolver, error) {

	data, err := ioutil.ReadFile(configFile)

	if err != nil {
		return nil, fmt.Errorf("error reading secret resolver config file err: %v", err)
	}

	err = yaml.Unmarshal(data, &Config)

	if err != nil {
		return nil, fmt.Errorf("error unmarshaling config file err: %v", err)
	}

	IdpsClient, err := client.NewIdpsClient(Config, clientProvider)
	if err != nil {
		log.Infof("Failed to init IDPS clients in err%v", err)
		return nil, err
	}

	return IDPSResolver{
		IdpsConfig:      Config,
		IdpsClient:      IdpsClient,
		KubeConfigCache: &sync.Map{},
	}, nil
}
