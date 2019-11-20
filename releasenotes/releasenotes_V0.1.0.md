# Release Notes 0.1.0

This service is used for retrieving metrics from a prometheus API endpoint. Per default, it fetches metrics from the prometheus instance set up by Keptn
(`prometheus-service.monitoring.svc.cluster.local:8080`), but it can also be configures to use any reachable Prometheus endpoint using basic authentication by providing the credentials
via a secret in the `keptn` namespace of the cluster.

The supported default SLIs are:

 - throughput
 - error_rate
 - response_time_p50
 - response_time_p90
 - response_time_p95
 
The queries for those SLIs can be overridden by providing custom Prometheus queries. Similarly, it is also possible to add additional custom SLIs and their queries.
For detailed instructions, please head to the [README section](https://github.com/keptn-contrib/prometheus-sli-service/tree/0.1.0).

