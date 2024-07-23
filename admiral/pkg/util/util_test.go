package util

import "testing"

func TestGetPortProtocol(t *testing.T) {
	cases := []struct {
		name        string
		protocol    string
		expProtocol string
	}{
		{
			name: "Given valid input parameters, " +
				"When port name is " + Http + ", " +
				"Then protocol should be " + Http,
			protocol:    Http,
			expProtocol: Http,
		},
		{
			name: "Given valid input parameters, " +
				"When port name is " + GrpcWeb + ", " +
				"Then protocol should be " + GrpcWeb,
			protocol:    GrpcWeb,
			expProtocol: GrpcWeb,
		},
		{
			name: "Given valid input parameters, " +
				"When port name is " + Grpc + ", " +
				"Then protocol should be " + Grpc,
			protocol:    Grpc,
			expProtocol: Grpc,
		},
		{
			name: "Given valid input parameters, " +
				"When port name is " + Http2 + ", " +
				"Then protocol should be " + Http2,
			protocol:    Http2,
			expProtocol: Http2,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			protocol := GetPortProtocol(c.protocol)
			if protocol != c.expProtocol {
				t.Errorf("expected=%v, got=%v", c.expProtocol, protocol)
			}
		})
	}
}
