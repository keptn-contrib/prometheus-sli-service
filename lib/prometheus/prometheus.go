package prometheus

import (
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	keptnevents "github.com/keptn/go-utils/pkg/events"
)

const Throughput = "throughput"
const ErrorRate = "error_rate"
const RequestLatencyP50 = "request_latency_p50"
const RequestLatencyP90 = "request_latency_p90"
const RequestLatencyP95 = "request_latency_p95"

type prometheusResponse struct {
	Status string `json:"status"`
	Data   struct {
		ResultType string `json:"resultType"`
		Result     []struct {
			Metric struct {
			} `json:"metric"`
			Value []interface{} `json:"value"`
		} `json:"result"`
	} `json:"data"`
}

// Handler interacts with a prometheus API endpoint
type Handler struct {
	ApiURL        string
	Username      string
	Password      string
	Project       string
	Stage         string
	Service       string
	HTTPClient    *http.Client
	CustomFilters []*keptnevents.SLIFilter
}

// NewPrometheusHandler returns a new prometheus handler that interacts with the Prometheus REST API
func NewPrometheusHandler(apiURL string, project string, stage string, service string, customFilters []*keptnevents.SLIFilter) (*Handler, error) {
	ph := &Handler{
		ApiURL:        apiURL,
		Project:       project,
		Stage:         stage,
		Service:       service,
		HTTPClient:    &http.Client{},
		CustomFilters: customFilters,
	}

	return ph, nil
}

func (ph *Handler) GetSLIValue(metric string, start string, end string) (float64, error) {
	http.DefaultTransport.(*http.Transport).TLSClientConfig = &tls.Config{InsecureSkipVerify: true}

	startUnix, err := parseUnixTimestamp(start)
	if err != nil {
		return 0, err
	}
	endUnix, _ := parseUnixTimestamp(end)
	if err != nil {
		return 0, err
	}
	query, err := ph.getMetricQuery(metric, startUnix, endUnix)
	if err != nil {
		return 0, err
	}
	queryString := ph.ApiURL + "/api/v1/query?query=" + url.QueryEscape(query) + "&time=" + strconv.FormatInt(endUnix.Unix(), 10)
	req, err := http.NewRequest("GET", queryString, nil)
	req.Header.Set("Content-Type", "application/json")

	resp, err := ph.HTTPClient.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	body, _ := ioutil.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return 0, errors.New("metric could not be received")
	}

	prometheusResult := &prometheusResponse{}

	err = json.Unmarshal(body, prometheusResult)
	if err != nil {
		return 0, err
	}

	if len(prometheusResult.Data.Result) == 0 || len(prometheusResult.Data.Result[0].Value) == 0 {
		// for the error rate query, the result is received with no value if the error rate is 0, so we have to assume that's OK at this point
		return 0, nil
	}

	parsedValue := fmt.Sprintf("%v", prometheusResult.Data.Result[0].Value[1])
	floatValue, err := strconv.ParseFloat(parsedValue, 64)
	if err != nil {
		return 0, nil
	}
	return floatValue, nil
}

func (ph *Handler) getMetricQuery(metric string, start time.Time, end time.Time) (string, error) {
	switch metric {
	case Throughput:
		return ph.getThroughputQuery(start, end), nil
	case ErrorRate:
		return ph.getErrorRateQuery(start, end), nil
	case RequestLatencyP50:
		return ph.getRequestLatencyQuery("50", start, end), nil
	case RequestLatencyP90:
		return ph.getRequestLatencyQuery("90", start, end), nil
	case RequestLatencyP95:
		return ph.getRequestLatencyQuery("95", start, end), nil
	default:
		return "", errors.New("unsupported SLI")
	}
}

func (ph *Handler) getThroughputQuery(start time.Time, end time.Time) string {
	filterExpr := ph.getDefaultFilterExpression()
	durationString := strconv.FormatInt(getDurationInSeconds(start, end), 10) + "s"
	// e.g. sum(rate(http_requests_total{job="carts-sockshop-dev"}[30m]))&time=1571649085

	/*
		{
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
		}
	*/
	// TODO: allow user-defined custom metrics
	return "sum(rate(http_requests_total{" + filterExpr + "}[" + durationString + "]))"
}

func (ph *Handler) getErrorRateQuery(start time.Time, end time.Time) string {
	filterExpr := ph.getDefaultFilterExpression()
	durationString := strconv.FormatInt(getDurationInSeconds(start, end), 10) + "s"
	// e.g. sum(rate(http_requests_total{job="carts-sockshop-dev",status!~'2..'}[30m]))/sum(rate(http_requests_total{job="carts-sockshop-dev"}[30m]))&time=1571649085

	/*
		with value:
		{
		    "status": "success",
		    "data": {
		        "resultType": "vector",
		        "result": [
		            {
		                "metric": {},
		                "value": [
		                    1571649085,
		                    "1.00505917125441"
		                ]
		            }
		        ]
		    }
		}

		no value (error rate 0):
		{
		    "status": "success",
		    "data": {
		        "resultType": "vector",
		        "result": []
		    }
		}
	*/
	// TODO: allow user-defined custom metrics
	return "sum(rate(http_requests_total{" + filterExpr + ",status!~'2..'}[" + durationString + "]))/sum(rate(http_requests_total{" + filterExpr + "}[" + durationString + "]))"
}

func (ph *Handler) getRequestLatencyQuery(percentile string, start time.Time, end time.Time) string {
	filterExpr := ph.getDefaultFilterExpression()
	durationString := strconv.FormatInt(getDurationInSeconds(start, end), 10) + "s"
	// e.g. histogram_quantile(0.95, sum(rate(http_response_time_milliseconds_bucket{job='carts-sockshop-dev'}[30m])) by (le))&time=1571649085

	/*
		{
		    "status": "success",
		    "data": {
		        "resultType": "vector",
		        "result": [
		            {
		                "metric": {},
		                "value": [
		                    1571649085,
		                    "4.607481671642585"
		                ]
		            }
		        ]
		    }
		}
	*/
	// TODO: allow user-defined custom metrics
	return "histogram_quantile(0." + percentile + ",sum(rate(http_response_time_milliseconds_bucket{" + filterExpr + "}[" + durationString + "]))by(le))"
}

func (ph *Handler) getDefaultFilterExpression() string {
	filterExpression := "job='" + ph.Service + "-" + ph.Project + "-" + ph.Stage + "'"
	if ph.CustomFilters != nil && len(ph.CustomFilters) > 0 {
		for _, filter := range ph.CustomFilters {
			/* if no operator has been included in the label filter, use exact matching (=), e.g.
			e.g.:
			key: handler
			value: ItemsController
			*/
			if !strings.HasPrefix(filter.Value, "=") && !strings.HasPrefix(filter.Value, "!=") && !strings.HasPrefix(filter.Value, "=~") && !strings.HasPrefix(filter.Value, "!~") {
				filter.Value = strings.Replace(filter.Value, "'", "", -1)
				filter.Value = strings.Replace(filter.Value, "\"", "", -1)
				filterExpression = filterExpression + "," + filter.Key + "='" + filter.Value + "'"
			} else {
				/* if a valid operator (=, !=, =~, !~) is prepended to the value, use that one
				e.g.:
				key: handler
				value: !=HealthCheckController

				OR

				key: handler
				value: =~.+ItemsController|.+VersionController
				*/
				filter.Value = strings.Replace(filter.Value, "\"", "'", -1)
				filterExpression = filterExpression + "," + filter.Key + filter.Value
			}
		}
	}
	return filterExpression
}

func parseUnixTimestamp(timestamp string) (time.Time, error) {
	parsedTime, err := time.Parse(time.RFC3339, timestamp)
	if err == nil {
		return parsedTime, nil
	}

	timestampInt, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil {
		return time.Now(), err
	}
	unix := time.Unix(timestampInt, 0)
	return unix, nil
}

func getDurationInSeconds(start, end time.Time) int64 {
	seconds := end.Sub(start).Seconds()
	return int64(math.Ceil(seconds))
}
