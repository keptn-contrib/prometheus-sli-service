# Release Notes 0.1.0

This service is used for retrieving metrics from a prometheus API endpoint. Per default, it fetches metrics from the prometheus instance set up by Keptn
(`prometheus-service.monitoring.svc.cluster.local:8080`), but it can also be configures to use any reachable Prometheus endpoint using basic authentication by providing the credentials
via a secret in the `keptn` namespace of the cluster.

The supported metrics are:

 - throughput
 - error_rate
 - request_latency_p50
 - request_latency_p90
 - request_latency_p95

## New Features
- 

## Fixed Issues
- 

## Known Limitations
