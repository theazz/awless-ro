// Auto generated narrow interfaces over the AWS SDK

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

package awsfetch

// DO NOT EDIT - This file was automatically generated with go generate

// AWS SDK v2 ships no equivalent of v1's <service>iface packages, so awless-ro
// declares the operation sets it uses itself. Each service gets one interface
// composed of two halves: the operations the generated fetchers call, declared
// here, and the ones only the hand-written fetchers call, declared in
// manual_apis.go. Narrow interfaces are also what the SDK's own paginator
// constructors accept, so the generated fetchers can pass them straight through.
//
// The compile-time assertions at the bottom are what keep the hand-written half
// honest: if manual_apis.go names an operation the real client does not have, or
// gets a signature wrong, the build fails here rather than at the call site.

import (
	"context"
	"github.com/aws/aws-sdk-go-v2/service/acm"
	"github.com/aws/aws-sdk-go-v2/service/autoscaling"
	"github.com/aws/aws-sdk-go-v2/service/cloudformation"
	"github.com/aws/aws-sdk-go-v2/service/cloudfront"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ecr"
	"github.com/aws/aws-sdk-go-v2/service/ecs"
	"github.com/aws/aws-sdk-go-v2/service/elasticloadbalancing"
	"github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	"github.com/aws/aws-sdk-go-v2/service/lambda"
	"github.com/aws/aws-sdk-go-v2/service/rds"
	"github.com/aws/aws-sdk-go-v2/service/route53"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/sns"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/aws/aws-sdk-go-v2/service/sts"
)

// AcmAPI is every acm operation awless-ro calls.
type AcmAPI interface {
	generatedAcmAPI
	manualAcmAPI
}

type generatedAcmAPI interface {
	ListCertificates(context.Context, *acm.ListCertificatesInput, ...func(*acm.Options)) (*acm.ListCertificatesOutput, error)
}

// AutoscalingAPI is every autoscaling operation awless-ro calls.
type AutoscalingAPI interface {
	generatedAutoscalingAPI
	manualAutoscalingAPI
}

type generatedAutoscalingAPI interface {
	DescribeAutoScalingGroups(context.Context, *autoscaling.DescribeAutoScalingGroupsInput, ...func(*autoscaling.Options)) (*autoscaling.DescribeAutoScalingGroupsOutput, error)
	DescribeLaunchConfigurations(context.Context, *autoscaling.DescribeLaunchConfigurationsInput, ...func(*autoscaling.Options)) (*autoscaling.DescribeLaunchConfigurationsOutput, error)
	DescribePolicies(context.Context, *autoscaling.DescribePoliciesInput, ...func(*autoscaling.Options)) (*autoscaling.DescribePoliciesOutput, error)
}

// CloudformationAPI is every cloudformation operation awless-ro calls.
type CloudformationAPI interface {
	generatedCloudformationAPI
	manualCloudformationAPI
}

type generatedCloudformationAPI interface {
	DescribeStacks(context.Context, *cloudformation.DescribeStacksInput, ...func(*cloudformation.Options)) (*cloudformation.DescribeStacksOutput, error)
}

// CloudfrontAPI is every cloudfront operation awless-ro calls.
type CloudfrontAPI interface {
	generatedCloudfrontAPI
	manualCloudfrontAPI
}

type generatedCloudfrontAPI interface {
	ListDistributions(context.Context, *cloudfront.ListDistributionsInput, ...func(*cloudfront.Options)) (*cloudfront.ListDistributionsOutput, error)
}

// CloudwatchAPI is every cloudwatch operation awless-ro calls.
type CloudwatchAPI interface {
	generatedCloudwatchAPI
	manualCloudwatchAPI
}

type generatedCloudwatchAPI interface {
	DescribeAlarms(context.Context, *cloudwatch.DescribeAlarmsInput, ...func(*cloudwatch.Options)) (*cloudwatch.DescribeAlarmsOutput, error)
	ListMetrics(context.Context, *cloudwatch.ListMetricsInput, ...func(*cloudwatch.Options)) (*cloudwatch.ListMetricsOutput, error)
}

// Ec2API is every ec2 operation awless-ro calls.
type Ec2API interface {
	generatedEc2API
	manualEc2API
}

type generatedEc2API interface {
	DescribeAddresses(context.Context, *ec2.DescribeAddressesInput, ...func(*ec2.Options)) (*ec2.DescribeAddressesOutput, error)
	DescribeAvailabilityZones(context.Context, *ec2.DescribeAvailabilityZonesInput, ...func(*ec2.Options)) (*ec2.DescribeAvailabilityZonesOutput, error)
	DescribeImages(context.Context, *ec2.DescribeImagesInput, ...func(*ec2.Options)) (*ec2.DescribeImagesOutput, error)
	DescribeImportImageTasks(context.Context, *ec2.DescribeImportImageTasksInput, ...func(*ec2.Options)) (*ec2.DescribeImportImageTasksOutput, error)
	DescribeInstances(context.Context, *ec2.DescribeInstancesInput, ...func(*ec2.Options)) (*ec2.DescribeInstancesOutput, error)
	DescribeInternetGateways(context.Context, *ec2.DescribeInternetGatewaysInput, ...func(*ec2.Options)) (*ec2.DescribeInternetGatewaysOutput, error)
	DescribeKeyPairs(context.Context, *ec2.DescribeKeyPairsInput, ...func(*ec2.Options)) (*ec2.DescribeKeyPairsOutput, error)
	DescribeNatGateways(context.Context, *ec2.DescribeNatGatewaysInput, ...func(*ec2.Options)) (*ec2.DescribeNatGatewaysOutput, error)
	DescribeNetworkInterfaces(context.Context, *ec2.DescribeNetworkInterfacesInput, ...func(*ec2.Options)) (*ec2.DescribeNetworkInterfacesOutput, error)
	DescribeRouteTables(context.Context, *ec2.DescribeRouteTablesInput, ...func(*ec2.Options)) (*ec2.DescribeRouteTablesOutput, error)
	DescribeSecurityGroups(context.Context, *ec2.DescribeSecurityGroupsInput, ...func(*ec2.Options)) (*ec2.DescribeSecurityGroupsOutput, error)
	DescribeSnapshots(context.Context, *ec2.DescribeSnapshotsInput, ...func(*ec2.Options)) (*ec2.DescribeSnapshotsOutput, error)
	DescribeSubnets(context.Context, *ec2.DescribeSubnetsInput, ...func(*ec2.Options)) (*ec2.DescribeSubnetsOutput, error)
	DescribeVolumes(context.Context, *ec2.DescribeVolumesInput, ...func(*ec2.Options)) (*ec2.DescribeVolumesOutput, error)
	DescribeVpcs(context.Context, *ec2.DescribeVpcsInput, ...func(*ec2.Options)) (*ec2.DescribeVpcsOutput, error)
}

// EcrAPI is every ecr operation awless-ro calls.
type EcrAPI interface {
	generatedEcrAPI
	manualEcrAPI
}

type generatedEcrAPI interface {
	DescribeRepositories(context.Context, *ecr.DescribeRepositoriesInput, ...func(*ecr.Options)) (*ecr.DescribeRepositoriesOutput, error)
}

// EcsAPI is every ecs operation awless-ro calls.
type EcsAPI interface {
	generatedEcsAPI
	manualEcsAPI
}

type generatedEcsAPI interface {
	// no generated fetcher calls ecs directly
}

// ElbAPI is every elasticloadbalancing operation awless-ro calls.
type ElbAPI interface {
	generatedElbAPI
	manualElbAPI
}

type generatedElbAPI interface {
	DescribeLoadBalancers(context.Context, *elasticloadbalancing.DescribeLoadBalancersInput, ...func(*elasticloadbalancing.Options)) (*elasticloadbalancing.DescribeLoadBalancersOutput, error)
}

// Elbv2API is every elasticloadbalancingv2 operation awless-ro calls.
type Elbv2API interface {
	generatedElbv2API
	manualElbv2API
}

type generatedElbv2API interface {
	DescribeLoadBalancers(context.Context, *elasticloadbalancingv2.DescribeLoadBalancersInput, ...func(*elasticloadbalancingv2.Options)) (*elasticloadbalancingv2.DescribeLoadBalancersOutput, error)
	DescribeTargetGroups(context.Context, *elasticloadbalancingv2.DescribeTargetGroupsInput, ...func(*elasticloadbalancingv2.Options)) (*elasticloadbalancingv2.DescribeTargetGroupsOutput, error)
}

// IamAPI is every iam operation awless-ro calls.
type IamAPI interface {
	generatedIamAPI
	manualIamAPI
}

type generatedIamAPI interface {
	ListInstanceProfiles(context.Context, *iam.ListInstanceProfilesInput, ...func(*iam.Options)) (*iam.ListInstanceProfilesOutput, error)
	ListVirtualMFADevices(context.Context, *iam.ListVirtualMFADevicesInput, ...func(*iam.Options)) (*iam.ListVirtualMFADevicesOutput, error)
}

// LambdaAPI is every lambda operation awless-ro calls.
type LambdaAPI interface {
	generatedLambdaAPI
	manualLambdaAPI
}

type generatedLambdaAPI interface {
	ListFunctions(context.Context, *lambda.ListFunctionsInput, ...func(*lambda.Options)) (*lambda.ListFunctionsOutput, error)
}

// RdsAPI is every rds operation awless-ro calls.
type RdsAPI interface {
	generatedRdsAPI
	manualRdsAPI
}

type generatedRdsAPI interface {
	DescribeDBInstances(context.Context, *rds.DescribeDBInstancesInput, ...func(*rds.Options)) (*rds.DescribeDBInstancesOutput, error)
	DescribeDBSubnetGroups(context.Context, *rds.DescribeDBSubnetGroupsInput, ...func(*rds.Options)) (*rds.DescribeDBSubnetGroupsOutput, error)
}

// Route53API is every route53 operation awless-ro calls.
type Route53API interface {
	generatedRoute53API
	manualRoute53API
}

type generatedRoute53API interface {
	ListHostedZones(context.Context, *route53.ListHostedZonesInput, ...func(*route53.Options)) (*route53.ListHostedZonesOutput, error)
}

// S3API is every s3 operation awless-ro calls.
type S3API interface {
	generatedS3API
	manualS3API
}

type generatedS3API interface {
	// no generated fetcher calls s3 directly
}

// SnsAPI is every sns operation awless-ro calls.
type SnsAPI interface {
	generatedSnsAPI
	manualSnsAPI
}

type generatedSnsAPI interface {
	ListSubscriptions(context.Context, *sns.ListSubscriptionsInput, ...func(*sns.Options)) (*sns.ListSubscriptionsOutput, error)
	ListTopics(context.Context, *sns.ListTopicsInput, ...func(*sns.Options)) (*sns.ListTopicsOutput, error)
}

// SqsAPI is every sqs operation awless-ro calls.
type SqsAPI interface {
	generatedSqsAPI
	manualSqsAPI
}

type generatedSqsAPI interface {
	// no generated fetcher calls sqs directly
}

// StsAPI is every sts operation awless-ro calls.
type StsAPI interface {
	generatedStsAPI
	manualStsAPI
}

type generatedStsAPI interface {
	// no generated fetcher calls sts directly
}

var (
	_ AcmAPI            = (*acm.Client)(nil)
	_ AutoscalingAPI    = (*autoscaling.Client)(nil)
	_ CloudformationAPI = (*cloudformation.Client)(nil)
	_ CloudfrontAPI     = (*cloudfront.Client)(nil)
	_ CloudwatchAPI     = (*cloudwatch.Client)(nil)
	_ Ec2API            = (*ec2.Client)(nil)
	_ EcrAPI            = (*ecr.Client)(nil)
	_ EcsAPI            = (*ecs.Client)(nil)
	_ ElbAPI            = (*elasticloadbalancing.Client)(nil)
	_ Elbv2API          = (*elasticloadbalancingv2.Client)(nil)
	_ IamAPI            = (*iam.Client)(nil)
	_ LambdaAPI         = (*lambda.Client)(nil)
	_ RdsAPI            = (*rds.Client)(nil)
	_ Route53API        = (*route53.Client)(nil)
	_ S3API             = (*s3.Client)(nil)
	_ SnsAPI            = (*sns.Client)(nil)
	_ SqsAPI            = (*sqs.Client)(nil)
	_ StsAPI            = (*sts.Client)(nil)
)
