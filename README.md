# Prometheus SLI Service

This service is used for retrieving service level indicators (SLIs) from a prometheus API endpoint. Per default, it fetches metrics from the prometheus instance set up by Keptn
(`prometheus-service.monitoring.svc.cluster.local:8080`), but it can also be configures to use any reachable Prometheus endpoint using basic authentication by providing the credentials
via a secret in the `keptn` namespace of the cluster.

The supported SLIs are:

 - throughput
 - error_rate
 - request_latency_p50
 - request_latency_p90
 - request_latency_p95
 
The provided SLIs are based on the [RED metrics](https://grafana.com/files/grafanacon_eu_2018/Tom_Wilkie_GrafanaCon_EU_2018.pdf)

## Usage 

Per default, the service works with the following assumptions regarding the setup of the Prometheus instance:

 - Each **service** within a **stage** of a **project** has a Prometheus scrape job definition with the name: `<service>-<project>-<stage>`

    For example, if `project=sockshop`, `stage=production` and `service=carts`, the scrape job name would have to be `carts-sockshop-production`.
    
 - Every service provides the following Metrics for its corresponding scrape job:
     - http_response_time_milliseconds (Histogram)
     - http_requests_total (Counter)
     
       This metric has to contain the `status` label, indicating the HTTP response code of the requests handled by the service.
       It is highly recommended that this metric also provides a label to query metric values for specific endpoints, e.g. `handler`
       
       An example of an entry would look like this: `http_requests_total{method="GET",handler="VersionController.getInformation",status="200",} 4.0`
       
 - Based on those metrics, the queries for the SLIs are built as follows:
 
   - **throughput**: `sum(rate(http_requests_total{job="<service>-<project>-<stage>"}[<test_duration_in_seconds>s]))`
   - **error_rate**: `sum(rate(http_requests_total{job="<service>-<project>-<stage>",status!~'2..'}[<test_duration_in_seconds>s]))/sum(rate(http_requests_total{job="<service>-<project>-<stage>"}[<test_duration_in_seconds>s]))`
   - **request_latency_p50**: `histogram_quantile(0.50, sum(rate(http_response_time_milliseconds_bucket{job='<service>-<project>-<stage>'}[<test_duration_in_seconds>s])) by (le))`
   - **request_latency_p90**: `histogram_quantile(0.90, sum(rate(http_response_time_milliseconds_bucket{job='<service>-<project>-<stage>'}[<test_duration_in_seconds>s])) by (le))`
   - **request_latency_p95**: `histogram_quantile(0.95, sum(rate(http_response_time_milliseconds_bucket{job='<service>-<project>-<stage>'}[<test_duration_in_seconds>s])) by (le))` 
   
## Advanced Usage

### Using an external Prometheus instance
To use a Prometheus instance other than the one that's being managed by Keptn for a certain project, a secret containing the URL and the access credentials has to be deployed into the `keptn` namespace. The secret must have the following format:

```yaml
user: test
password: test
url: http://prometheus-service.monitoring.svc.cluster.local:8080
```

If this information is stored in a file, e.g. `prometheus-creds.yaml`, it can be stored with the following command (don't forget to replace the `<project>` placeholder with the name of your project:

```bash
kubectl create secret -n keptn generic prometheus-credentials-<project> --from-file=prometheus-credentials=./mock_secret.yaml
```

Please note that there is a naming convention for the secret, because this can be configured per **project**. Therefore, the secret has to have the name `prometheus-credentials-<project>`


### Overriding SLI queries



    
    

## Installation

The *prometheus-sli-service* is installed as a part of [Keptn's uniform](https://keptn.sh).

## Deploy in your Kubernetes cluster

To deploy the current version of the *prometheus-sli-service* in your Keptn Kubernetes cluster, use the file `deploy/service.yaml` from this repository and apply it:

```console
kubectl apply -f deploy/service.yaml
```

## Delete in your Kubernetes cluster

To delete a deployed *prometheus-sli-service*, use the file `deploy/service.yaml` from this repository and delete the Kubernetes resources:

```console
kubectl delete -f deploy/service.yaml
```
