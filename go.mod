module github.com/keptn-contrib/prometheus-sli-service

go 1.12

require (
	github.com/cloudevents/sdk-go v0.10.0
	github.com/google/uuid v1.1.1
	github.com/kelseyhightower/envconfig v1.4.0
	github.com/keptn/go-utils v0.0.0-20191217074231-08f0c05f8de6
	github.com/stretchr/testify v1.4.0
	golang.org/x/net v0.0.0-20191021144547-ec77196f6094
	gopkg.in/yaml.v2 v2.2.4
	k8s.io/apimachinery v0.0.0-20190313205120-d7deff9243b1
	k8s.io/client-go v11.0.0+incompatible
)

replace github.com/cloudevents/sdk-go => github.com/cloudevents/sdk-go v0.0.0-20190509003705-56931988abe3
