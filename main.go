package main

import (
	"context"
	"errors"
	"fmt"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"log"
	"math"
	"net/url"
	"os"
	"strings"

	"github.com/keptn-contrib/prometheus-sli-service/lib/prometheus"
	"gopkg.in/yaml.v2"

	cloudevents "github.com/cloudevents/sdk-go/v2"

	"github.com/kelseyhightower/envconfig"
	keptncommon "github.com/keptn/go-utils/pkg/lib/keptn"
	keptnv2 "github.com/keptn/go-utils/pkg/lib/v0_2_0"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	v1 "k8s.io/client-go/kubernetes/typed/core/v1"
)

const configservice = "CONFIGURATION_SERVICE"
const eventbroker = "EVENTBROKER"
const sliResourceURI = "prometheus/sli.yaml"

type envConfig struct {
	// Port on which to listen for cloudevents
	Port int    `envconfig:"RCV_PORT" default:"8080"`
	Path string `envconfig:"RCV_PATH" default:"/"`
}

type prometheusCredentials struct {
	URL      string `json:"url" yaml:"url"`
	User     string `json:"user" yaml:"user"`
	Password string `json:"password" yaml:"password"`
}

var namespace = os.Getenv("POD_NAMESPACE")

func main() {
	var env envConfig
	if err := envconfig.Process("", &env); err != nil {
		log.Fatalf("Failed to process env var: %s", err)
	}
	os.Exit(_main(os.Args[1:], env))
}

func _main(args []string, env envConfig) int {

	ctx := context.Background()
	ctx = cloudevents.WithEncodingStructured(ctx)

	p, err := cloudevents.NewHTTP(cloudevents.WithPath(env.Path), cloudevents.WithPort(env.Port))
	if err != nil {
		log.Fatalf("failed to create client, %v", err)
	}
	c, err := cloudevents.NewClient(p)
	if err != nil {
		log.Fatalf("failed to create client, %v", err)
	}
	log.Fatal(c.StartReceiver(ctx, gotEvent))

	return 0
}

func gotEvent(event cloudevents.Event) error {

	switch event.Type() {
	case keptnv2.GetTriggeredEventType(keptnv2.GetSLITaskName):
		return processEvent(event) // backwards compatibility to Keptn versions <= 0.5.x
	default:
		return errors.New("received unknown event type")
	}
}

func processEvent(event cloudevents.Event) error {

	eventData := &keptnv2.GetSLITriggeredEventData{}
	err := event.DataAs(eventData)
	if err != nil {
		return err
	}

	// don't continue if SLIProvider is not prometheus
	if eventData.GetSLI.SLIProvider != "prometheus" {
		return nil
	}

	// 1: send .started event
	var sliResults = []*keptnv2.SLIResult{}
	if err = sendGetSLIStartedEvent(event, eventData); err != nil {
		if err = sendGetSLIFinishedEvent(event, eventData, sliResults, err); err != nil {
			return err
		}
	}

	// 2: try to fetch metrics
	if sliResults, err = retrieveMetrics(event, eventData); err != nil {
		if err = sendGetSLIFinishedEvent(event, eventData, sliResults, err); err != nil {
			return err
		}
	}

	// 3: send .finished event
	return sendGetSLIFinishedEvent(event, eventData, sliResults, nil)
}

func retrieveMetrics(event cloudevents.Event, eventData *keptnv2.GetSLITriggeredEventData) ([]*keptnv2.SLIResult, error) {
	var shkeptncontext string
	event.Context.ExtensionAs("shkeptncontext", &shkeptncontext)

	stdLogger := keptncommon.NewLogger(shkeptncontext, event.Context.GetID(), "prometheus-sli-service")
	stdLogger.Info("Retrieving Prometheus metrics")

	clusterConfig, err := rest.InClusterConfig()
	if err != nil {
		stdLogger.Error("could not create Kubernetes client")
		return nil, errors.New("could not create Kubernetes client")
	}

	kubeClient, err := kubernetes.NewForConfig(clusterConfig)
	if err != nil {
		stdLogger.Error("could not create Kubernetes client")
		return nil, errors.New("could not create Kubernetes client")
	}

	prometheusApiURL, err := getPrometheusApiURL(eventData.Project, kubeClient.CoreV1(), stdLogger)
	if err != nil {
		return nil, err
	}

	eventBrokerURL := os.Getenv(eventbroker)
	if eventBrokerURL == "" {
		eventBrokerURL = "http://event-broker/keptn"
	}

	keptnHandler, err := keptnv2.NewKeptn(&event, keptncommon.KeptnOpts{
		EventBrokerURL: eventBrokerURL,
	})
	if err != nil {
		stdLogger.Error("Failed to get custom queries for project " + eventData.Project)
		stdLogger.Error(err.Error())
		return nil, err
	}
	// retrieve custom metrics for project
	projectCustomQueries, err := getCustomQueries(keptnHandler, eventData.Project, eventData.Stage, eventData.Service, stdLogger)
	if err != nil {
		stdLogger.Error("Failed to get custom queries for project " + eventData.Project)
		stdLogger.Error(err.Error())
		return nil, err
	}

	prometheusHandler := prometheus.NewPrometheusHandler(prometheusApiURL, eventData.Project, eventData.Stage, eventData.Service, eventData.GetSLI.CustomFilters)

	if projectCustomQueries != nil {
		prometheusHandler.CustomQueries = projectCustomQueries
	}

	var sliResults []*keptnv2.SLIResult

	for _, indicator := range eventData.GetSLI.Indicators {
		stdLogger.Info("Fetching indicator: " + indicator)
		sliValue, err := prometheusHandler.GetSLIValue(indicator, eventData.GetSLI.Start, eventData.GetSLI.End, stdLogger)
		if err != nil {
			sliResults = append(sliResults, &keptnv2.SLIResult{
				Metric:  indicator,
				Value:   0,
				Success: false,
				Message: err.Error(),
			})
		} else if math.IsNaN(sliValue) {
			sliResults = append(sliResults, &keptnv2.SLIResult{
				Metric:  indicator,
				Value:   0,
				Success: false,
				Message: "SLI value is NaN",
			})
		} else {
			sliResults = append(sliResults, &keptnv2.SLIResult{
				Metric:  indicator,
				Value:   sliValue,
				Success: true,
			})
		}
	}
	return sliResults, nil
}

// getCustomQueries returns custom queries as stored in configuration store
func getCustomQueries(keptnHandler *keptnv2.Keptn, project string, stage string, service string, logger *keptncommon.Logger) (map[string]string, error) {
	logger.Info("Checking for custom SLI queries")

	customQueries, err := keptnHandler.GetSLIConfiguration(project, stage, service, sliResourceURI)
	if err != nil {
		return nil, err
	}

	return customQueries, nil
}

func getPrometheusApiURL(project string, kubeClient v1.CoreV1Interface, logger *keptncommon.Logger) (string, error) {
	logger.Info("Checking if external prometheus instance has been defined for project " + project)
	// check if secret 'prometheus-credentials-<project> exists

	secret, err := kubeClient.Secrets(namespace).Get("prometheus-credentials-"+project, metav1.GetOptions{})

	// return cluster-internal prometheus URL if no secret has been found
	if err != nil {
		logger.Info("No external prometheus instance defined for project " + project + ". Using default: http://prometheus-service.monitoring.svc.cluster.local:8080")
		return "http://prometheus-service.monitoring.svc.cluster.local:8080", nil
	}

	/*
		required data format of the secret:
		  url: string
		  user: string
		  password: string
	*/
	pc := &prometheusCredentials{}
	err = yaml.Unmarshal(secret.Data["prometheus-credentials"], pc)

	if err != nil {
		logger.Error("Could not parse credentials for external prometheus instance: " + err.Error())
		return "", errors.New("invalid credentials format found in secret 'prometheus-credentials-" + project)
	}
	logger.Info("Using external prometheus instance for project " + project + ": " + pc.URL)
	prometheusURL := generatePrometheusURL(pc)

	return prometheusURL, nil
}

func generatePrometheusURL(pc *prometheusCredentials) string {
	prometheusURL := pc.URL

	credentialsString := ""

	if pc.User != "" && pc.Password != "" {
		credentialsString = url.QueryEscape(pc.User) + ":" + url.QueryEscape(pc.Password) + "@"
	}
	if strings.HasPrefix(prometheusURL, "https://") {
		prometheusURL = strings.TrimPrefix(prometheusURL, "https://")
		prometheusURL = "https://" + credentialsString + prometheusURL
	} else if strings.HasPrefix(prometheusURL, "http://") {
		prometheusURL = strings.TrimPrefix(prometheusURL, "http://")
		prometheusURL = "http://" + credentialsString + prometheusURL
	} else {
		// assume https transport
		prometheusURL = "https://" + credentialsString + prometheusURL
	}
	return strings.Replace(prometheusURL, " ", "", -1)
}

func sendGetSLIStartedEvent(inputEvent cloudevents.Event, eventData *keptnv2.GetSLITriggeredEventData) error {

	source, _ := url.Parse("prometheus-sli-service")

	getSLIStartedEvent := keptnv2.GetSLIStartedEventData{
		EventData: keptnv2.EventData{
			Project: eventData.Project,
			Stage:   eventData.Stage,
			Service: eventData.Service,
			Labels:  eventData.Labels,
			Status:  keptnv2.StatusSucceeded,
			Result:  keptnv2.ResultPass,
		},
	}

	keptnContext, err := inputEvent.Context.GetExtension("shkeptncontext")

	if err != nil {
		return fmt.Errorf("could not determine keptnContext of input event: %s", err.Error())
	}

	event := cloudevents.NewEvent()
	event.SetType(keptnv2.GetStartedEventType(keptnv2.GetSLITaskName))
	event.SetSource(source.String())
	event.SetDataContentType(cloudevents.ApplicationJSON)
	event.SetExtension("shkeptncontext", keptnContext)
	event.SetExtension("triggeredid", inputEvent.ID())
	event.SetData(cloudevents.ApplicationJSON, getSLIStartedEvent)

	return sendEvent(event)
}

func sendGetSLIFinishedEvent(inputEvent cloudevents.Event, eventData *keptnv2.GetSLITriggeredEventData, indicatorValues []*keptnv2.SLIResult, err error) error {
	source, _ := url.Parse("prometheus-sli-service")

	var status = keptnv2.StatusSucceeded
	var result = keptnv2.ResultPass
	var message = ""

	if err != nil {
		status = keptnv2.StatusErrored
		result = keptnv2.ResultFailed
		message = err.Error()
	}

	getSLIEvent := keptnv2.GetSLIFinishedEventData{
		EventData: keptnv2.EventData{
			Project: eventData.Project,
			Stage:   eventData.Stage,
			Service: eventData.Service,
			Labels:  eventData.Labels,
			Status:  status,
			Result:  result,
			Message: message,
		},
		GetSLI: struct {
			Start           string               `json:"start"`
			End             string               `json:"end"`
			IndicatorValues []*keptnv2.SLIResult `json:"indicatorValues"`
		}{
			IndicatorValues: indicatorValues,
			Start:           eventData.GetSLI.Start,
			End:             eventData.GetSLI.End,
		},
	}
	keptnContext, err := inputEvent.Context.GetExtension("shkeptncontext")

	if err != nil {
		return fmt.Errorf("could not determine keptnContext of input event: %s", err.Error())
	}

	event := cloudevents.NewEvent()
	event.SetType(keptnv2.GetFinishedEventType(keptnv2.GetSLITaskName))
	event.SetSource(source.String())
	event.SetDataContentType(cloudevents.ApplicationJSON)
	event.SetExtension("shkeptncontext", keptnContext)
	event.SetExtension("triggeredid", inputEvent.ID())
	event.SetData(cloudevents.ApplicationJSON, getSLIEvent)

	return sendEvent(event)
}

func sendEvent(event cloudevents.Event) error {
	keptnHandler, err := keptnv2.NewKeptn(&event, keptncommon.KeptnOpts{})
	if err != nil {
		return err
	}

	return keptnHandler.SendCloudEvent(event)
}
