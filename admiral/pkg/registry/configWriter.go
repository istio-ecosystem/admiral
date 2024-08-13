package registry

import (
	"context"
	"fmt"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/common"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/util"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	k8sV1 "k8s.io/api/core/v1"
	k8sErrors "k8s.io/apimachinery/pkg/api/errors"
	metaV1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"os"
	"strings"
)

type ConfigWriter interface {
	Write(name string, data []byte) error
}

func NewConfigMapWriter(k8sClient kubernetes.Interface, ctxLogger *logrus.Entry) ConfigWriter {
	return &configMapWriter{
		k8sClient: k8sClient,
		ctxLogger: ctxLogger,
	}
}

type configMapWriter struct {
	ctxLogger *logrus.Entry
	k8sClient kubernetes.Interface
}

func (c *configMapWriter) Write(name string, data []byte) error {
	var (
		task = "ConfigMapWriter"
		cm   *k8sV1.ConfigMap
		err  error
	)
	defer util.LogElapsedTimeForTask(c.ctxLogger, task, name, "", "", "processingTime")()
	if common.GetSecretFilterTags() != secretLabel {
		c.ctxLogger.Infof(common.CtxLogFormat, task, name, "", "", "writing to local file")
		return writeToFile(name, data)
	}
	nameParts := strings.Split(name, "/")
	configMapDataKeyName := nameParts[len(nameParts)-1]
	c.ctxLogger.Infof(common.CtxLogFormat, task, name, "", "", "updating configmap="+serviceRegistryIdentityConfigMapName+" key="+configMapDataKeyName)
	cm, err = c.k8sClient.CoreV1().ConfigMaps(admiralQaNs).Get(context.TODO(), serviceRegistryIdentityConfigMapName, metaV1.GetOptions{})
	if err != nil {
		if k8sErrors.IsNotFound(err) {
			// configmap doesn't exist. Create one
			c.ctxLogger.Infof(common.CtxLogFormat, task, name, "", "", "configmap doesn't exist, creating one")
			cm = &k8sV1.ConfigMap{
				ObjectMeta: metaV1.ObjectMeta{
					Name:      serviceRegistryIdentityConfigMapName,
					Namespace: admiralQaNs,
				},
			}
		}
		c.ctxLogger.Warnf(common.CtxLogFormat, task, name, "", "", "error getting configmap, error="+err.Error()+" will still try to update it")
	}
	currentData := cm.Data
	currentData[configMapDataKeyName] = string(data)
	cm.Data = currentData
	_, err = c.k8sClient.CoreV1().ConfigMaps(admiralQaNs).Update(context.TODO(), cm, metaV1.UpdateOptions{})
	if err != nil {
		c.ctxLogger.Warnf(common.CtxLogFormat, task, name, "", "", "error getting configmap, error="+err.Error()+" will still try to update it")
		return errors.Wrap(err, "failed to update configMap")
	}
	return nil
}

func writeToFile(name string, data []byte) error {
	f, err := os.Create(name)
	if err != nil {
		fmt.Printf("[state syncer] err=%v", err)
		return err
	}
	_, err = f.Write(data)
	if err != nil {
		fmt.Printf("[state syncer] err=%v", err)
		return err
	}
	return err
}
