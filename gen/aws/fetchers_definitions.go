/*
Copyright 2017 WALLIX

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Package aws holds the declarations the code generator reads. Editing a
// declaration here and running `make generate` is the only supported way to
// change the generated fetchers, services and property constants.
package aws

import (
	"strings"
	"unicode"

	"github.com/theazz/awless-ro/cloud"
)

// apiPackages maps the short API key used throughout awless-ro to the Go
// package name of the corresponding AWS SDK v2 module. Only the two load
// balancing services differ; every other key matches its package.
var apiPackages = map[string]string{
	"elb":   "elasticloadbalancing",
	"elbv2": "elasticloadbalancingv2",
}

// ApiPackage returns the SDK v2 package name for an API key.
func ApiPackage(api string) string {
	if pkg, ok := apiPackages[api]; ok {
		return pkg
	}
	return api
}

// ApiImportPath returns the import path of the SDK v2 service module.
func ApiImportPath(api string) string {
	return "github.com/aws/aws-sdk-go-v2/service/" + ApiPackage(api)
}

// ApiTypesAlias returns the import alias used for the service's types package.
// SDK v2 splits operation inputs and outputs (service package) from the shapes
// they carry (types package), and every generated file needs both.
func ApiTypesAlias(api string) string {
	return api + "types"
}

// ApiField returns the field name the API occupies in awsfetch.AWSAPI and in the
// generated service structs.
func ApiField(api string) string {
	return Title(api)
}

// ApiInterface returns the name of the narrow interface generated for the API.
// SDK v2 ships no equivalent of v1's <service>iface packages, so awless-ro
// declares the operation sets it uses itself; see the spec, decision D2.
func ApiInterface(api string) string {
	return Title(api) + "API"
}

// Title upper-cases the first rune. strings.Title is deprecated and word-splits,
// which is not what any of the generator templates want.
func Title(s string) string {
	if s == "" {
		return s
	}
	r := []rune(s)
	r[0] = unicode.ToUpper(r[0])
	return string(r)
}

// UniqueApis returns every API key that appears in the definitions, sorted, so
// that generated files have a stable order.
func UniqueApis() []string {
	seen := make(map[string]bool)
	var apis []string
	for _, def := range FetchersDefs {
		for _, api := range def.Api {
			if !seen[api] {
				seen[api] = true
				apis = append(apis, api)
			}
		}
	}
	sortStrings(apis)
	return apis
}

// GeneratedFetcherApis returns the API keys that have at least one generated
// Fetcher. Services reached only through hand-written fetchers, or not fetched
// at all, must stay out of the generated Fetcher file's imports.
func GeneratedFetcherApis() []string {
	seen := make(map[string]bool)
	var apis []string
	for _, def := range FetchersDefs {
		for _, f := range def.Fetchers {
			if f.ManualFetcher || f.ApiMethod == "" || seen[f.Api] {
				continue
			}
			seen[f.Api] = true
			apis = append(apis, f.Api)
		}
	}
	sortStrings(apis)
	return apis
}

// FetcherTypeApis returns the API keys whose types package is referenced by any
// Fetcher, hand-written ones included, since the generated services cast every
// fetched object list back to its concrete type.
func FetcherTypeApis() []string {
	seen := make(map[string]bool)
	var apis []string
	for _, def := range FetchersDefs {
		for _, f := range def.Fetchers {
			if seen[f.Api] || !strings.Contains(f.AWSType, ".") {
				continue // e.g. the SQS queue Fetcher, whose objects are plain strings
			}
			seen[f.Api] = true
			apis = append(apis, f.Api)
		}
	}
	sortStrings(apis)
	return apis
}

// GeneratedApiMethods returns the operations the generated fetchers call for the
// given API, sorted and de-duplicated.
func GeneratedApiMethods(api string) []string {
	seen := make(map[string]bool)
	var methods []string
	for _, def := range FetchersDefs {
		for _, f := range def.Fetchers {
			if f.Api != api || f.ManualFetcher || f.ApiMethod == "" {
				continue
			}
			if !seen[f.ApiMethod] {
				seen[f.ApiMethod] = true
				methods = append(methods, f.ApiMethod)
			}
		}
	}
	sortStrings(methods)
	return methods
}

func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && strings.Compare(s[j-1], s[j]) > 0; j-- {
			s[j-1], s[j] = s[j], s[j-1]
		}
	}
}

type FetchersDef struct {
	Name     string
	Global   bool
	Api      []string
	Fetchers []Fetcher
}

type Fetcher struct {
	// ResourceType is the awless-ro resource type, e.g. cloud.Instance.
	ResourceType string

	// AWSType is the SDK shape the API returns for this resource, written with
	// the types-package alias, e.g. "ec2types.Instance".
	AWSType string

	// ApiMethod is the SDK v2 operation name, without any suffix. The generator
	// derives the input type, the output type and, when Paginated is set, the
	// paginator constructor from it.
	ApiMethod string

	// InputFields is the literal body of the operation input, empty for most
	// operations.
	InputFields string

	// OutputsExtractor is the field path on the output holding the resources.
	// OutputsContainers, when set, is an intermediate list to walk first.
	OutputsContainers string
	OutputsExtractor  string

	// OutputsContainerType and OutputsWrapperType name the shapes the test mock
	// has to rebuild in order to hand back an output of the right form: the
	// element type of OutputsContainers, and the type behind a dotted
	// OutputsExtractor. Only the two fetchers with a nested output need them.
	OutputsContainerType string
	OutputsWrapperType   string

	// Paginated marks operations that page. It is set wherever the SDK offers a
	// paginator: the three EC2 operations that lack one do not page in the API
	// either. See the spec, decision D1b - ten of these used to fetch a single
	// page only.
	Paginated bool

	// ManualFetcher excludes the resource from generation; the Fetcher lives in
	// aws/fetch/manual_fetchers.go instead.
	ManualFetcher bool

	Api string
}

var FetchersDefs = []FetchersDef{
	{
		Name: "infra",
		Api:  []string{"ec2", "elbv2", "elb", "rds", "autoscaling", "ecr", "ecs", "acm"},
		Fetchers: []Fetcher{
			{Api: "ec2", ResourceType: cloud.Instance, AWSType: "ec2types.Instance", ApiMethod: "DescribeInstances", OutputsExtractor: "Instances", OutputsContainers: "Reservations", OutputsContainerType: "ec2types.Reservation", Paginated: true},
			{Api: "ec2", ResourceType: cloud.Subnet, AWSType: "ec2types.Subnet", ApiMethod: "DescribeSubnets", OutputsExtractor: "Subnets", Paginated: true},
			{Api: "ec2", ResourceType: cloud.Vpc, AWSType: "ec2types.Vpc", ApiMethod: "DescribeVpcs", OutputsExtractor: "Vpcs", Paginated: true},
			// DescribeKeyPairs, DescribeAvailabilityZones and DescribeAddresses
			// have no paginator because the API returns everything at once.
			{Api: "ec2", ResourceType: cloud.Keypair, AWSType: "ec2types.KeyPairInfo", ApiMethod: "DescribeKeyPairs", OutputsExtractor: "KeyPairs"},
			{Api: "ec2", ResourceType: cloud.SecurityGroup, AWSType: "ec2types.SecurityGroup", ApiMethod: "DescribeSecurityGroups", OutputsExtractor: "SecurityGroups", Paginated: true},
			{Api: "ec2", ResourceType: cloud.Volume, AWSType: "ec2types.Volume", ApiMethod: "DescribeVolumes", OutputsExtractor: "Volumes", Paginated: true},
			{Api: "ec2", ResourceType: cloud.InternetGateway, AWSType: "ec2types.InternetGateway", ApiMethod: "DescribeInternetGateways", OutputsExtractor: "InternetGateways", Paginated: true},
			{Api: "ec2", ResourceType: cloud.NatGateway, AWSType: "ec2types.NatGateway", ApiMethod: "DescribeNatGateways", OutputsExtractor: "NatGateways", Paginated: true},
			{Api: "ec2", ResourceType: cloud.RouteTable, AWSType: "ec2types.RouteTable", ApiMethod: "DescribeRouteTables", OutputsExtractor: "RouteTables", Paginated: true},
			{Api: "ec2", ResourceType: cloud.AvailabilityZone, AWSType: "ec2types.AvailabilityZone", ApiMethod: "DescribeAvailabilityZones", OutputsExtractor: "AvailabilityZones"},
			{Api: "ec2", ResourceType: cloud.Image, AWSType: "ec2types.Image", ApiMethod: "DescribeImages", InputFields: `Owners: []string{"self"}`, OutputsExtractor: "Images", Paginated: true},
			{Api: "ec2", ResourceType: cloud.ImportImageTask, AWSType: "ec2types.ImportImageTask", ApiMethod: "DescribeImportImageTasks", OutputsExtractor: "ImportImageTasks", Paginated: true},
			{Api: "ec2", ResourceType: cloud.ElasticIP, AWSType: "ec2types.Address", ApiMethod: "DescribeAddresses", OutputsExtractor: "Addresses"},
			{Api: "ec2", ResourceType: cloud.Snapshot, AWSType: "ec2types.Snapshot", ApiMethod: "DescribeSnapshots", InputFields: `OwnerIds: []string{"self"}`, OutputsExtractor: "Snapshots", Paginated: true},
			{Api: "ec2", ResourceType: cloud.NetworkInterface, AWSType: "ec2types.NetworkInterface", ApiMethod: "DescribeNetworkInterfaces", OutputsExtractor: "NetworkInterfaces", Paginated: true},
			{Api: "elb", ResourceType: cloud.ClassicLoadBalancer, AWSType: "elbtypes.LoadBalancerDescription", ApiMethod: "DescribeLoadBalancers", OutputsExtractor: "LoadBalancerDescriptions", Paginated: true},
			{Api: "elbv2", ResourceType: cloud.LoadBalancer, AWSType: "elbv2types.LoadBalancer", ApiMethod: "DescribeLoadBalancers", OutputsExtractor: "LoadBalancers", Paginated: true},
			{Api: "elbv2", ResourceType: cloud.TargetGroup, AWSType: "elbv2types.TargetGroup", ApiMethod: "DescribeTargetGroups", OutputsExtractor: "TargetGroups", Paginated: true},
			{Api: "elbv2", ResourceType: cloud.Listener, AWSType: "elbv2types.Listener", ManualFetcher: true},
			{Api: "rds", ResourceType: cloud.Database, AWSType: "rdstypes.DBInstance", ApiMethod: "DescribeDBInstances", OutputsExtractor: "DBInstances", Paginated: true},
			{Api: "rds", ResourceType: cloud.DbSubnetGroup, AWSType: "rdstypes.DBSubnetGroup", ApiMethod: "DescribeDBSubnetGroups", OutputsExtractor: "DBSubnetGroups", Paginated: true},
			{Api: "autoscaling", ResourceType: cloud.LaunchConfiguration, AWSType: "autoscalingtypes.LaunchConfiguration", ApiMethod: "DescribeLaunchConfigurations", OutputsExtractor: "LaunchConfigurations", Paginated: true},
			{Api: "autoscaling", ResourceType: cloud.ScalingGroup, AWSType: "autoscalingtypes.AutoScalingGroup", ApiMethod: "DescribeAutoScalingGroups", OutputsExtractor: "AutoScalingGroups", Paginated: true},
			{Api: "autoscaling", ResourceType: cloud.ScalingPolicy, AWSType: "autoscalingtypes.ScalingPolicy", ApiMethod: "DescribePolicies", OutputsExtractor: "ScalingPolicies", Paginated: true},
			{Api: "ecr", ResourceType: cloud.Repository, AWSType: "ecrtypes.Repository", ApiMethod: "DescribeRepositories", OutputsExtractor: "Repositories", Paginated: true},
			{Api: "ecs", ResourceType: cloud.ContainerCluster, AWSType: "ecstypes.Cluster", ManualFetcher: true},
			{Api: "ecs", ResourceType: cloud.ContainerTask, AWSType: "ecstypes.TaskDefinition", ManualFetcher: true},
			{Api: "ecs", ResourceType: cloud.Container, AWSType: "ecstypes.Container", ManualFetcher: true},
			{Api: "ecs", ResourceType: cloud.ContainerInstance, AWSType: "ecstypes.ContainerInstance", ManualFetcher: true},
			{Api: "acm", ResourceType: cloud.Certificate, AWSType: "acmtypes.CertificateSummary", ApiMethod: "ListCertificates", OutputsExtractor: "CertificateSummaryList", Paginated: true},
		},
	},
	{
		Name:   "access",
		Global: true,
		Api:    []string{"iam", "sts"},
		Fetchers: []Fetcher{
			{Api: "iam", ResourceType: cloud.User, AWSType: "iamtypes.UserDetail", ManualFetcher: true},
			{Api: "iam", ResourceType: cloud.Group, AWSType: "iamtypes.GroupDetail", ManualFetcher: true},
			{Api: "iam", ResourceType: cloud.Role, AWSType: "iamtypes.RoleDetail", ManualFetcher: true},
			{Api: "iam", ResourceType: cloud.Policy, AWSType: "iamtypes.Policy", ManualFetcher: true},
			{Api: "iam", ResourceType: cloud.AccessKey, AWSType: "iamtypes.AccessKeyMetadata", ManualFetcher: true},
			{Api: "iam", ResourceType: cloud.InstanceProfile, AWSType: "iamtypes.InstanceProfile", ApiMethod: "ListInstanceProfiles", OutputsExtractor: "InstanceProfiles", Paginated: true},
			{Api: "iam", ResourceType: cloud.MFADevice, AWSType: "iamtypes.VirtualMFADevice", ApiMethod: "ListVirtualMFADevices", OutputsExtractor: "VirtualMFADevices", Paginated: true},
		},
	},
	{
		Name: "storage",
		Api:  []string{"s3"},
		Fetchers: []Fetcher{
			{Api: "s3", ResourceType: cloud.Bucket, AWSType: "s3types.Bucket", ManualFetcher: true},
			{Api: "s3", ResourceType: cloud.S3Object, AWSType: "s3types.Object", ManualFetcher: true},
		},
	},
	{
		Name: "messaging",
		Api:  []string{"sns", "sqs"},
		Fetchers: []Fetcher{
			{Api: "sns", ResourceType: cloud.Subscription, AWSType: "snstypes.Subscription", ApiMethod: "ListSubscriptions", OutputsExtractor: "Subscriptions", Paginated: true},
			{Api: "sns", ResourceType: cloud.Topic, AWSType: "snstypes.Topic", ApiMethod: "ListTopics", OutputsExtractor: "Topics", Paginated: true},
			{Api: "sqs", ResourceType: cloud.Queue, AWSType: "string", ManualFetcher: true},
		},
	},
	{
		Name:   "dns",
		Global: true,
		Api:    []string{"route53"},
		Fetchers: []Fetcher{
			{Api: "route53", ResourceType: cloud.Zone, AWSType: "route53types.HostedZone", ApiMethod: "ListHostedZones", OutputsExtractor: "HostedZones", Paginated: true},
			{Api: "route53", ResourceType: cloud.Record, AWSType: "route53types.ResourceRecordSet", ManualFetcher: true},
		},
	},
	{
		Name: "lambda",
		Api:  []string{"lambda"},
		Fetchers: []Fetcher{
			{Api: "lambda", ResourceType: cloud.Function, AWSType: "lambdatypes.FunctionConfiguration", ApiMethod: "ListFunctions", OutputsExtractor: "Functions", Paginated: true},
		},
	},
	{
		Name: "monitoring",
		Api:  []string{"cloudwatch"},
		Fetchers: []Fetcher{
			{Api: "cloudwatch", ResourceType: cloud.Metric, AWSType: "cloudwatchtypes.Metric", ApiMethod: "ListMetrics", OutputsExtractor: "Metrics", Paginated: true},
			{Api: "cloudwatch", ResourceType: cloud.Alarm, AWSType: "cloudwatchtypes.MetricAlarm", ApiMethod: "DescribeAlarms", OutputsExtractor: "MetricAlarms", Paginated: true},
		},
	},
	{
		Name:   "cdn",
		Global: true,
		Api:    []string{"cloudfront"},
		Fetchers: []Fetcher{
			{Api: "cloudfront", ResourceType: cloud.Distribution, AWSType: "cloudfronttypes.DistributionSummary", ApiMethod: "ListDistributions", OutputsExtractor: "DistributionList.Items", OutputsWrapperType: "cloudfronttypes.DistributionList", Paginated: true},
		},
	},
	{
		Name: "cloudformation",
		Api:  []string{"cloudformation"},
		Fetchers: []Fetcher{
			{Api: "cloudformation", ResourceType: cloud.Stack, AWSType: "cloudformationtypes.Stack", ApiMethod: "DescribeStacks", OutputsExtractor: "Stacks", Paginated: true},
		},
	},
}
