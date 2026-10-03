package awsservices

import (
	"context"
	"fmt"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ecs"
	ecstypes "github.com/aws/aws-sdk-go-v2/service/ecs/types"
	"github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2"
	elbv2types "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2/types"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	iamtypes "github.com/aws/aws-sdk-go-v2/service/iam/types"
	"github.com/aws/aws-sdk-go-v2/service/route53"
	route53types "github.com/aws/aws-sdk-go-v2/service/route53/types"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
)

// The hand-written half of the service mocks: the canned answers for the
// fetchers that are not generated. gen_mocks_test.go embeds each of these
// structs into the matching mock, because Go does not let one file add fields to
// a type declared in another.
//
// Services whose fetchers are all generated still need an empty struct here for
// the generated file to embed.
type (
	manualAcmMock            struct{}
	manualAutoscalingMock    struct{}
	manualCloudformationMock struct{}
	manualCloudfrontMock     struct{}
	manualCloudwatchMock     struct{}
	manualEc2Mock            struct{}
	manualEcrMock            struct{}
	manualElbMock            struct{}
	manualLambdaMock         struct{}
	manualRdsMock            struct{}
	manualSnsMock            struct{}
	manualStsMock            struct{}
)

type manualElbv2Mock struct {
	listeners                []elbv2types.Listener
	targethealthdescriptions map[string][]elbv2types.TargetHealthDescription
}

type manualRoute53Mock struct {
	resourcerecordsets map[string][]route53types.ResourceRecordSet
}

type manualIamMock struct {
	users                []iamtypes.User
	userdetails          []iamtypes.UserDetail
	groupdetails         []iamtypes.GroupDetail
	roledetails          []iamtypes.RoleDetail
	managedpolicydetails []iamtypes.ManagedPolicyDetail
}

type manualS3Mock struct {
	// buckets is keyed by region, so that the region filtering in
	// getBucketsPerRegion can be exercised.
	buckets map[string][]s3types.Bucket
	objects map[string][]s3types.Object
	grants  map[string][]s3types.Grant
}

type manualSqsMock struct {
	queues     []string
	attributes map[string]map[string]string
}

type manualEcsMock struct {
	clusters                []ecstypes.Cluster
	taskdefinitions         []ecstypes.TaskDefinition
	tasks                   map[string][]ecstypes.Task
	tasksNames              map[string][]string
	containerinstances      map[string][]ecstypes.ContainerInstance
	containerinstancesNames map[string][]string
}

func (m *mockElbv2) DescribeListeners(_ context.Context, input *elasticloadbalancingv2.DescribeListenersInput, _ ...func(*elasticloadbalancingv2.Options)) (*elasticloadbalancingv2.DescribeListenersOutput, error) {
	perBalancer := make(map[string][]elbv2types.Listener)
	for _, l := range m.listeners {
		arn := awssdk.ToString(l.LoadBalancerArn)
		perBalancer[arn] = append(perBalancer[arn], l)
	}
	return &elasticloadbalancingv2.DescribeListenersOutput{
		Listeners: perBalancer[awssdk.ToString(input.LoadBalancerArn)],
	}, nil
}

func (m *mockElbv2) DescribeTargetHealth(_ context.Context, input *elasticloadbalancingv2.DescribeTargetHealthInput, _ ...func(*elasticloadbalancingv2.Options)) (*elasticloadbalancingv2.DescribeTargetHealthOutput, error) {
	return &elasticloadbalancingv2.DescribeTargetHealthOutput{
		TargetHealthDescriptions: m.targethealthdescriptions[awssdk.ToString(input.TargetGroupArn)],
	}, nil
}

func (m *mockRoute53) ListResourceRecordSets(_ context.Context, input *route53.ListResourceRecordSetsInput, _ ...func(*route53.Options)) (*route53.ListResourceRecordSetsOutput, error) {
	return &route53.ListResourceRecordSetsOutput{
		ResourceRecordSets: m.resourcerecordsets[awssdk.ToString(input.HostedZoneId)],
	}, nil
}

func (m *mockIam) ListUsers(_ context.Context, _ *iam.ListUsersInput, _ ...func(*iam.Options)) (*iam.ListUsersOutput, error) {
	return &iam.ListUsersOutput{Users: m.users}, nil
}

func (m *mockIam) ListAccessKeys(_ context.Context, _ *iam.ListAccessKeysInput, _ ...func(*iam.Options)) (*iam.ListAccessKeysOutput, error) {
	return &iam.ListAccessKeysOutput{}, nil
}

func (m *mockIam) GetAccountAuthorizationDetails(_ context.Context, _ *iam.GetAccountAuthorizationDetailsInput, _ ...func(*iam.Options)) (*iam.GetAccountAuthorizationDetailsOutput, error) {
	return &iam.GetAccountAuthorizationDetailsOutput{
		GroupDetailList: m.groupdetails,
		Policies:        m.managedpolicydetails,
		RoleDetailList:  m.roledetails,
		UserDetailList:  m.userdetails,
	}, nil
}

func (m *mockS3) GetBucketAcl(_ context.Context, input *s3.GetBucketAclInput, _ ...func(*s3.Options)) (*s3.GetBucketAclOutput, error) {
	return &s3.GetBucketAclOutput{Grants: m.grants[awssdk.ToString(input.Bucket)]}, nil
}

func (m *mockS3) ListBuckets(_ context.Context, input *s3.ListBucketsInput, _ ...func(*s3.Options)) (*s3.ListBucketsOutput, error) {
	// Keyed by region, as S3 answers with BucketRegion set; "" is us-east-1, as the
	// location constraint used to say.
	var buckets []s3types.Bucket
	for region, bs := range m.buckets {
		if region == "" {
			region = "us-east-1"
		}
		if r := awssdk.ToString(input.BucketRegion); r != "" && r != region {
			continue
		}
		buckets = append(buckets, bs...)
	}
	return &s3.ListBucketsOutput{Buckets: buckets}, nil
}

func (m *mockS3) ListObjectsV2(_ context.Context, input *s3.ListObjectsV2Input, _ ...func(*s3.Options)) (*s3.ListObjectsV2Output, error) {
	return &s3.ListObjectsV2Output{Contents: m.objects[awssdk.ToString(input.Bucket)]}, nil
}

func (m *mockSqs) ListQueues(_ context.Context, _ *sqs.ListQueuesInput, _ ...func(*sqs.Options)) (*sqs.ListQueuesOutput, error) {
	return &sqs.ListQueuesOutput{QueueUrls: m.queues}, nil
}

func (m *mockSqs) GetQueueAttributes(_ context.Context, input *sqs.GetQueueAttributesInput, _ ...func(*sqs.Options)) (*sqs.GetQueueAttributesOutput, error) {
	return &sqs.GetQueueAttributesOutput{
		Attributes: m.attributes[awssdk.ToString(input.QueueUrl)],
	}, nil
}

func (m *mockEcs) ListClusters(_ context.Context, _ *ecs.ListClustersInput, _ ...func(*ecs.Options)) (*ecs.ListClustersOutput, error) {
	var arns []string
	for _, c := range m.clusters {
		arns = append(arns, awssdk.ToString(c.ClusterArn))
	}
	return &ecs.ListClustersOutput{ClusterArns: arns}, nil
}

func (m *mockEcs) DescribeClusters(_ context.Context, input *ecs.DescribeClustersInput, _ ...func(*ecs.Options)) (*ecs.DescribeClustersOutput, error) {
	var clusters []ecstypes.Cluster
	for _, cluster := range m.clusters {
		for _, wanted := range input.Clusters {
			if awssdk.ToString(cluster.ClusterArn) == wanted {
				clusters = append(clusters, cluster)
			}
		}
	}
	return &ecs.DescribeClustersOutput{Clusters: clusters}, nil
}

func (m *mockEcs) ListTaskDefinitions(_ context.Context, _ *ecs.ListTaskDefinitionsInput, _ ...func(*ecs.Options)) (*ecs.ListTaskDefinitionsOutput, error) {
	var arns []string
	for _, def := range m.taskdefinitions {
		arns = append(arns, awssdk.ToString(def.TaskDefinitionArn))
	}
	return &ecs.ListTaskDefinitionsOutput{TaskDefinitionArns: arns}, nil
}

func (m *mockEcs) DescribeTaskDefinition(_ context.Context, input *ecs.DescribeTaskDefinitionInput, _ ...func(*ecs.Options)) (*ecs.DescribeTaskDefinitionOutput, error) {
	wanted := awssdk.ToString(input.TaskDefinition)
	for _, def := range m.taskdefinitions {
		family := awssdk.ToString(def.Family)
		familyRevision := fmt.Sprintf("%s:%d", family, def.Revision)
		if family == wanted || familyRevision == wanted || awssdk.ToString(def.TaskDefinitionArn) == wanted {
			d := def
			return &ecs.DescribeTaskDefinitionOutput{TaskDefinition: &d}, nil
		}
	}
	return nil, fmt.Errorf("task definition not found")
}

func (m *mockEcs) ListTasks(_ context.Context, input *ecs.ListTasksInput, _ ...func(*ecs.Options)) (*ecs.ListTasksOutput, error) {
	if input.DesiredStatus == ecstypes.DesiredStatusStopped {
		return &ecs.ListTasksOutput{}, nil
	}
	return &ecs.ListTasksOutput{TaskArns: m.tasksNames[awssdk.ToString(input.Cluster)]}, nil
}

func (m *mockEcs) DescribeTasks(_ context.Context, input *ecs.DescribeTasksInput, _ ...func(*ecs.Options)) (*ecs.DescribeTasksOutput, error) {
	var tasks []ecstypes.Task
	for _, task := range m.tasks[awssdk.ToString(input.Cluster)] {
		for _, wanted := range input.Tasks {
			if awssdk.ToString(task.TaskArn) == wanted {
				tasks = append(tasks, task)
			}
		}
	}
	return &ecs.DescribeTasksOutput{Tasks: tasks}, nil
}

func (m *mockEcs) ListContainerInstances(_ context.Context, input *ecs.ListContainerInstancesInput, _ ...func(*ecs.Options)) (*ecs.ListContainerInstancesOutput, error) {
	return &ecs.ListContainerInstancesOutput{
		ContainerInstanceArns: m.containerinstancesNames[awssdk.ToString(input.Cluster)],
	}, nil
}

func (m *mockEcs) DescribeContainerInstances(_ context.Context, input *ecs.DescribeContainerInstancesInput, _ ...func(*ecs.Options)) (*ecs.DescribeContainerInstancesOutput, error) {
	return &ecs.DescribeContainerInstancesOutput{
		ContainerInstances: m.containerinstances[awssdk.ToString(input.Cluster)],
	}, nil
}
