// Auto generated test mocks for the AWS cloud service

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

package awsservices

// DO NOT EDIT - This file was automatically generated with go generate

// Each mock embeds the narrow interface of its service rather than implementing
// it in full. That is what lets a mock answer only the operations a test cares
// about; anything else is a nil interface call, which panics with the operation
// name instead of quietly returning a zero value.
//
// Mocks hand back all their objects in a single page. Verifying that the
// paginators actually accumulate pages is a separate, hand-written test: the
// continuation token is named differently by each service, so generating
// multi-page mocks would encode more SDK trivia than it is worth.
//
// Each mock also embeds a manual<Api>Mock struct declared in mocks_test.go. That
// is where the canned answers for the hand-written fetchers live, since Go does
// not let one file add fields to a type declared in another.

import (
	"context"
	"github.com/aws/aws-sdk-go-v2/service/acm"
	acmtypes "github.com/aws/aws-sdk-go-v2/service/acm/types"
	"github.com/aws/aws-sdk-go-v2/service/autoscaling"
	autoscalingtypes "github.com/aws/aws-sdk-go-v2/service/autoscaling/types"
	"github.com/aws/aws-sdk-go-v2/service/cloudformation"
	cloudformationtypes "github.com/aws/aws-sdk-go-v2/service/cloudformation/types"
	"github.com/aws/aws-sdk-go-v2/service/cloudfront"
	cloudfronttypes "github.com/aws/aws-sdk-go-v2/service/cloudfront/types"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	cloudwatchtypes "github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/aws-sdk-go-v2/service/ecr"
	ecrtypes "github.com/aws/aws-sdk-go-v2/service/ecr/types"
	"github.com/aws/aws-sdk-go-v2/service/elasticloadbalancing"
	elbtypes "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancing/types"
	"github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2"
	elbv2types "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2/types"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	iamtypes "github.com/aws/aws-sdk-go-v2/service/iam/types"
	"github.com/aws/aws-sdk-go-v2/service/lambda"
	lambdatypes "github.com/aws/aws-sdk-go-v2/service/lambda/types"
	"github.com/aws/aws-sdk-go-v2/service/rds"
	rdstypes "github.com/aws/aws-sdk-go-v2/service/rds/types"
	"github.com/aws/aws-sdk-go-v2/service/route53"
	route53types "github.com/aws/aws-sdk-go-v2/service/route53/types"
	"github.com/aws/aws-sdk-go-v2/service/sns"
	snstypes "github.com/aws/aws-sdk-go-v2/service/sns/types"

	"github.com/theazz/awless-ro/aws/fetch"
	"github.com/theazz/awless-ro/cloud"
)

type mockAcm struct {
	awsfetch.AcmAPI
	manualAcmMock
	certificatesummarys []acmtypes.CertificateSummary
}

// cloud.Service, so that a mock can stand in for a whole service in the
// registry. The graph-building tests drive the fetchers directly, so these are
// inert on purpose.
func (m *mockAcm) Name() string            { return "" }
func (m *mockAcm) Region() string          { return "" }
func (m *mockAcm) Profile() string         { return "" }
func (m *mockAcm) ResourceTypes() []string { return []string{} }
func (m *mockAcm) IsSyncDisabled() bool    { return false }

func (m *mockAcm) Fetch(context.Context) (cloud.GraphAPI, error) {
	return nil, nil
}

func (m *mockAcm) FetchByType(context.Context, string) (cloud.GraphAPI, error) {
	return nil, nil
}

func (m *mockAcm) ListCertificates(_ context.Context, _ *acm.ListCertificatesInput, _ ...func(*acm.Options)) (*acm.ListCertificatesOutput, error) {
	return &acm.ListCertificatesOutput{CertificateSummaryList: m.certificatesummarys}, nil
}

type mockAutoscaling struct {
	awsfetch.AutoscalingAPI
	manualAutoscalingMock
	launchconfigurations []autoscalingtypes.LaunchConfiguration
	autoscalinggroups    []autoscalingtypes.AutoScalingGroup
	scalingpolicys       []autoscalingtypes.ScalingPolicy
}

// cloud.Service, so that a mock can stand in for a whole service in the
// registry. The graph-building tests drive the fetchers directly, so these are
// inert on purpose.
func (m *mockAutoscaling) Name() string            { return "" }
func (m *mockAutoscaling) Region() string          { return "" }
func (m *mockAutoscaling) Profile() string         { return "" }
func (m *mockAutoscaling) ResourceTypes() []string { return []string{} }
func (m *mockAutoscaling) IsSyncDisabled() bool    { return false }

func (m *mockAutoscaling) Fetch(context.Context) (cloud.GraphAPI, error) {
	return nil, nil
}

func (m *mockAutoscaling) FetchByType(context.Context, string) (cloud.GraphAPI, error) {
	return nil, nil
}

func (m *mockAutoscaling) DescribeLaunchConfigurations(_ context.Context, _ *autoscaling.DescribeLaunchConfigurationsInput, _ ...func(*autoscaling.Options)) (*autoscaling.DescribeLaunchConfigurationsOutput, error) {
	return &autoscaling.DescribeLaunchConfigurationsOutput{LaunchConfigurations: m.launchconfigurations}, nil
}

func (m *mockAutoscaling) DescribeAutoScalingGroups(_ context.Context, _ *autoscaling.DescribeAutoScalingGroupsInput, _ ...func(*autoscaling.Options)) (*autoscaling.DescribeAutoScalingGroupsOutput, error) {
	return &autoscaling.DescribeAutoScalingGroupsOutput{AutoScalingGroups: m.autoscalinggroups}, nil
}

func (m *mockAutoscaling) DescribePolicies(_ context.Context, _ *autoscaling.DescribePoliciesInput, _ ...func(*autoscaling.Options)) (*autoscaling.DescribePoliciesOutput, error) {
	return &autoscaling.DescribePoliciesOutput{ScalingPolicies: m.scalingpolicys}, nil
}

type mockCloudformation struct {
	awsfetch.CloudformationAPI
	manualCloudformationMock
	stacks []cloudformationtypes.Stack
}

// cloud.Service, so that a mock can stand in for a whole service in the
// registry. The graph-building tests drive the fetchers directly, so these are
// inert on purpose.
func (m *mockCloudformation) Name() string            { return "" }
func (m *mockCloudformation) Region() string          { return "" }
func (m *mockCloudformation) Profile() string         { return "" }
func (m *mockCloudformation) ResourceTypes() []string { return []string{} }
func (m *mockCloudformation) IsSyncDisabled() bool    { return false }

func (m *mockCloudformation) Fetch(context.Context) (cloud.GraphAPI, error) {
	return nil, nil
}

func (m *mockCloudformation) FetchByType(context.Context, string) (cloud.GraphAPI, error) {
	return nil, nil
}

func (m *mockCloudformation) DescribeStacks(_ context.Context, _ *cloudformation.DescribeStacksInput, _ ...func(*cloudformation.Options)) (*cloudformation.DescribeStacksOutput, error) {
	return &cloudformation.DescribeStacksOutput{Stacks: m.stacks}, nil
}

type mockCloudfront struct {
	awsfetch.CloudfrontAPI
	manualCloudfrontMock
	distributionsummarys []cloudfronttypes.DistributionSummary
}

// cloud.Service, so that a mock can stand in for a whole service in the
// registry. The graph-building tests drive the fetchers directly, so these are
// inert on purpose.
func (m *mockCloudfront) Name() string            { return "" }
func (m *mockCloudfront) Region() string          { return "" }
func (m *mockCloudfront) Profile() string         { return "" }
func (m *mockCloudfront) ResourceTypes() []string { return []string{} }
func (m *mockCloudfront) IsSyncDisabled() bool    { return false }

func (m *mockCloudfront) Fetch(context.Context) (cloud.GraphAPI, error) {
	return nil, nil
}

func (m *mockCloudfront) FetchByType(context.Context, string) (cloud.GraphAPI, error) {
	return nil, nil
}

func (m *mockCloudfront) ListDistributions(_ context.Context, _ *cloudfront.ListDistributionsInput, _ ...func(*cloudfront.Options)) (*cloudfront.ListDistributionsOutput, error) {
	return &cloudfront.ListDistributionsOutput{DistributionList: &cloudfronttypes.DistributionList{Items: m.distributionsummarys}}, nil
}

type mockCloudwatch struct {
	awsfetch.CloudwatchAPI
	manualCloudwatchMock
	metrics      []cloudwatchtypes.Metric
	metricalarms []cloudwatchtypes.MetricAlarm
}

// cloud.Service, so that a mock can stand in for a whole service in the
// registry. The graph-building tests drive the fetchers directly, so these are
// inert on purpose.
func (m *mockCloudwatch) Name() string            { return "" }
func (m *mockCloudwatch) Region() string          { return "" }
func (m *mockCloudwatch) Profile() string         { return "" }
func (m *mockCloudwatch) ResourceTypes() []string { return []string{} }
func (m *mockCloudwatch) IsSyncDisabled() bool    { return false }

func (m *mockCloudwatch) Fetch(context.Context) (cloud.GraphAPI, error) {
	return nil, nil
}

func (m *mockCloudwatch) FetchByType(context.Context, string) (cloud.GraphAPI, error) {
	return nil, nil
}

func (m *mockCloudwatch) ListMetrics(_ context.Context, _ *cloudwatch.ListMetricsInput, _ ...func(*cloudwatch.Options)) (*cloudwatch.ListMetricsOutput, error) {
	return &cloudwatch.ListMetricsOutput{Metrics: m.metrics}, nil
}

func (m *mockCloudwatch) DescribeAlarms(_ context.Context, _ *cloudwatch.DescribeAlarmsInput, _ ...func(*cloudwatch.Options)) (*cloudwatch.DescribeAlarmsOutput, error) {
	return &cloudwatch.DescribeAlarmsOutput{MetricAlarms: m.metricalarms}, nil
}

type mockEc2 struct {
	awsfetch.Ec2API
	manualEc2Mock
	instances         []ec2types.Instance
	subnets           []ec2types.Subnet
	vpcs              []ec2types.Vpc
	keypairinfos      []ec2types.KeyPairInfo
	securitygroups    []ec2types.SecurityGroup
	volumes           []ec2types.Volume
	internetgateways  []ec2types.InternetGateway
	natgateways       []ec2types.NatGateway
	routetables       []ec2types.RouteTable
	availabilityzones []ec2types.AvailabilityZone
	images            []ec2types.Image
	importimagetasks  []ec2types.ImportImageTask
	addresss          []ec2types.Address
	snapshots         []ec2types.Snapshot
	networkinterfaces []ec2types.NetworkInterface
}

// cloud.Service, so that a mock can stand in for a whole service in the
// registry. The graph-building tests drive the fetchers directly, so these are
// inert on purpose.
func (m *mockEc2) Name() string            { return "" }
func (m *mockEc2) Region() string          { return "" }
func (m *mockEc2) Profile() string         { return "" }
func (m *mockEc2) ResourceTypes() []string { return []string{} }
func (m *mockEc2) IsSyncDisabled() bool    { return false }

func (m *mockEc2) Fetch(context.Context) (cloud.GraphAPI, error) {
	return nil, nil
}

func (m *mockEc2) FetchByType(context.Context, string) (cloud.GraphAPI, error) {
	return nil, nil
}

func (m *mockEc2) DescribeInstances(_ context.Context, _ *ec2.DescribeInstancesInput, _ ...func(*ec2.Options)) (*ec2.DescribeInstancesOutput, error) {
	return &ec2.DescribeInstancesOutput{Reservations: []ec2types.Reservation{{Instances: m.instances}}}, nil
}

func (m *mockEc2) DescribeSubnets(_ context.Context, _ *ec2.DescribeSubnetsInput, _ ...func(*ec2.Options)) (*ec2.DescribeSubnetsOutput, error) {
	return &ec2.DescribeSubnetsOutput{Subnets: m.subnets}, nil
}

func (m *mockEc2) DescribeVpcs(_ context.Context, _ *ec2.DescribeVpcsInput, _ ...func(*ec2.Options)) (*ec2.DescribeVpcsOutput, error) {
	return &ec2.DescribeVpcsOutput{Vpcs: m.vpcs}, nil
}

func (m *mockEc2) DescribeKeyPairs(_ context.Context, _ *ec2.DescribeKeyPairsInput, _ ...func(*ec2.Options)) (*ec2.DescribeKeyPairsOutput, error) {
	return &ec2.DescribeKeyPairsOutput{KeyPairs: m.keypairinfos}, nil
}

func (m *mockEc2) DescribeSecurityGroups(_ context.Context, _ *ec2.DescribeSecurityGroupsInput, _ ...func(*ec2.Options)) (*ec2.DescribeSecurityGroupsOutput, error) {
	return &ec2.DescribeSecurityGroupsOutput{SecurityGroups: m.securitygroups}, nil
}

func (m *mockEc2) DescribeVolumes(_ context.Context, _ *ec2.DescribeVolumesInput, _ ...func(*ec2.Options)) (*ec2.DescribeVolumesOutput, error) {
	return &ec2.DescribeVolumesOutput{Volumes: m.volumes}, nil
}

func (m *mockEc2) DescribeInternetGateways(_ context.Context, _ *ec2.DescribeInternetGatewaysInput, _ ...func(*ec2.Options)) (*ec2.DescribeInternetGatewaysOutput, error) {
	return &ec2.DescribeInternetGatewaysOutput{InternetGateways: m.internetgateways}, nil
}

func (m *mockEc2) DescribeNatGateways(_ context.Context, _ *ec2.DescribeNatGatewaysInput, _ ...func(*ec2.Options)) (*ec2.DescribeNatGatewaysOutput, error) {
	return &ec2.DescribeNatGatewaysOutput{NatGateways: m.natgateways}, nil
}

func (m *mockEc2) DescribeRouteTables(_ context.Context, _ *ec2.DescribeRouteTablesInput, _ ...func(*ec2.Options)) (*ec2.DescribeRouteTablesOutput, error) {
	return &ec2.DescribeRouteTablesOutput{RouteTables: m.routetables}, nil
}

func (m *mockEc2) DescribeAvailabilityZones(_ context.Context, _ *ec2.DescribeAvailabilityZonesInput, _ ...func(*ec2.Options)) (*ec2.DescribeAvailabilityZonesOutput, error) {
	return &ec2.DescribeAvailabilityZonesOutput{AvailabilityZones: m.availabilityzones}, nil
}

func (m *mockEc2) DescribeImages(_ context.Context, _ *ec2.DescribeImagesInput, _ ...func(*ec2.Options)) (*ec2.DescribeImagesOutput, error) {
	return &ec2.DescribeImagesOutput{Images: m.images}, nil
}

func (m *mockEc2) DescribeImportImageTasks(_ context.Context, _ *ec2.DescribeImportImageTasksInput, _ ...func(*ec2.Options)) (*ec2.DescribeImportImageTasksOutput, error) {
	return &ec2.DescribeImportImageTasksOutput{ImportImageTasks: m.importimagetasks}, nil
}

func (m *mockEc2) DescribeAddresses(_ context.Context, _ *ec2.DescribeAddressesInput, _ ...func(*ec2.Options)) (*ec2.DescribeAddressesOutput, error) {
	return &ec2.DescribeAddressesOutput{Addresses: m.addresss}, nil
}

func (m *mockEc2) DescribeSnapshots(_ context.Context, _ *ec2.DescribeSnapshotsInput, _ ...func(*ec2.Options)) (*ec2.DescribeSnapshotsOutput, error) {
	return &ec2.DescribeSnapshotsOutput{Snapshots: m.snapshots}, nil
}

func (m *mockEc2) DescribeNetworkInterfaces(_ context.Context, _ *ec2.DescribeNetworkInterfacesInput, _ ...func(*ec2.Options)) (*ec2.DescribeNetworkInterfacesOutput, error) {
	return &ec2.DescribeNetworkInterfacesOutput{NetworkInterfaces: m.networkinterfaces}, nil
}

type mockEcr struct {
	awsfetch.EcrAPI
	manualEcrMock
	repositorys []ecrtypes.Repository
}

// cloud.Service, so that a mock can stand in for a whole service in the
// registry. The graph-building tests drive the fetchers directly, so these are
// inert on purpose.
func (m *mockEcr) Name() string            { return "" }
func (m *mockEcr) Region() string          { return "" }
func (m *mockEcr) Profile() string         { return "" }
func (m *mockEcr) ResourceTypes() []string { return []string{} }
func (m *mockEcr) IsSyncDisabled() bool    { return false }

func (m *mockEcr) Fetch(context.Context) (cloud.GraphAPI, error) {
	return nil, nil
}

func (m *mockEcr) FetchByType(context.Context, string) (cloud.GraphAPI, error) {
	return nil, nil
}

func (m *mockEcr) DescribeRepositories(_ context.Context, _ *ecr.DescribeRepositoriesInput, _ ...func(*ecr.Options)) (*ecr.DescribeRepositoriesOutput, error) {
	return &ecr.DescribeRepositoriesOutput{Repositories: m.repositorys}, nil
}

type mockEcs struct {
	awsfetch.EcsAPI
	manualEcsMock
}

// cloud.Service, so that a mock can stand in for a whole service in the
// registry. The graph-building tests drive the fetchers directly, so these are
// inert on purpose.
func (m *mockEcs) Name() string            { return "" }
func (m *mockEcs) Region() string          { return "" }
func (m *mockEcs) Profile() string         { return "" }
func (m *mockEcs) ResourceTypes() []string { return []string{} }
func (m *mockEcs) IsSyncDisabled() bool    { return false }

func (m *mockEcs) Fetch(context.Context) (cloud.GraphAPI, error) {
	return nil, nil
}

func (m *mockEcs) FetchByType(context.Context, string) (cloud.GraphAPI, error) {
	return nil, nil
}

type mockElb struct {
	awsfetch.ElbAPI
	manualElbMock
	loadbalancerdescriptions []elbtypes.LoadBalancerDescription
}

// cloud.Service, so that a mock can stand in for a whole service in the
// registry. The graph-building tests drive the fetchers directly, so these are
// inert on purpose.
func (m *mockElb) Name() string            { return "" }
func (m *mockElb) Region() string          { return "" }
func (m *mockElb) Profile() string         { return "" }
func (m *mockElb) ResourceTypes() []string { return []string{} }
func (m *mockElb) IsSyncDisabled() bool    { return false }

func (m *mockElb) Fetch(context.Context) (cloud.GraphAPI, error) {
	return nil, nil
}

func (m *mockElb) FetchByType(context.Context, string) (cloud.GraphAPI, error) {
	return nil, nil
}

func (m *mockElb) DescribeLoadBalancers(_ context.Context, _ *elasticloadbalancing.DescribeLoadBalancersInput, _ ...func(*elasticloadbalancing.Options)) (*elasticloadbalancing.DescribeLoadBalancersOutput, error) {
	return &elasticloadbalancing.DescribeLoadBalancersOutput{LoadBalancerDescriptions: m.loadbalancerdescriptions}, nil
}

type mockElbv2 struct {
	awsfetch.Elbv2API
	manualElbv2Mock
	loadbalancers []elbv2types.LoadBalancer
	targetgroups  []elbv2types.TargetGroup
}

// cloud.Service, so that a mock can stand in for a whole service in the
// registry. The graph-building tests drive the fetchers directly, so these are
// inert on purpose.
func (m *mockElbv2) Name() string            { return "" }
func (m *mockElbv2) Region() string          { return "" }
func (m *mockElbv2) Profile() string         { return "" }
func (m *mockElbv2) ResourceTypes() []string { return []string{} }
func (m *mockElbv2) IsSyncDisabled() bool    { return false }

func (m *mockElbv2) Fetch(context.Context) (cloud.GraphAPI, error) {
	return nil, nil
}

func (m *mockElbv2) FetchByType(context.Context, string) (cloud.GraphAPI, error) {
	return nil, nil
}

func (m *mockElbv2) DescribeLoadBalancers(_ context.Context, _ *elasticloadbalancingv2.DescribeLoadBalancersInput, _ ...func(*elasticloadbalancingv2.Options)) (*elasticloadbalancingv2.DescribeLoadBalancersOutput, error) {
	return &elasticloadbalancingv2.DescribeLoadBalancersOutput{LoadBalancers: m.loadbalancers}, nil
}

func (m *mockElbv2) DescribeTargetGroups(_ context.Context, _ *elasticloadbalancingv2.DescribeTargetGroupsInput, _ ...func(*elasticloadbalancingv2.Options)) (*elasticloadbalancingv2.DescribeTargetGroupsOutput, error) {
	return &elasticloadbalancingv2.DescribeTargetGroupsOutput{TargetGroups: m.targetgroups}, nil
}

type mockIam struct {
	awsfetch.IamAPI
	manualIamMock
	instanceprofiles  []iamtypes.InstanceProfile
	virtualmfadevices []iamtypes.VirtualMFADevice
}

// cloud.Service, so that a mock can stand in for a whole service in the
// registry. The graph-building tests drive the fetchers directly, so these are
// inert on purpose.
func (m *mockIam) Name() string            { return "" }
func (m *mockIam) Region() string          { return "" }
func (m *mockIam) Profile() string         { return "" }
func (m *mockIam) ResourceTypes() []string { return []string{} }
func (m *mockIam) IsSyncDisabled() bool    { return false }

func (m *mockIam) Fetch(context.Context) (cloud.GraphAPI, error) {
	return nil, nil
}

func (m *mockIam) FetchByType(context.Context, string) (cloud.GraphAPI, error) {
	return nil, nil
}

func (m *mockIam) ListInstanceProfiles(_ context.Context, _ *iam.ListInstanceProfilesInput, _ ...func(*iam.Options)) (*iam.ListInstanceProfilesOutput, error) {
	return &iam.ListInstanceProfilesOutput{InstanceProfiles: m.instanceprofiles}, nil
}

func (m *mockIam) ListVirtualMFADevices(_ context.Context, _ *iam.ListVirtualMFADevicesInput, _ ...func(*iam.Options)) (*iam.ListVirtualMFADevicesOutput, error) {
	return &iam.ListVirtualMFADevicesOutput{VirtualMFADevices: m.virtualmfadevices}, nil
}

type mockLambda struct {
	awsfetch.LambdaAPI
	manualLambdaMock
	functionconfigurations []lambdatypes.FunctionConfiguration
}

// cloud.Service, so that a mock can stand in for a whole service in the
// registry. The graph-building tests drive the fetchers directly, so these are
// inert on purpose.
func (m *mockLambda) Name() string            { return "" }
func (m *mockLambda) Region() string          { return "" }
func (m *mockLambda) Profile() string         { return "" }
func (m *mockLambda) ResourceTypes() []string { return []string{} }
func (m *mockLambda) IsSyncDisabled() bool    { return false }

func (m *mockLambda) Fetch(context.Context) (cloud.GraphAPI, error) {
	return nil, nil
}

func (m *mockLambda) FetchByType(context.Context, string) (cloud.GraphAPI, error) {
	return nil, nil
}

func (m *mockLambda) ListFunctions(_ context.Context, _ *lambda.ListFunctionsInput, _ ...func(*lambda.Options)) (*lambda.ListFunctionsOutput, error) {
	return &lambda.ListFunctionsOutput{Functions: m.functionconfigurations}, nil
}

type mockRds struct {
	awsfetch.RdsAPI
	manualRdsMock
	dbinstances    []rdstypes.DBInstance
	dbsubnetgroups []rdstypes.DBSubnetGroup
}

// cloud.Service, so that a mock can stand in for a whole service in the
// registry. The graph-building tests drive the fetchers directly, so these are
// inert on purpose.
func (m *mockRds) Name() string            { return "" }
func (m *mockRds) Region() string          { return "" }
func (m *mockRds) Profile() string         { return "" }
func (m *mockRds) ResourceTypes() []string { return []string{} }
func (m *mockRds) IsSyncDisabled() bool    { return false }

func (m *mockRds) Fetch(context.Context) (cloud.GraphAPI, error) {
	return nil, nil
}

func (m *mockRds) FetchByType(context.Context, string) (cloud.GraphAPI, error) {
	return nil, nil
}

func (m *mockRds) DescribeDBInstances(_ context.Context, _ *rds.DescribeDBInstancesInput, _ ...func(*rds.Options)) (*rds.DescribeDBInstancesOutput, error) {
	return &rds.DescribeDBInstancesOutput{DBInstances: m.dbinstances}, nil
}

func (m *mockRds) DescribeDBSubnetGroups(_ context.Context, _ *rds.DescribeDBSubnetGroupsInput, _ ...func(*rds.Options)) (*rds.DescribeDBSubnetGroupsOutput, error) {
	return &rds.DescribeDBSubnetGroupsOutput{DBSubnetGroups: m.dbsubnetgroups}, nil
}

type mockRoute53 struct {
	awsfetch.Route53API
	manualRoute53Mock
	hostedzones []route53types.HostedZone
}

// cloud.Service, so that a mock can stand in for a whole service in the
// registry. The graph-building tests drive the fetchers directly, so these are
// inert on purpose.
func (m *mockRoute53) Name() string            { return "" }
func (m *mockRoute53) Region() string          { return "" }
func (m *mockRoute53) Profile() string         { return "" }
func (m *mockRoute53) ResourceTypes() []string { return []string{} }
func (m *mockRoute53) IsSyncDisabled() bool    { return false }

func (m *mockRoute53) Fetch(context.Context) (cloud.GraphAPI, error) {
	return nil, nil
}

func (m *mockRoute53) FetchByType(context.Context, string) (cloud.GraphAPI, error) {
	return nil, nil
}

func (m *mockRoute53) ListHostedZones(_ context.Context, _ *route53.ListHostedZonesInput, _ ...func(*route53.Options)) (*route53.ListHostedZonesOutput, error) {
	return &route53.ListHostedZonesOutput{HostedZones: m.hostedzones}, nil
}

type mockS3 struct {
	awsfetch.S3API
	manualS3Mock
}

// cloud.Service, so that a mock can stand in for a whole service in the
// registry. The graph-building tests drive the fetchers directly, so these are
// inert on purpose.
func (m *mockS3) Name() string            { return "" }
func (m *mockS3) Region() string          { return "" }
func (m *mockS3) Profile() string         { return "" }
func (m *mockS3) ResourceTypes() []string { return []string{} }
func (m *mockS3) IsSyncDisabled() bool    { return false }

func (m *mockS3) Fetch(context.Context) (cloud.GraphAPI, error) {
	return nil, nil
}

func (m *mockS3) FetchByType(context.Context, string) (cloud.GraphAPI, error) {
	return nil, nil
}

type mockSns struct {
	awsfetch.SnsAPI
	manualSnsMock
	subscriptions []snstypes.Subscription
	topics        []snstypes.Topic
}

// cloud.Service, so that a mock can stand in for a whole service in the
// registry. The graph-building tests drive the fetchers directly, so these are
// inert on purpose.
func (m *mockSns) Name() string            { return "" }
func (m *mockSns) Region() string          { return "" }
func (m *mockSns) Profile() string         { return "" }
func (m *mockSns) ResourceTypes() []string { return []string{} }
func (m *mockSns) IsSyncDisabled() bool    { return false }

func (m *mockSns) Fetch(context.Context) (cloud.GraphAPI, error) {
	return nil, nil
}

func (m *mockSns) FetchByType(context.Context, string) (cloud.GraphAPI, error) {
	return nil, nil
}

func (m *mockSns) ListSubscriptions(_ context.Context, _ *sns.ListSubscriptionsInput, _ ...func(*sns.Options)) (*sns.ListSubscriptionsOutput, error) {
	return &sns.ListSubscriptionsOutput{Subscriptions: m.subscriptions}, nil
}

func (m *mockSns) ListTopics(_ context.Context, _ *sns.ListTopicsInput, _ ...func(*sns.Options)) (*sns.ListTopicsOutput, error) {
	return &sns.ListTopicsOutput{Topics: m.topics}, nil
}

type mockSqs struct {
	awsfetch.SqsAPI
	manualSqsMock
}

// cloud.Service, so that a mock can stand in for a whole service in the
// registry. The graph-building tests drive the fetchers directly, so these are
// inert on purpose.
func (m *mockSqs) Name() string            { return "" }
func (m *mockSqs) Region() string          { return "" }
func (m *mockSqs) Profile() string         { return "" }
func (m *mockSqs) ResourceTypes() []string { return []string{} }
func (m *mockSqs) IsSyncDisabled() bool    { return false }

func (m *mockSqs) Fetch(context.Context) (cloud.GraphAPI, error) {
	return nil, nil
}

func (m *mockSqs) FetchByType(context.Context, string) (cloud.GraphAPI, error) {
	return nil, nil
}

type mockSts struct {
	awsfetch.StsAPI
	manualStsMock
}

// cloud.Service, so that a mock can stand in for a whole service in the
// registry. The graph-building tests drive the fetchers directly, so these are
// inert on purpose.
func (m *mockSts) Name() string            { return "" }
func (m *mockSts) Region() string          { return "" }
func (m *mockSts) Profile() string         { return "" }
func (m *mockSts) ResourceTypes() []string { return []string{} }
func (m *mockSts) IsSyncDisabled() bool    { return false }

func (m *mockSts) Fetch(context.Context) (cloud.GraphAPI, error) {
	return nil, nil
}

func (m *mockSts) FetchByType(context.Context, string) (cloud.GraphAPI, error) {
	return nil, nil
}
