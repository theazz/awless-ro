package awsfetch

import (
	"github.com/theazz/awless-ro/logger"
)

// AWSAPI holds one client per AWS service, typed by the narrow interfaces in
// gen_apis.go and manual_apis.go rather than by the concrete SDK clients, so
// that tests can substitute them.
type AWSAPI struct {
	Iam            IamAPI
	Ec2            Ec2API
	Elbv2          Elbv2API
	Elb            ElbAPI
	Rds            RdsAPI
	Autoscaling    AutoscalingAPI
	Ecr            EcrAPI
	Ecs            EcsAPI
	Sts            StsAPI
	S3             S3API
	Sns            SnsAPI
	Sqs            SqsAPI
	Route53        Route53API
	Lambda         LambdaAPI
	Cloudwatch     CloudwatchAPI
	Cloudfront     CloudfrontAPI
	Cloudformation CloudformationAPI
	Acm            AcmAPI
}

type Config struct {
	Log   *logger.Logger
	Extra map[string]interface{}
	APIs  *AWSAPI
}

// NewConfig takes the clients by name.
//
// Upstream passed them as an unordered bag and let reflection drop each one into
// the first field it was assignable to. That worked while the fields were typed
// by the SDK's own per-service interfaces, which never overlap. It does not work
// here: a service awless-ro calls no operations on would get an empty interface,
// which every client satisfies, so the first client offered would be filed under
// the wrong name and the right field left nil. Naming the fields removes the
// ambiguity instead of relying on no interface ever being empty.
func NewConfig(apis *AWSAPI) *Config {
	if apis == nil {
		apis = new(AWSAPI)
	}
	return &Config{
		Extra: make(map[string]interface{}),
		Log:   logger.DiscardLogger,
		APIs:  apis,
	}
}

func (c *Config) getBoolDefaultTrue(key string) bool {
	if c.Extra == nil {
		return true
	}

	if b, ok := c.Extra[key].(bool); ok {
		return b
	}

	return true
}
