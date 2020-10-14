package main

import (
	"context"
	"errors"
	"log"
	"math"
	"net/url"
	"os"
	"strings"
	"time"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	"github.com/keptn-contrib/prometheus-sli-service/lib/prometheus"
	"gopkg.in/yaml.v2"

	"github.com/cloudevents/sdk-go/pkg/cloudevents"
	"github.com/cloudevents/sdk-go/pkg/cloudevents/client"
	cloudeventshttp "github.com/cloudevents/sdk-go/pkg/cloudevents/transport/http"
	"github.com/cloudevents/sdk-go/pkg/cloudevents/types"

	"github.com/google/uuid"
	"github.com/kelseyhightower/envconfig"

	keptn "github.com/keptn/go-utils/pkg/lib"
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
	case keptn.InternalGetSLIEventType:
		return retrieveMetrics(event) // backwards compatibility to Keptn versions <= 0.5.x
	default:
		return errors.New("received unknown event type")
	}
}

func retrieveMetrics(event cloudevents.Event) error {
	var shkeptncontext string
	event.Context.ExtensionAs("shkeptncontext", &shkeptncontext)
	eventData := &keptn.InternalGetSLIEventData{}
	err := event.DataAs(eventData)
	if err != nil {
		return err
	}

	// don't continue if SLIProvider is not prometheus
	if eventData.SLIProvider != "prometheus" {
		return nil
	}

	stdLogger := keptn.NewLogger(shkeptncontext, event.Context.GetID(), "prometheus-sli-service")
	stdLogger.Info("Retrieving Prometheus metrics")

	clusterConfig, err := rest.InClusterConfig()
	if err != nil {
		stdLogger.Error("could not create Kubernetes client")
		return errors.New("could not create Kubernetes client")
	}

	kubeClient, err := kubernetes.NewForConfig(clusterConfig)
	if err != nil {
		stdLogger.Error("could not create Kubernetes client")
		return errors.New("could not create Kubernetes client")
	}

	prometheusApiURL, err := getPrometheusApiURL(eventData.Project, kubeClient.CoreV1(), stdLogger)
	if err != nil {
		return err
	}

	eventBrokerURL := os.Getenv(eventbroker)
	if eventBrokerURL == "" {
		eventBrokerURL = "http://event-broker/keptn"
	}
	keptnHandler, err := keptn.NewKeptn(&event, keptn.KeptnOpts{
		EventBrokerURL: eventBrokerURL,
	})
	if err != nil {
		stdLogger.Error("Failed to get custom queries for project " + eventData.Project)
		stdLogger.Error(err.Error())
		return err
	}
	// retrieve custom metrics for project
	projectCustomQueries, err := getCustomQueries(keptnHandler, eventData.Project, eventData.Stage, eventData.Service, stdLogger)
	if err != nil {
		stdLogger.Error("Failed to get custom queries for project " + eventData.Project)
		stdLogger.Error(err.Error())
		return err
	}

	prometheusHandler := prometheus.NewPrometheusHandler(prometheusApiURL, eventData.Project, eventData.Stage, eventData.Service, eventData.CustomFilters)

	if projectCustomQueries != nil {
		prometheusHandler.CustomQueries = projectCustomQueries
	}

	var sliResults []*keptn.SLIResult

	for _, indicator := range eventData.Indicators {
		stdLogger.Info("Fetching indicator: " + indicator)
		sliValue, err := prometheusHandler.GetSLIValue(indicator, eventData.Start, eventData.End, stdLogger)
		if err != nil {
			sliResults = append(sliResults, &keptn.SLIResult{
				Metric:  indicator,
				Value:   0,
				Success: false,
				Message: err.Error(),
			})
		} else if math.IsNaN(sliValue) {
			sliResults = append(sliResults, &keptn.SLIResult{
				Metric:  indicator,
				Value:   0,
				Success: false,
				Message: "SLI value is NaN",
			})
		} else {
			sliResults = append(sliResults, &keptn.SLIResult{
				Metric:  indicator,
				Value:   sliValue,
				Success: true,
			})
		}
	}

	return sendInternalGetSLIDoneEvent(keptnHandler,
		sliResults, eventData.Start, eventData.End, eventData.TestStrategy, eventData.DeploymentStrategy, eventData.Labels)
}

// getCustomQueries returns custom queries as stored in configuration store
func getCustomQueries(keptnHandler *keptn.Keptn, project string, stage string, service string, logger *keptn.Logger) (map[string]string, error) {
	logger.Info("Checking for custom SLI queries")

	customQueries, err := keptnHandler.GetSLIConfiguration(project, stage, service, sliResourceURI)
	if err != nil {
		return nil, err
	}

	return customQueries, nil
}

func getPrometheusApiURL(project string, kubeClient v1.CoreV1Interface, logger *keptn.Logger) (string, error) {
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

func sendInternalGetSLIDoneEvent(keptnHandler *keptn.Keptn, indicatorValues []*keptn.SLIResult, start string, end string, testStrategy string, deploymentStrategy string, labels map[string]string) error {

	source, _ := url.Parse("prometheus-sli-service")
	contentType := "application/json"

	getSLIEvent := keptn.InternalGetSLIDoneEventData{
		Project:            keptnHandler.KeptnBase.Project,
		Service:            keptnHandler.KeptnBase.Service,
		Stage:              keptnHandler.KeptnBase.Stage,
		IndicatorValues:    indicatorValues,
		Start:              start,
		End:                end,
		TestStrategy:       testStrategy,
		DeploymentStrategy: deploymentStrategy,
		Labels:             labels,
	}
	event := cloudevents.Event{
		Context: cloudevents.EventContextV02{
			ID:          uuid.New().String(),
			Time:        &types.Timestamp{Time: time.Now()},
			Type:        keptn.InternalGetSLIDoneEventType,
			Source:      types.URLRef{URL: *source},
			ContentType: &contentType,
			Extensions:  map[string]interface{}{"shkeptncontext": keptnHandler.KeptnContext},
		}.AsV02(),
		Data: getSLIEvent,
	}

	return keptnHandler.SendCloudEvent(event)
}
