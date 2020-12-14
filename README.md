# Prometheus SLI Service
![GitHub release (latest by date)](https://img.shields.io/github/v/release/keptn-contrib/prometheus-sli-service)
[![Build Status](https://travis-ci.org/keptn-contrib/prometheus-sli-service.svg?branch=master)](https://travis-ci.org/keptn-contrib/prometheus-sli-service)
[![Go Report Card](https://goreportcard.com/badge/github.com/keptn-contrib/prometheus-sli-service)](https://goreportcard.com/report/github.com/keptn-contrib/prometheus-sli-service)

This service is used for retrieving Service Level Indicators (SLIs) from a Prometheus API endpoint. Per default, it fetches metrics from the prometheus instance set up by Keptn
(`prometheus-service.monitoring.svc.cluster.local:8080`), but it can also be configured to use any reachable Prometheus endpoint using basic authentication by providing the credentials
via a secret in the `keptn` namespace of the cluster.

The supported default SLIs are:

 - throughput
 - error_rate
 - response_time_p50
 - response_time_p90
 - response_time_p95
 
The provided SLIs are based on the [RED metrics](https://grafana.com/files/grafanacon_eu_2018/Tom_Wilkie_GrafanaCon_EU_2018.pdf)

## Compatibility Matrix

Please always double check the version of Keptn you are using compared to the version of this service, and follow the compatibility matrix below.


| Keptn Version    | [Prometheus SLI Service Image](https://hub.docker.com/r/keptncontrib/prometheus-sli-service/tags) |
|:----------------:|:----------------------------------------:|
|       0.6.1      | keptn/prometheus-service:0.2.1  |
|       0.6.2      | keptn/prometheus-service:0.2.2  |
|       0.7.0      | keptn/prometheus-service:0.2.2  |
|       0.7.1      | keptn/prometheus-service:0.2.2  |
|       0.7.2      | keptn/prometheus-service:0.2.3  |
|       0.7.3      | keptn/prometheus-service:0.2.3  |

## Basic Usage 

Per default, the service works with the following assumptions regarding the setup of the Prometheus instance:

 - Each **service** within a **stage** of a **project** has a Prometheus scrape job definition with the name: `<service>-<project>-<stage>`

    For example, if `project=sockshop`, `stage=production` and `service=carts`, the scrape job name would have to be `carts-sockshop-production`.
    
 - Every service provides the following metrics for its corresponding scrape job:
     - http_response_time_milliseconds (Histogram)
     - http_requests_total (Counter)
     
       This metric has to contain the `status` label, indicating the HTTP response code of the requests handled by the service.
       It is highly recommended that this metric also provides a label to query metric values for specific endpoints, e.g. `handler`.
       
       An example of an entry would look like this: `http_requests_total{method="GET",handler="VersionController.getInformation",status="200",} 4.0`
       
 - Based on those metrics, the queries for the SLIs are built as follows:
 
   - **throughput**: `sum(rate(http_requests_total{job="<service>-<project>-<stage>-canary"}[<test_duration_in_seconds>s]))`
   - **error_rate**: `sum(rate(http_requests_total{job="<service>-<project>-<stage>-canary",status!~'2..'}[<test_duration_in_seconds>s]))/sum(rate(http_requests_total{job="<service>-<project>-<stage>-canary"}[<test_duration_in_seconds>s]))`
   - **response_time_p50**: `histogram_quantile(0.50, sum(rate(http_response_time_milliseconds_bucket{job='<service>-<project>-<stage>-canary'}[<test_duration_in_seconds>s])) by (le))`
   - **response_time_p90**: `histogram_quantile(0.90, sum(rate(http_response_time_milliseconds_bucket{job='<service>-<project>-<stage>-canary'}[<test_duration_in_seconds>s])) by (le))`
   - **response_time_p95**: `histogram_quantile(0.95, sum(rate(http_response_time_milliseconds_bucket{job='<service>-<project>-<stage>-canary'}[<test_duration_in_seconds>s])) by (le))` 
   
## Advanced Usage

### Using an external Prometheus instance
To use a Prometheus instance other than the one that is being managed by Keptn for a certain project, a secret containing the URL and the access credentials has to be deployed into the `keptn` namespace. The secret must have the following format:

```yaml
user: test
password: test
url: http://prometheus-service.monitoring.svc.cluster.local:8080
```

If this information is stored in a file, e.g. `prometheus-creds.yaml`, it can be stored with the following command (don't forget to replace the `<project>` placeholder with the name of your project:

```console
kubectl create secret -n keptn generic prometheus-credentials-<project> --from-file=prometheus-credentials=./mock_secret.yaml
```

Please note that there is a naming convention for the secret, because this can be configured per **project**. Therefore, the secret has to have the name `prometheus-credentials-<project>`

### Custom SLI queries

Users can override the predefined queries, as well as add custom queries by creating a SLI configuration. 

* A SLI configuration is a yaml file as shown below:

    ```yaml
    ---
    spec_version: '1.0'
    indicators:
      cpu_usage: avg(rate(container_cpu_usage_seconds_total{namespace="$PROJECT-$STAGE",pod_name=~"$SERVICE-primary-.*"}[5m]))
      response_time_p95: histogram_quantile(0.95, sum by(le) (rate(http_response_time_milliseconds_bucket{handler="ItemsController.addToCart",job="$SERVICE-$PROJECT-$STAGE-canary"}[$DURATION_SECONDS])))
    ```

* To store this configuration, you need to add this file to a Keptn's configuration store. This is done by using the Keptn CLI with the [add-resource](https://keptn.sh/docs/0.6.0/reference/cli/#keptn-add-resource) command. 

---

Within the user-defined queries, the following variables can be used to dynamically build the query, depending on the project/stage/service, and the time frame:

- $PROJECT: will be replaced with the name of the project
- $STAGE: will be replaced with the name of the stage
- $SERVICE: will be replaced with the name of the service
- $DURATION_SECONDS: will be replaced with the test run duration, e.g. 30s

For example, if an evaluation for the service **carts**  in the stage **production** of the project **sockshop** is triggered, and the tests ran for 30s these will be the resulting queries:

```
rate(my_custom_metric{job='$SERVICE-$PROJECT-$STAGE',handler=~'$handler'}[$DURATION_SECONDS]) => rate(my_custom_metric{job='carts-sockshop-production',handler=~'$handler'}[30s])
```

## Deploy in your Kubernetes cluster

To deploy the current version of the *prometheus-sli-service* in your Keptn Kubernetes cluster, use the file `deploy/service.yaml` from this repository and apply it:

```console
KEPTN_NAMESPACE=<your keptn namespace>
kubectl apply -f deploy/service.yaml -n $KEPTN_NAMESPACE
```

## Delete in your Kubernetes cluster

To delete a deployed *prometheus-sli-service*, use the file `deploy/service.yaml` from this repository and delete the Kubernetes resources:

```console
KEPTN_NAMESPACE=<your keptn namespace>
kubectl delete -f deploy/service.yaml -n $KEPTN_NAMESPACE
```
