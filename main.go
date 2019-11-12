package main

import (
	"context"
	"errors"
	"fmt"
	"github.com/keptn-contrib/prometheus-sli-service/lib/prometheus"
	"gopkg.in/yaml.v2"
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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	v1 "k8s.io/client-go/kubernetes/typed/core/v1"
)

const configservice = "CONFIGURATION_SERVICE"
const eventbroker = "EVENTBROKER"

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
	var shkeptncontext string
	event.Context.ExtensionAs("shkeptncontext", &shkeptncontext)
	eventData := &keptnevents.InternalGetSLIEventData{}
	err := event.DataAs(eventData)
	if err != nil {
		return err
	}

	// don't continue if SLIProvider != prometheus
	if eventData.SLIProvider != "prometheus" {
		return nil
	}

	stdLogger := keptnutils.NewLogger(shkeptncontext, event.Context.GetID(), "prometheus-sli-service")
	stdLogger.Info("Retrieving prometheus metrics")
	kubeClient, err := keptnutils.GetKubeAPI(true)
	if err != nil {
		stdLogger.Error("could not create kube client")
		return errors.New("could not create kube client")
	}
	prometheusApiURL, err := getPrometheusApiURL(eventData.Project, kubeClient, stdLogger)

	if err != nil {
		return err
	}

	// get custom metrics for Keptn installation
	customQueries, err := getGlobalCustomQueries(kubeClient, stdLogger)

	if err != nil {
		log.Fatal(err)
		return err
	}

	// get custom metrics for project
	projectCustomQueries, err := getCustomQueriesForProject(eventData.Project, kubeClient, stdLogger)

	if err != nil {
		log.Fatal(err)
		return err
	}

	log.Printf("Custom Query Config\n")

	// make sure custom queries exists
	if customQueries == nil {
		customQueries = make(map[string]string)
	} else {
		for k, v := range customQueries {
			log.Printf("\tFound custom query %s with value %s\n", k, v)
		}
	}

	if projectCustomQueries != nil {
		log.Println("Merging custom queries with projectCustomQueries")
		// merge global custom queries and project custom queries
		for k, v := range projectCustomQueries {
			// overwrite / append project custom query on global custom queries
			customQueries[k] = v
			log.Printf("\tOverwriting custom query %s with value %s\n", k, v)
		}
	}

	prometheusHandler := prometheus.NewPrometheusHandler(prometheusApiURL, eventData.Project, eventData.Stage, eventData.Service, eventData.CustomFilters)

	if customQueries != nil {
		prometheusHandler.CustomQueries = customQueries
	}

	var sliResults []*keptnevents.SLIResult

	for _, indicator := range eventData.Indicators {
		stdLogger.Info("Fetching indicator: " + indicator)
		sliValue, err := prometheusHandler.GetSLIValue(indicator, eventData.Start, eventData.End)
		if err != nil {
			sliResults = append(sliResults, &keptnevents.SLIResult{
				Metric:  indicator,
				Value:   0,
				Success: false,
				Message: err.Error(),
			})
		} else {
			sliResults = append(sliResults, &keptnevents.SLIResult{
				Metric:  indicator,
				Value:   sliValue,
				Success: true,
			})
		}
	}
	return sendInternalGetSLIDoneEvent(shkeptncontext, eventData.Project, eventData.Service, eventData.Stage,
		sliResults, eventData.Start, eventData.End, eventData.TestStrategy)
}

const keptnPrometheusSliConfigMapName = "prometheus-sli-service-config"

// Return Custom Queries for Keptn Installation
func getGlobalCustomQueries(kubeClient v1.CoreV1Interface, logger *keptnutils.Logger) (map[string]string, error) {
	logger.Info(fmt.Sprintf("Checking for custom SLI queries for Keptn installation (querying %s)", keptnPrometheusSliConfigMapName))

	configMap, err := kubeClient.ConfigMaps("keptn").Get(keptnPrometheusSliConfigMapName, metav1.GetOptions{})
	if err != nil {
		logger.Info("No global custom queries defined")
		return nil, nil
	}

	customQueries := make(map[string]string)
	err = yaml.Unmarshal([]byte(configMap.Data["custom-queries"]), &customQueries)

	if err != nil {
		logger.Info("Global custom queries found, but could not parse them: " + err.Error())
		return nil, err
	}
	logger.Info("Global custom queries found and parsed")
	return customQueries, nil
}

// Return Custom Queries for Keptn Installation
func getCustomQueriesForProject(project string, kubeClient v1.CoreV1Interface, logger *keptnutils.Logger) (map[string]string, error) {
	logger.Info(fmt.Sprintf("Checking for custom SLI queries for Keptn installation (querying %s)", keptnPrometheusSliConfigMapName))

	configMap, err := kubeClient.ConfigMaps("keptn").Get(keptnPrometheusSliConfigMapName+"-"+project, metav1.GetOptions{})
	if err != nil {
		logger.Info("No custom queries defined for project " + project)
		return nil, nil
	}

	customQueries := make(map[string]string)
	err = yaml.Unmarshal([]byte(configMap.Data["custom-queries"]), &customQueries)

	if err != nil {
		logger.Info("Project custom queries found, but could not parse them: " + err.Error())
		return nil, err
	}
	logger.Info("Project custom queries found and parsed")
	return customQueries, nil
}

func getPrometheusApiURL(project string, kubeClient v1.CoreV1Interface, logger *keptnutils.Logger) (string, error) {
	logger.Info("Checking if external prometheus instance has been defined for project " + project)
	// check if secret 'prometheus-credentials-<project> exists

	secret, err := kubeClient.Secrets("keptn").Get("prometheus-credentials-"+project, metav1.GetOptions{})

	// return cluster-internal prometheus URL if no secret has been found
	if err != nil {
		logger.Info("No external prometheus instance defined for project " + project + ". Using default: http://prometheus-service.monitoring.svc.cluster.local:8080")
		return "http://prometheus-service.monitoring.svc.cluster.local:8080", nil
	}

	/*
		data format:
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

func sendInternalGetSLIDoneEvent(shkeptncontext string, project string,
	service string, stage string, indicatorValues []*keptnevents.SLIResult, start string, end string, testStrategy string) error {

	source, _ := url.Parse("prometheus-sli-service")
	contentType := "application/json"

	getSLIEvent := keptnevents.InternalGetSLIDoneEventData{
		Project:         project,
		Service:         service,
		Stage:           stage,
		IndicatorValues: indicatorValues,
		Start:           start,
		End:             end,
		TestStrategy:    testStrategy,
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
