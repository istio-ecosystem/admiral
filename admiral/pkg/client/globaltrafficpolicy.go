package client

import (
	"context"
	"fmt"

	v1 "github.com/istio-ecosystem/admiral/admiral/pkg/apis/admiral/v1"
	admiral "github.com/istio-ecosystem/admiral/admiral/pkg/client/clientset/versioned"
	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const gtpEnvAnnotation = "admiral.io/env"

type GlobalTrafficPolicyReaderAPI interface {
	List(ctx context.Context, clientset admiral.Interface, namespace string, options metav1.ListOptions) (*v1.GlobalTrafficPolicyList, error)
	GetByEnv(ctx context.Context, clientset admiral.Interface, namespace string, env string, options metav1.ListOptions) (*v1.GlobalTrafficPolicy, error)
}

type GlobalTrafficPolicyWriterAPI interface {
	Create(ctx context.Context, clientset admiral.Interface, namespace string, globalTrafficPolicy *v1.GlobalTrafficPolicy, options metav1.CreateOptions) (*v1.GlobalTrafficPolicy, error)
	Update(ctx context.Context, clientset admiral.Interface, namespace string, globalTrafficPolicy *v1.GlobalTrafficPolicy, options metav1.UpdateOptions) (*v1.GlobalTrafficPolicy, error)
	Delete(ctx context.Context, clientset admiral.Interface, namespace string, name string, options metav1.DeleteOptions) error
}

type GlobalTrafficPolicy struct {
}

func (*GlobalTrafficPolicy) List(ctx context.Context, clientset admiral.Interface, namespace string,
	options metav1.ListOptions) (*v1.GlobalTrafficPolicyList, error) {
	if namespace == "" {
		return nil, errors.New("namespace parameter is empty")
	}
	if clientset == nil {
		return nil, errors.New("*admiral.Clientset is nil")
	}
	if clientset.AdmiralV1() == nil {
		return nil, errors.New("*admiralv1.AdmiralV1Client is nil")
	}
	gtpClient := clientset.AdmiralV1().GlobalTrafficPolicies(namespace)
	return gtpClient.List(ctx, options)
}

func (*GlobalTrafficPolicy) GetByEnv(ctx context.Context, clientset admiral.Interface, namespace string,
	env string, options metav1.ListOptions) (*v1.GlobalTrafficPolicy, error) {
	if namespace == "" {
		return nil, errors.New("namespace parameter is empty")
	}
	if clientset == nil {
		return nil, errors.New("*admiral.Clientset is nil")
	}
	if clientset.AdmiralV1() == nil {
		return nil, errors.New("*admiralv1.AdmiralV1Client is nil")
	}
	if env == "" {
		return nil, errors.New("env parameter is empty")
	}
	gtpClient := clientset.AdmiralV1().GlobalTrafficPolicies(namespace)
	gtpList, err := gtpClient.List(ctx, options)
	if err != nil {
		return nil, err
	}
	for _, gtp := range gtpList.Items {
		if gtp.Annotations[gtpEnvAnnotation] == env {
			return &gtp, nil
		}
	}
	return nil, fmt.Errorf("no gtp found with env: %s in namespace: %s", env, namespace)
}

func (*GlobalTrafficPolicy) Create(ctx context.Context, clientset admiral.Interface, namespace string, globalTrafficPolicy *v1.GlobalTrafficPolicy, options metav1.CreateOptions) (*v1.GlobalTrafficPolicy, error) {
	if namespace == "" {
		return nil, errors.New("namespace parameter is empty")
	}
	if clientset == nil {
		return nil, errors.New("*admiral.Clientset is nil")
	}
	if clientset.AdmiralV1() == nil {
		return nil, errors.New("*admiralv1.AdmiralV1Client is nil")
	}
	if globalTrafficPolicy == nil {
		return nil, errors.New("the globaltrafficpolicy param is nil")
	}

	gtpClient := clientset.AdmiralV1().GlobalTrafficPolicies(namespace)
	return gtpClient.Create(ctx, globalTrafficPolicy, options)
}

func (*GlobalTrafficPolicy) Update(ctx context.Context, clientset admiral.Interface, namespace string, globalTrafficPolicy *v1.GlobalTrafficPolicy, options metav1.UpdateOptions) (*v1.GlobalTrafficPolicy, error) {
	if namespace == "" {
		return nil, errors.New("namespace parameter is empty")
	}
	if clientset == nil {
		return nil, errors.New("*admiral.Clientset is nil")
	}
	if clientset.AdmiralV1() == nil {
		return nil, errors.New("*admiralv1.AdmiralV1Client is nil")
	}
	if globalTrafficPolicy == nil {
		return nil, errors.New("the globaltrafficpolicy param is nil")
	}

	gtpClient := clientset.AdmiralV1().GlobalTrafficPolicies(namespace)
	return gtpClient.Update(ctx, globalTrafficPolicy, options)
}

func (*GlobalTrafficPolicy) Delete(ctx context.Context, clientset admiral.Interface, namespace string, name string, options metav1.DeleteOptions) error {
	if namespace == "" {
		return errors.New("namespace parameter is empty")
	}
	if clientset == nil {
		return errors.New("*admiral.Clientset is nil")
	}
	if clientset.AdmiralV1() == nil {
		return errors.New("*admiralv1.AdmiralV1Client is nil")
	}
	if name == "" {
		return errors.New("the name param is empty")
	}

	gtpClient := clientset.AdmiralV1().GlobalTrafficPolicies(namespace)
	return gtpClient.Delete(ctx, name, options)
}
