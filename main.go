package main

import (
	"context"
	"errors"
	"fmt"
	"github.com/keptn-contrib/prometheus-sli-service/lib/prometheus"
	"gopkg.in/yaml.v2"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"log"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/cloudevents/sdk-go/pkg/cloudevents"
	"github.com/cloudevents/sdk-go/pkg/cloudevents/client"
	cloudeventshttp "github.com/cloudevents/sdk-go/pkg/cloudevents/transport/http"
	"github.com/cloudevents/sdk-go/pkg/cloudevents/types"
	"github.com/google/uuid"
	"github.com/kelseyhightower/envconfig"
	keptnevents "github.com/keptn/go-utils/pkg/events"
	keptnutils "github.com/keptn/go-utils/pkg/utils"
)

const configservice = "CONFIGURATION_SERVICE"
const eventbroker = "EVENTBROKER"

type envConfig struct {
	// Port on which to listen for cloudevents
	Port int    `envconfig:"RCV_PORT" default:"8080"`
	Path string `envconfig:"RCV_PATH" default:"/"`
}

type prometheusCredentials struct {
	URL      string `json:"url",yaml:"url"`
	User     string `json:"user",yaml:"user"`
	Password string `json:"user",yaml:"user"`
}

func main() {
	var env envConfig
	if err := envconfig.Process("", &env); err != nil {
		log.Fatalf("Failed to process env var: %s", err)
	}
	os.Exit(_main(os.Args[1:], env))
}

func _main(args []string, env envConfig) int {

	ctx := context.Background()

	t, err := cloudeventshttp.New(
		cloudeventshttp.WithPort(env.Port),
		cloudeventshttp.WithPath(env.Path),
	)

	if err != nil {
		log.Fatalf("failed to create transport, %v", err)
	}
	c, err := client.New(t)
	if err != nil {
		log.Fatalf("failed to create client, %v", err)
	}

	log.Fatalf("failed to start receiver: %s", c.StartReceiver(ctx, gotEvent))

	return 0
}

func gotEvent(ctx context.Context, event cloudevents.Event) error {

	switch event.Type() {
	case keptnevents.InternalGetSLIEventType:
		return retrieveMetrics(event) // backwards compatibility to Keptn versions <= 0.5.x
	default:
		return errors.New("received unknown event type")
	}
}

func retrieveMetrics(event cloudevents.Event) error {
	eventData := &keptnevents.InternalGetSLIEventData{}
	err := event.DataAs(eventData)

	if err != nil {
		return err
	}

	prometheusApiURL, err := getPrometheusApiURL(eventData.Project)

	if err != nil {
		return err
	}

	prometheusHandler, err := prometheus.NewPrometheusHandler(
		prometheusApiURL,
		eventData.Project,
		eventData.Stage,
		eventData.Service,
	)

	if err != nil {
		return err
	}

	var sliResults []*keptnevents.SLIResult

	for _, indicator := range eventData.Indicators {
		sliValue, err := prometheusHandler.GetSLIValue(indicator, eventData.Start, eventData.End)
		if err == nil {
			sliResults = append(sliResults, &keptnevents.SLIResult{
				Metric: indicator,
				Value:  sliValue,
			})
		}
	}
	return nil
}

func getPrometheusApiURL(project string) (string, error) {
	// check if secret 'prometheus-credentials-<project> exists
	kubeClient, err := keptnutils.GetKubeAPI(true)
	secret, err := kubeClient.Secrets("keptn").Get("prometheus-credentials-"+project, v1.GetOptions{})

	// return cluster-internal prometheus URL if no secret has been found
	if err != nil {
		return "http://prometheus-service.monitoring.svc.cluster.local:8080", nil
	}

	/*
		data format:
		url: string
		user: string
		password: string
	*/
	var pc prometheusCredentials
	err = yaml.Unmarshal(secret.Data["prometheus-credentials"], pc)

	if err != nil {
		return "", errors.New("invalid credentials format found in secret 'prometheus-credentials-" + project)
	}

	prometheusURL := pc.URL

	if strings.HasPrefix(prometheusURL, "https://") && pc.User != "" && pc.Password != "" {
		prometheusURL = strings.TrimPrefix(prometheusURL, "https://")
		prometheusURL = "https://" + url.QueryEscape(pc.User) + ":" + url.QueryEscape(pc.Password) + "@" + prometheusURL
	} else if strings.HasPrefix(prometheusURL, "http://") {
		prometheusURL = strings.TrimPrefix(prometheusURL, "http://")
		prometheusURL = "http://" + url.QueryEscape(pc.User) + ":" + url.QueryEscape(pc.Password) + "@" + prometheusURL
	} else {
		// assume https transport
		prometheusURL = "https://" + url.QueryEscape(pc.User) + ":" + url.QueryEscape(pc.Password) + "@" + prometheusURL
	}

	return prometheusURL, nil
}

func sendInternalGetSLIDoneEvent(shkeptncontext string, project string,
	service string, stage string, indicatorValues []*keptnevents.SLIResult) error {

	source, _ := url.Parse("gatekeeper-service")
	contentType := "application/json"

	getSLIEvent := keptnevents.InternalGetSLIDoneEventData{
		Project:         project,
		Service:         service,
		Stage:           stage,
		IndicatorValues: indicatorValues,
	}
	event := cloudevents.Event{
		Context: cloudevents.EventContextV02{
			ID:          uuid.New().String(),
			Time:        &types.Timestamp{Time: time.Now()},
			Type:        keptnevents.InternalGetSLIDoneEventType,
			Source:      types.URLRef{URL: *source},
			ContentType: &contentType,
			Extensions:  map[string]interface{}{"shkeptncontext": shkeptncontext},
		}.AsV02(),
		Data: getSLIEvent,
	}

	return sendEvent(event)
}

func sendEvent(event cloudevents.Event) error {
	endPoint, err := getServiceEndpoint(eventbroker)
	if err != nil {
		return errors.New("Failed to retrieve endpoint of eventbroker. %s" + err.Error())
	}

	if endPoint.Host == "" {
		return errors.New("Host of eventbroker not set")
	}

	transport, err := cloudeventshttp.New(
		cloudeventshttp.WithTarget(endPoint.String()),
		cloudeventshttp.WithEncoding(cloudeventshttp.StructuredV02),
	)
	if err != nil {
		return errors.New("Failed to create transport:" + err.Error())
	}

	c, err := client.New(transport)
	if err != nil {
		return errors.New("Failed to create HTTP client:" + err.Error())
	}

	if _, err := c.Send(context.Background(), event); err != nil {
		return errors.New("Failed to send cloudevent:, " + err.Error())
	}
	return nil
}

// getServiceEndpoint gets an endpoint stored in an environment variable and sets http as default scheme
func getServiceEndpoint(service string) (url.URL, error) {
	url, err := url.Parse(os.Getenv(service))
	if err != nil {
		return *url, fmt.Errorf("Failed to retrieve value from ENVIRONMENT_VARIABLE: %s", service)
	}

	if url.Scheme == "" {
		url.Scheme = "http"
	}

	return *url, nil
}
