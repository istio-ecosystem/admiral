package registry

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"strings"
	"time"
)

const (
	DefaultReqTimeoutSeconds          = 10
	DefaultConnectionTimeoutSeconds   = 5
	DefaultTlsHandShakeTimeoutSeconds = 5
	StateSyncerAppId                  = "statesyncer_appId"
	StateSyncerSecret                 = "statesyncer_secret"
)

type Config struct {
	Host                       string
	AppId                      string
	AppSecret                  string
	BaseURI                    string
	ReqTimeoutSeconds          int
	ConnectionTimeoutSeconds   int
	TlsHandShakeTimeoutSeconds int
}

type BaseClient interface {
	MakePrivateAuthCall(url string, tid string, method string, body []byte) (*http.Response, error)
	GetConfig() *Config
}

type Client struct {
	Config     *Config
	HttpClient *http.Client
}

func NewClient(config *Config) *Client {
	c := Client{
		Config: config,
	}

	if c.Config.ReqTimeoutSeconds == 0 {
		c.Config.ReqTimeoutSeconds = DefaultReqTimeoutSeconds
	}

	if c.Config.ConnectionTimeoutSeconds == 0 {
		c.Config.ConnectionTimeoutSeconds = DefaultConnectionTimeoutSeconds
	}

	if c.Config.TlsHandShakeTimeoutSeconds == 0 {
		c.Config.TlsHandShakeTimeoutSeconds = DefaultTlsHandShakeTimeoutSeconds
	}

	//set the ssl hand shake and connection timeout to 5 seconds
	netTransport := &http.Transport{
		Dial: (&net.Dialer{
			Timeout: time.Second * time.Duration(config.ConnectionTimeoutSeconds),
		}).Dial,
		TLSHandshakeTimeout: time.Second * time.Duration(config.TlsHandShakeTimeoutSeconds),
	}

	//set the http response time to 5 seconds
	c.HttpClient = &http.Client{
		Timeout:   time.Second * time.Duration(config.ReqTimeoutSeconds),
		Transport: netTransport,
	}

	return &c
}

func (c *Client) MakePrivateAuthCall(url string, tid string, method string, body []byte) (*http.Response, error) {
	headers := make(http.Header)

	if len(tid) > 0 {
		headers.Set("intuit_tid", tid)
	}

	authHeader := fmt.Sprintf(
		"Intuit_IAM_Authentication intuit_appid=%s, "+
			"intuit_app_secret=%s",
		c.Config.AppId,
		c.Config.AppSecret,
	)

	headers.Set("Content-Type", "application/json")
	headers.Set("Authorization", authHeader)

	return makeCall(url, headers, method, body, c)
}

func (c *Client) GetConfig() *Config {
	return c.Config
}

func makeCall(url string, headers http.Header, method string, body []byte, c *Client) (*http.Response, error) {
	var request *http.Request
	var err error

	request, err = http.NewRequest(method, url, bytes.NewBuffer(body))

	request.Header = headers

	var response *http.Response
	response, err = c.HttpClient.Do(request)

	if err != nil {
		return nil, fmt.Errorf("error executing http request err:%v", err)
	}

	return response, nil
}

// ReadSecret is a helper function to read secrets used for authentication from environment variable. If it doesn't
// exist inside environment variables, fall back to files
func ReadSecret(key string) (string, error) {
	if key != StateSyncerAppId && key != StateSyncerSecret {
		return "", fmt.Errorf("invalid input value for ReadSecret function ")
	}

	var err error
	secret := os.Getenv(key)
	// secret not found in environment variable is returning as empty string
	if secret == "" {
		filePath := "/secrets/" + key + ".txt"
		_, err := os.Stat(filePath)
		if err == nil {
			content, err := ioutil.ReadFile(filePath)
			check(err)
			secret = strings.TrimSpace(string(content))
		}
	}

	if err != nil {
		switch key {
		case StateSyncerAppId:
			return "", fmt.Errorf("statesyncer_appId.txt not found. Using the default appId")
		case StateSyncerSecret:
			return "", fmt.Errorf("error reading statesyncer secret :%v", err)
		}
	}

	return secret, err
}

func check(e error) {
	if e != nil {
		panic(e)
	}
}
