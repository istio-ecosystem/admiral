package model

//go:generate protoc -I . dependency.proto --go_out=plugins=grpc:.
//go:generate protoc -I . globalrouting.proto --go_out=plugins=grpc:.
//go:generate protoc -I . routingpolicy.proto --go_out=plugins=grpc:.
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +k8s:deepcopy-gen=package,register
