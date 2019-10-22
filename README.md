# Prometheus SLI Service

The *prometheus-sli-service* is a Keptn service that is responsible for retrieving the values of Keptn-supported SLIs from a Prometheus API endpoint.

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
