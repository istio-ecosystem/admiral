package types

import (
	coreV1 "k8s.io/api/core/v1"
)

// WeightedService utility to store weighted services for argo rollouts
type WeightedService struct {
	Weight  int32
	Service *coreV1.Service
}
