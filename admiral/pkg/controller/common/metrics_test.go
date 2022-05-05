package common

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/stretchr/testify/assert"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strconv"
	"testing"
)

func TestNewGaugeFrom(t *testing.T) {
	type args struct {
		prom        bool
		name        string
		help        string
		value       int64
		labelNames  []string
		labelValues []string
	}
	tc := []struct {
		name       string
		args       args
		wantMetric bool
		wantValue  int64
	}{
		{
			name:       "Should return a Prometheus gauge",
			args:       args{true, "mygauge", "", 10, []string{"l1", "l2"}, []string{"v1", "v2"}},
			wantMetric: true,
			wantValue:  10,
		},
		{
			name:       "Should return a Noop gauge",
			args:       args{false, "mygauge", "", 10, []string{}, []string{}},
			wantMetric: false,
		},
	}

	for _, tt := range tc {
		t.Run(tt.name, func(t *testing.T) {
			SetEnablePrometheus(tt.args.prom)

			// exercise metric
			actual := NewGaugeFrom(tt.args.name, tt.args.help, tt.args.labelNames)
			actual.With(tt.args.labelValues...).Set(float64(tt.args.value))

			// query metrics endpoint
			s := httptest.NewServer(promhttp.HandlerFor(prometheus.DefaultGatherer, promhttp.HandlerOpts{}))
			defer s.Close()

			// parse response
			resp, _ := http.Get(s.URL)
			buf, _ := ioutil.ReadAll(resp.Body)
			actualString := string(buf)

			// verify
			if tt.wantMetric {
				pattern := tt.args.name + `{l1="v1",l2="v2"} ([0-9]+)`
				re := regexp.MustCompile(pattern)
				matches := re.FindStringSubmatch(actualString)
				f, _ := strconv.ParseInt(matches[1], 0, 64)
				assert.Equal(t, tt.wantValue, f)
			}
			assert.Equal(t, 200, resp.StatusCode)
		})
	}
}

func TestNewCounterFrom(t *testing.T) {
	type args struct {
		prom        bool
		name        string
		help        string
		value       int64
		labelNames  []string
		labelValues []string
	}
	tc := []struct {
		name       string
		args       args
		wantMetric bool
		wantValue  int64
	}{
		{
			name:       "Should return a Noop counter",
			args:       args{false, "mycounter", "", 10, []string{}, []string{}},
			wantMetric: false,
		},
		{
			name:       "Should return a Prometheus counter",
			args:       args{true, "mycounter", "", 1, []string{"l1", "l2"}, []string{"v1", "v2"}},
			wantMetric: true,
			wantValue:  1,
		},
	}

	for _, tt := range tc {
		t.Run(tt.name, func(t *testing.T) {
			SetEnablePrometheus(tt.args.prom)

			// exercise metric
			actual := NewCounterFrom(tt.args.name, tt.args.help, tt.args.labelNames)
			var i int64
			for i = 0; i < tt.args.value; i++ {
				actual.With(tt.args.labelValues...).Inc()
			}

			// query metrics endpoint
			s := httptest.NewServer(promhttp.HandlerFor(prometheus.DefaultGatherer, promhttp.HandlerOpts{}))
			defer s.Close()

			// parse response
			resp, _ := http.Get(s.URL)
			buf, _ := ioutil.ReadAll(resp.Body)
			actualString := string(buf)

			// verify
			if tt.wantMetric {
				pattern := tt.args.name + `{l1="v1",l2="v2"} ([0-9]+)`
				re := regexp.MustCompile(pattern)
				s2 := re.FindStringSubmatch(actualString)[1]
				f, _ := strconv.ParseInt(s2, 0, 64)
				assert.Equal(t, tt.wantValue, f)
			}
			assert.Equal(t, 200, resp.StatusCode)
		})
	}
}
