package registry

import (
	"errors"
	"fmt"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewClient_InvalidValues(t *testing.T) {

	client := NewClient(&Config{
		AppId:     "InvalidAppId",
		AppSecret: "InvalidSecret",
		Host:      "host.com",
	})

	url := fmt.Sprintf("%s/%s/apps/assetalias/%s", client.Config.Host, client.Config.BaseURI, "1234")

	assert.Equal(t, client.Config.ConnectionTimeoutSeconds, DefaultConnectionTimeoutSeconds)
	assert.Equal(t, client.Config.ReqTimeoutSeconds, DefaultReqTimeoutSeconds)
	assert.Equal(t, client.Config.TlsHandShakeTimeoutSeconds, DefaultTlsHandShakeTimeoutSeconds)

	resp, err := client.MakePrivateAuthCall(url, "tid", "GET", nil)

	assert.NotNil(t, err)
	assert.Nil(t, resp)
}

func Test_ReadSecret(t *testing.T) {
	testCases := []struct {
		name          string
		secretKey     string
		secretVal     string
		expectedError error
	}{
		{
			"Success scenario", StateSyncerSecret, "1", nil,
		},
		{
			"Invalid secretKey name", "foo", "bar", errors.New("invalid input value for ReadSecret function "),
		},
		{
			"Empty secret value", StateSyncerSecret, "", errors.New("error reading statesyncer secret"),
		},
		{
			"Empty secret value with StateSyncerAppId", StateSyncerAppId, "", errors.New("statesyncer_appId.txt not found. Using the default appId"),
		},
	}

	for _, c := range testCases {
		t.Run(c.name, func(t *testing.T) {
			_ = os.Setenv(c.secretKey, c.secretVal)
			secret, err := ReadSecret(c.secretKey)
			if c.expectedError != nil && err != nil {
				assert.Contains(t, err.Error(), c.expectedError.Error())
			} else {
				if c.expectedError == nil && err != nil {
					t.Errorf("expected error to be: nil, got: %v", err)
				}
			}
			if c.expectedError == nil {
				assert.Equal(t, c.secretVal, secret)
			}
		})
	}
}
func TestGetConfig(t *testing.T) {
	client := NewClient(&Config{
		AppId:     "TestAppId",
		AppSecret: "TestSecret",
		Host:      "test.api.com",
		BaseURI:   "v1",
	})

	config := client.GetConfig()

	assert.Equal(t, "TestAppId", config.AppId)
	assert.Equal(t, "TestSecret", config.AppSecret)
	assert.Equal(t, "test.api.com", config.Host)
	assert.Equal(t, "v1", config.BaseURI)
	assert.Equal(t, DefaultReqTimeoutSeconds, config.ReqTimeoutSeconds)
	assert.Equal(t, DefaultConnectionTimeoutSeconds, config.ConnectionTimeoutSeconds)
	assert.Equal(t, DefaultTlsHandShakeTimeoutSeconds, config.TlsHandShakeTimeoutSeconds)
}
func TestCheck_NoError(t *testing.T) {
	assert.NotPanics(t, func() {
		check(nil)
	})
}

func TestCheck_WithError(t *testing.T) {
	assert.Panics(t, func() {
		check(errors.New("test error"))
	})
}
