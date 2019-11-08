package prometheus

import (
	"github.com/stretchr/testify/assert"
	"golang.org/x/net/context"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	keptnevents "github.com/keptn/go-utils/pkg/events"
)

func testingHTTPClient(handler http.Handler) (*http.Client, func()) {
	s := httptest.NewServer(handler)

	cli := &http.Client{
		Transport: &http.Transport{
			DialContext: func(_ context.Context, network, _ string) (net.Conn, error) {
				return net.Dial(network, s.Listener.Addr().String())
			},
		},
	}

	return cli, s.Close
}

func TestGetErrorRateQueryWithoutFilter(t *testing.T) {
	ph := NewPrometheusHandler("prometheus", "sockshop", "dev", "carts", nil)

	start := time.Unix(1571649084, 0)
	end := time.Unix(1571649085, 0)
	query := ph.getErrorRateQuery(start, end)

	expectedQuery := "sum(rate(http_requests_total{job='carts-sockshop-dev-canary',status!~'2..'}[1s]))/sum(rate(http_requests_total{job='carts-sockshop-dev-canary'}[1s]))"

	if strings.Compare(strings.Replace(query, " ", "", -1), strings.Replace(expectedQuery, " ", "", -1)) != 0 {
		t.Errorf("Expected query did not match: \n expected: " + expectedQuery + "\n got: " + query)
	}
}

func TestGetErrorRateQueryWithFilter(t *testing.T) {

	var customFilters []*keptnevents.SLIFilter

	customFilters = append(customFilters, &keptnevents.SLIFilter{
		Key:   "handler",
		Value: "=~'ItemsController'",
	})

	ph := NewPrometheusHandler("prometheus", "sockshop", "dev", "carts", customFilters)

	start := time.Unix(1571649084, 0)
	end := time.Unix(1571649085, 0)
	query := ph.getErrorRateQuery(start, end)

	expectedQuery := "sum(rate(http_requests_total{job='carts-sockshop-dev-canary',handler=~'ItemsController',status!~'2..'}[1s]))/sum(rate(http_requests_total{job='carts-sockshop-dev-canary',handler=~'ItemsController'}[1s]))"

	if strings.Compare(strings.Replace(query, " ", "", -1), strings.Replace(expectedQuery, " ", "", -1)) != 0 {
		t.Errorf("Expected query did not match: \n expected: " + expectedQuery + "\n got: " + query)
	}
}

func TestGetCustomErrorRateQueryWithFilter(t *testing.T) {

	var customFilters []*keptnevents.SLIFilter

	customQueries := map[string]string{}
	customQueries["error_rate"] = "sum(rate(my_custom_metric{job='$SERVICE-$PROJECT-$STAGE',handler=~'$handler',status!~'2..'}[$DURATION_SECONDS]))/sum(rate(my_custom_metric{job='$SERVICE-$PROJECT-$STAGE',handler=~'$handler'}[$DURATION_SECONDS]))"

	customFilters = append(customFilters, &keptnevents.SLIFilter{
		Key:   "handler",
		Value: "'ItemsController'",
	})

	ph := NewPrometheusHandler("prometheus", "sockshop", "dev", "carts", customFilters)

	ph.CustomQueries = customQueries

	start := time.Unix(1571649084, 0)
	end := time.Unix(1571649085, 0)
	query := ph.getErrorRateQuery(start, end)

	expectedQuery := "sum(rate(my_custom_metric{job='carts-sockshop-dev',handler=~'ItemsController',status!~'2..'}[1s]))/sum(rate(my_custom_metric{job='carts-sockshop-dev',handler=~'ItemsController'}[1s]))"

	if strings.Compare(strings.Replace(query, " ", "", -1), strings.Replace(expectedQuery, " ", "", -1)) != 0 {
		t.Errorf("Expected query did not match: \n expected: " + expectedQuery + "\n got: " + query)
	}
}

func TestGetThroughputQuery(t *testing.T) {
	ph := NewPrometheusHandler("prometheus", "sockshop", "dev", "carts", nil)

	start := time.Unix(1571649084, 0)
	end := time.Unix(1571649085, 0)
	query := ph.getThroughputQuery(start, end)

	expectedQuery := "sum(rate(http_requests_total{job='carts-sockshop-dev-canary'}[1s]))"

	if strings.Compare(strings.Replace(query, " ", "", -1), strings.Replace(expectedQuery, " ", "", -1)) != 0 {
		t.Errorf("Expected query did not match: \n expected: " + expectedQuery + "\n got: " + query)
	}
}

func TestGetCustomThroughputQueryWithFilter(t *testing.T) {

	var customFilters []*keptnevents.SLIFilter

	customQueries := map[string]string{}
	customQueries["throughput"] = "rate(my_custom_metric{job='$SERVICE-$PROJECT-$STAGE',handler=~'$handler'}[$DURATION_SECONDS])"

	customFilters = append(customFilters, &keptnevents.SLIFilter{
		Key:   "handler",
		Value: "'ItemsController'",
	})

	ph := NewPrometheusHandler("prometheus", "sockshop", "dev", "carts", customFilters)
	ph.CustomQueries = customQueries

	start := time.Unix(1571649084, 0)
	end := time.Unix(1571649085, 0)
	query := ph.getThroughputQuery(start, end)

	expectedQuery := "rate(my_custom_metric{job='carts-sockshop-dev',handler=~'ItemsController'}[1s])"

	if strings.Compare(strings.Replace(query, " ", "", -1), strings.Replace(expectedQuery, " ", "", -1)) != 0 {
		t.Errorf("Expected query did not match: \n expected: " + expectedQuery + "\n got: " + query)
	}
}

func TestGetRequestLatencyQuery(t *testing.T) {
	ph := NewPrometheusHandler("prometheus", "sockshop", "dev", "carts", nil)

	start := time.Unix(1571649084, 0)
	end := time.Unix(1571649085, 0)
	query := ph.getRequestLatencyQuery("95", start, end)

	expectedQuery := "histogram_quantile(0.95,sum(rate(http_response_time_milliseconds_bucket{job='carts-sockshop-dev-canary'}[1s]))by(le))"

	if strings.Compare(strings.Replace(query, " ", "", -1), strings.Replace(expectedQuery, " ", "", -1)) != 0 {
		t.Errorf("Expected query did not match: \n expected: " + expectedQuery + "\n got: " + query)
	}
}

func TestGetCustomResponseTimeQueryWithFilter(t *testing.T) {

	var customFilters []*keptnevents.SLIFilter

	customQueries := map[string]string{}
	customQueries["response_time_p50"] = "histogram_quantile(0.50,sum(rate(my_custom_response_time_metric{job='$SERVICE-$PROJECT-$STAGE'}[$DURATION_SECONDS]))by(le))"

	customFilters = append(customFilters, &keptnevents.SLIFilter{
		Key:   "handler",
		Value: "'ItemsController'",
	})

	ph := NewPrometheusHandler("prometheus", "sockshop", "dev", "carts", customFilters)

	ph.CustomQueries = customQueries

	start := time.Unix(1571649084, 0)
	end := time.Unix(1571649085, 0)
	query := ph.getRequestLatencyQuery("50", start, end)

	expectedQuery := "histogram_quantile(0.50,sum(rate(my_custom_response_time_metric{job='carts-sockshop-dev'}[1s]))by(le))"

	if strings.Compare(strings.Replace(query, " ", "", -1), strings.Replace(expectedQuery, " ", "", -1)) != 0 {
		t.Errorf("Expected query did not match: \n expected: " + expectedQuery + "\n got: " + query)
	}
}

func TestGetCustomQuery(t *testing.T) {

	customQueries := map[string]string{}
	customQueries["custom_query"] = "my_custom_query{job='$SERVICE-$PROJECT-$STAGE'}[$DURATION_SECONDS]"

	ph := NewPrometheusHandler("prometheus", "sockshop", "dev", "carts", nil)

	ph.CustomQueries = customQueries

	start := time.Unix(1571649084, 0)
	end := time.Unix(1571649085, 0)
	query, _ := ph.getMetricQuery("custom_query", start, end)

	expectedQuery := "my_custom_query{job='carts-sockshop-dev'}[1s]"

	if strings.Compare(strings.Replace(query, " ", "", -1), strings.Replace(expectedQuery, " ", "", -1)) != 0 {
		t.Errorf("Expected query did not match: \n expected: " + expectedQuery + "\n got: " + query)
	}
}

func TestGetDefaultFilterExpression(t *testing.T) {

	var customFilters []*keptnevents.SLIFilter

	customFilters = append(customFilters, &keptnevents.SLIFilter{
		Key:   "handler",
		Value: "ItemsController",
	})

	ph := NewPrometheusHandler("prometheus", "sockshop", "dev", "carts", customFilters)

	filterExpression := ph.getDefaultFilterExpression()

	expectedFilterExpression := "job='carts-sockshop-dev-canary',handler='ItemsController'"

	if strings.Compare(strings.Replace(expectedFilterExpression, " ", "", -1), strings.Replace(filterExpression, " ", "", -1)) != 0 {
		t.Errorf("Expected query did not match: \n expected: " + expectedFilterExpression + "\n got: " + filterExpression)
	}
}

func TestGetDefaultFilterExpressionWithOperand(t *testing.T) {

	var customFilters []*keptnevents.SLIFilter

	customFilters = append(customFilters, &keptnevents.SLIFilter{
		Key:   "handler",
		Value: "!='ItemsController'",
	})

	ph := NewPrometheusHandler("prometheus", "sockshop", "dev", "carts", customFilters)

	filterExpression := ph.getDefaultFilterExpression()

	expectedFilterExpression := "job='carts-sockshop-dev-canary',handler!='ItemsController'"

	if strings.Compare(strings.Replace(expectedFilterExpression, " ", "", -1), strings.Replace(filterExpression, " ", "", -1)) != 0 {
		t.Errorf("Expected query did not match: \n expected: " + expectedFilterExpression + "\n got: " + filterExpression)
	}
}

func TestGetDefaultFilterExpressionWithJobName(t *testing.T) {

	var customFilters []*keptnevents.SLIFilter

	customFilters = append(customFilters, &keptnevents.SLIFilter{
		Key:   "job",
		Value: "my-job",
	})

	ph := NewPrometheusHandler("prometheus", "sockshop", "dev", "carts", customFilters)

	filterExpression := ph.getDefaultFilterExpression()

	expectedFilterExpression := "job='my-job'"

	if strings.Compare(strings.Replace(expectedFilterExpression, " ", "", -1), strings.Replace(filterExpression, " ", "", -1)) != 0 {
		t.Errorf("Expected query did not match: \n expected: " + expectedFilterExpression + "\n got: " + filterExpression)
	}
}

func TestGetDefaultFilterExpressionWithSingleQuote(t *testing.T) {

	var customFilters []*keptnevents.SLIFilter

	customFilters = append(customFilters, &keptnevents.SLIFilter{
		Key:   "handler",
		Value: "'ItemsController'",
	})

	ph := NewPrometheusHandler("prometheus", "sockshop", "dev", "carts", customFilters)

	filterExpression := ph.getDefaultFilterExpression()

	expectedFilterExpression := "job='carts-sockshop-dev-canary',handler='ItemsController'"

	if strings.Compare(strings.Replace(expectedFilterExpression, " ", "", -1), strings.Replace(filterExpression, " ", "", -1)) != 0 {
		t.Errorf("Expected query did not match: \n expected: " + expectedFilterExpression + "\n got: " + filterExpression)
	}
}

func TestGetDefaultFilterExpressionWithDoubleQuote(t *testing.T) {

	var customFilters []*keptnevents.SLIFilter

	customFilters = append(customFilters, &keptnevents.SLIFilter{
		Key:   "handler",
		Value: "\"ItemsController\"",
	})

	ph := NewPrometheusHandler("prometheus", "sockshop", "dev", "carts", customFilters)

	filterExpression := ph.getDefaultFilterExpression()

	expectedFilterExpression := "job='carts-sockshop-dev-canary',handler='ItemsController'"

	if strings.Compare(strings.Replace(expectedFilterExpression, " ", "", -1), strings.Replace(filterExpression, " ", "", -1)) != 0 {
		t.Errorf("Expected query did not match: \n expected: " + expectedFilterExpression + "\n got: " + filterExpression)
	}
}

func TestGetSLIValue(t *testing.T) {

	okResponse := `{
		    "status": "success",
		    "data": {
		        "resultType": "vector",
		        "result": [
		            {
		                "metric": {},
		                "value": [
		                    1571649085,
		                    "0.20111420612813372"
		                ]
		            }
		        ]
		    }
		}`

	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(okResponse))
	})

	httpClient, teardown := testingHTTPClient(h)
	defer teardown()

	ph := NewPrometheusHandler("http://prometheus", "sockshop", "dev", "carts", nil)
	ph.HTTPClient = httpClient

	start := strconv.FormatInt(time.Unix(1571649084, 0).UTC().UnixNano(), 10)
	end := strconv.FormatInt(time.Unix(1571649085, 0).UTC().UnixNano(), 10)
	value, _ := ph.GetSLIValue(Throughput, start, end)

	assert.EqualValues(t, value, 0.20111420612813372)
}

func TestGetSLIValueWithEmptyResult(t *testing.T) {

	okResponse := `{
		    "status": "success",
		    "data": {
		        "resultType": "vector",
		        "result": []
		    }
		}`

	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(okResponse))
	})

	httpClient, teardown := testingHTTPClient(h)
	defer teardown()

	ph := NewPrometheusHandler("http://prometheus", "sockshop", "dev", "carts", nil)
	ph.HTTPClient = httpClient

	start := strconv.FormatInt(time.Unix(1571649084, 0).UTC().UnixNano(), 10)
	end := strconv.FormatInt(time.Unix(1571649085, 0).UTC().UnixNano(), 10)
	value, _ := ph.GetSLIValue(Throughput, start, end)

	assert.EqualValues(t, value, 0.0)
}

func TestGetSLIValueWithErrorResponse(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// w.Write([]byte(response))
		w.WriteHeader(http.StatusBadRequest)
	})

	httpClient, teardown := testingHTTPClient(h)
	defer teardown()

	ph := NewPrometheusHandler("http://prometheus", "sockshop", "dev", "carts", nil)
	ph.HTTPClient = httpClient

	start := strconv.FormatInt(time.Unix(1571649084, 0).UTC().UnixNano(), 10)
	end := strconv.FormatInt(time.Unix(1571649085, 0).UTC().UnixNano(), 10)
	value, err := ph.GetSLIValue(Throughput, start, end)

	assert.EqualValues(t, value, 0.0)
	assert.NotNil(t, err, nil)
}
