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

package awsconv

// This file is the safety net for the AWS SDK v2 migration.
//
// The property extraction in NewResource is reflective, and SDK v2 changes the
// value semantics it assumes: scalars stop being pointers, lists hold values
// instead of pointers, and enums become named string types. None of that is a
// compile error. Without a test at this level the migration would produce a
// binary that builds, runs, and silently lists resources with empty columns.
//
// The test therefore drives NewResource for every resource type the fetchers
// produce and asserts that every property declared in awsResourcesDef actually
// comes out. Fixtures are built by reflection rather than by hand because
// awsResourcesDef declares roughly 450 properties; when the SDK version changes,
// only sdkPrototypes and the handful of overrides below need touching, while the
// assertions stay as they are.

import (
	"net/url"
	"reflect"
	"strings"
	"testing"
	"time"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	acmtypes "github.com/aws/aws-sdk-go-v2/service/acm/types"
	autoscalingtypes "github.com/aws/aws-sdk-go-v2/service/autoscaling/types"
	cloudformationtypes "github.com/aws/aws-sdk-go-v2/service/cloudformation/types"
	cloudfronttypes "github.com/aws/aws-sdk-go-v2/service/cloudfront/types"
	cloudwatchtypes "github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	ecrtypes "github.com/aws/aws-sdk-go-v2/service/ecr/types"
	ecstypes "github.com/aws/aws-sdk-go-v2/service/ecs/types"
	elbtypes "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancing/types"
	elbv2types "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2/types"
	iamtypes "github.com/aws/aws-sdk-go-v2/service/iam/types"
	lambdatypes "github.com/aws/aws-sdk-go-v2/service/lambda/types"
	rdstypes "github.com/aws/aws-sdk-go-v2/service/rds/types"
	route53types "github.com/aws/aws-sdk-go-v2/service/route53/types"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
	snstypes "github.com/aws/aws-sdk-go-v2/service/sns/types"

	"github.com/theazz/awless-ro/cloud"
)

// sdkPrototypes maps each cloud resource type to the AWS SDK shapes the fetchers
// hand to NewResource, with the field InitResource derives the ID from already
// set. This is the single place that has to change when the SDK version changes.
//
// Most resource types have exactly one shape. cloud.User has two, because its
// fetcher queries both GetAccountAuthorizationDetails and ListUsers and merges
// the results, and the two shapes carry different fields. A property therefore
// has to be extracted by at least one shape, not by every shape.
var sdkPrototypes = map[string][]func() interface{}{
	// EC2
	cloud.Instance:         {func() interface{} { return ec2types.Instance{InstanceId: awssdk.String("i-1234")} }},
	cloud.Vpc:              {func() interface{} { return ec2types.Vpc{VpcId: awssdk.String("vpc-1234")} }},
	cloud.Subnet:           {func() interface{} { return ec2types.Subnet{SubnetId: awssdk.String("subnet-1234")} }},
	cloud.SecurityGroup:    {func() interface{} { return ec2types.SecurityGroup{GroupId: awssdk.String("sg-1234")} }},
	cloud.Keypair:          {func() interface{} { return ec2types.KeyPairInfo{KeyName: awssdk.String("my-key")} }},
	cloud.Volume:           {func() interface{} { return ec2types.Volume{VolumeId: awssdk.String("vol-1234")} }},
	cloud.Snapshot:         {func() interface{} { return ec2types.Snapshot{SnapshotId: awssdk.String("snap-1234")} }},
	cloud.Image:            {func() interface{} { return ec2types.Image{ImageId: awssdk.String("ami-1234")} }},
	cloud.ImportImageTask:  {func() interface{} { return ec2types.ImportImageTask{ImportTaskId: awssdk.String("import-1234")} }},
	cloud.InternetGateway:  {func() interface{} { return ec2types.InternetGateway{InternetGatewayId: awssdk.String("igw-1234")} }},
	cloud.NatGateway:       {func() interface{} { return ec2types.NatGateway{NatGatewayId: awssdk.String("nat-1234")} }},
	cloud.RouteTable:       {func() interface{} { return ec2types.RouteTable{RouteTableId: awssdk.String("rtb-1234")} }},
	cloud.AvailabilityZone: {func() interface{} { return ec2types.AvailabilityZone{ZoneName: awssdk.String("eu-west-1a")} }},
	cloud.ElasticIP:        {func() interface{} { return ec2types.Address{AllocationId: awssdk.String("eipalloc-1234")} }},
	cloud.NetworkInterface: {func() interface{} {
		return ec2types.NetworkInterface{NetworkInterfaceId: awssdk.String("eni-1234")}
	}},

	// Load balancing
	cloud.ClassicLoadBalancer: {func() interface{} {
		return elbtypes.LoadBalancerDescription{LoadBalancerName: awssdk.String("my-classic-lb")}
	}},
	cloud.LoadBalancer: {func() interface{} {
		return elbv2types.LoadBalancer{LoadBalancerArn: awssdk.String("arn:aws:elasticloadbalancing:lb/1")}
	}},
	cloud.TargetGroup: {func() interface{} {
		return elbv2types.TargetGroup{TargetGroupArn: awssdk.String("arn:aws:elasticloadbalancing:tg/1")}
	}},
	cloud.Listener: {func() interface{} {
		return elbv2types.Listener{ListenerArn: awssdk.String("arn:aws:elasticloadbalancing:listener/1")}
	}},

	// RDS
	cloud.Database: {func() interface{} { return rdstypes.DBInstance{DBInstanceIdentifier: awssdk.String("my-db")} }},
	cloud.DbSubnetGroup: {func() interface{} {
		return rdstypes.DBSubnetGroup{DBSubnetGroupArn: awssdk.String("arn:aws:rds:subgrp/1")}
	}},

	// Autoscaling
	cloud.LaunchConfiguration: {func() interface{} {
		return autoscalingtypes.LaunchConfiguration{LaunchConfigurationARN: awssdk.String("arn:aws:autoscaling:lc/1")}
	}},
	cloud.ScalingGroup: {func() interface{} {
		return autoscalingtypes.AutoScalingGroup{AutoScalingGroupARN: awssdk.String("arn:aws:autoscaling:asg/1")}
	}},
	cloud.ScalingPolicy: {func() interface{} {
		return autoscalingtypes.ScalingPolicy{PolicyARN: awssdk.String("arn:aws:autoscaling:policy/1")}
	}},

	// Containers
	cloud.Repository:       {func() interface{} { return ecrtypes.Repository{RepositoryArn: awssdk.String("arn:aws:ecr:repo/1")} }},
	cloud.ContainerCluster: {func() interface{} { return ecstypes.Cluster{ClusterArn: awssdk.String("arn:aws:ecs:cluster/1")} }},
	cloud.ContainerTask: {func() interface{} {
		return ecstypes.TaskDefinition{TaskDefinitionArn: awssdk.String("arn:aws:ecs:task-definition/1")}
	}},
	cloud.Container: {func() interface{} { return ecstypes.Container{ContainerArn: awssdk.String("arn:aws:ecs:container/1")} }},
	cloud.ContainerInstance: {func() interface{} {
		return ecstypes.ContainerInstance{ContainerInstanceArn: awssdk.String("arn:aws:ecs:container-instance/1")}
	}},

	// ACM
	cloud.Certificate: {func() interface{} {
		return acmtypes.CertificateSummary{CertificateArn: awssdk.String("arn:aws:acm:certificate/1")}
	}},

	// IAM
	//
	// Users come back as two shapes that carry different fields: UserDetail from
	// GetAccountAuthorizationDetails has the inline policies, User from
	// ListUsers has PasswordLastUsed.
	cloud.User: {
		func() interface{} { return iamtypes.UserDetail{UserId: awssdk.String("AIDUSER1")} },
		func() interface{} { return iamtypes.User{UserId: awssdk.String("AIDUSER1")} },
	},
	cloud.Role:  {func() interface{} { return iamtypes.RoleDetail{RoleId: awssdk.String("AROLE1")} }},
	cloud.Group: {func() interface{} { return iamtypes.GroupDetail{GroupId: awssdk.String("AGROUP1")} }},
	cloud.Policy: {func() interface{} {
		return iamtypes.ManagedPolicyDetail{PolicyId: awssdk.String("APOLICY1")}
	}},
	cloud.AccessKey: {func() interface{} {
		return iamtypes.AccessKeyMetadata{AccessKeyId: awssdk.String("AKIAEXAMPLE1")}
	}},
	cloud.InstanceProfile: {func() interface{} {
		return iamtypes.InstanceProfile{InstanceProfileId: awssdk.String("AIPROFILE1")}
	}},
	cloud.MFADevice: {func() interface{} {
		return iamtypes.VirtualMFADevice{SerialNumber: awssdk.String("arn:aws:iam::mfa/user")}
	}},

	// S3
	cloud.Bucket:   {func() interface{} { return s3types.Bucket{Name: awssdk.String("my-bucket")} }},
	cloud.S3Object: {func() interface{} { return s3types.Object{Key: awssdk.String("my/object")} }},

	// Messaging
	cloud.Subscription: {func() interface{} {
		return snstypes.Subscription{
			Endpoint:        awssdk.String("me@example.com"),
			SubscriptionArn: awssdk.String("arn:aws:sns:eu-west-1:123456789012:alerts:sub-uuid"),
		}
	}},
	cloud.Topic: {func() interface{} { return snstypes.Topic{TopicArn: awssdk.String("arn:aws:sns:topic/1")} }},

	// DNS
	cloud.Zone: {func() interface{} { return route53types.HostedZone{Id: awssdk.String("/hostedzone/Z1")} }},
	// A record is built from a record set plus the zone it was read from. ZoneName
	// is pre-set so the Zone mapping is extracted rather than reported absent.
	cloud.Record: {func() interface{} {
		return RecordSetInZone{
			ResourceRecordSet: route53types.ResourceRecordSet{
				Name: awssdk.String("example.com."), Type: route53types.RRTypeA,
			},
			ZoneId:   "/hostedzone/Z1",
			ZoneName: "example.com.",
		}
	}},

	// Lambda
	cloud.Function: {func() interface{} {
		return lambdatypes.FunctionConfiguration{FunctionArn: awssdk.String("arn:aws:lambda:function/1")}
	}},

	// Monitoring
	cloud.Metric: {func() interface{} {
		return cloudwatchtypes.Metric{Namespace: awssdk.String("AWS/EC2"), MetricName: awssdk.String("CPUUtilization")}
	}},
	cloud.Alarm: {func() interface{} {
		return cloudwatchtypes.MetricAlarm{AlarmArn: awssdk.String("arn:aws:cloudwatch:alarm/1")}
	}},

	// CDN
	cloud.Distribution: {func() interface{} {
		return cloudfronttypes.DistributionSummary{Id: awssdk.String("E1DISTRIBUTION")}
	}},

	// CloudFormation
	cloud.Stack: {func() interface{} {
		return cloudformationtypes.Stack{StackId: awssdk.String("arn:aws:cloudformation:stack/1")}
	}},
}

// fieldOverrides supplies values for fields whose transform parses their
// content, keyed by "<resource type>.<source field>". The generic synthesiser
// cannot guess that, say, a role's trust policy has to be URL-encoded JSON.
var fieldOverrides = map[string]interface{}{
	cloud.Role + ".AssumeRolePolicyDocument": awssdk.String(url.QueryEscape(`{"Version": "2012-10-17"}`)),
	cloud.Image + ".CreationDate":            awssdk.String("2019-07-04T11:22:33.000Z"),
	cloud.Function + ".LastModified":         awssdk.String("2019-07-04T11:22:33.000+0000"),
}

var (
	sampleTime = time.Date(2019, time.July, 4, 11, 22, 33, 0, time.UTC)

	// typeSynths covers shapes whose transform inspects the contents, so a
	// zero-valued element would either error or yield nothing.
	typeSynths = map[reflect.Type]func() interface{}{
		reflect.TypeOf([]ec2types.Tag{}): func() interface{} {
			return []ec2types.Tag{{Key: awssdk.String("Name"), Value: awssdk.String("a-name")}}
		},
		reflect.TypeOf([]autoscalingtypes.TagDescription{}): func() interface{} {
			return []autoscalingtypes.TagDescription{{Key: awssdk.String("Name"), Value: awssdk.String("a-name")}}
		},
		reflect.TypeOf([]ec2types.IpPermission{}): func() interface{} {
			return []ec2types.IpPermission{{
				IpProtocol: awssdk.String("tcp"),
				FromPort:   awssdk.Int32(22),
				ToPort:     awssdk.Int32(22),
				IpRanges:   []ec2types.IpRange{{CidrIp: awssdk.String("10.0.0.0/24")}},
			}}
		},
		reflect.TypeOf([]ec2types.Route{}): func() interface{} {
			return []ec2types.Route{{
				DestinationCidrBlock: awssdk.String("0.0.0.0/0"),
				GatewayId:            awssdk.String("igw-1234"),
			}}
		},
		reflect.TypeOf([]ec2types.RouteTableAssociation{}): func() interface{} {
			return []ec2types.RouteTableAssociation{{
				Main:                    awssdk.Bool(true),
				RouteTableAssociationId: awssdk.String("rtbassoc-1234"),
				SubnetId:                awssdk.String("subnet-1234"),
			}}
		},
		reflect.TypeOf([]elbtypes.ListenerDescription{}): func() interface{} {
			return []elbtypes.ListenerDescription{{Listener: &elbtypes.Listener{
				Protocol:         awssdk.String("HTTPS"),
				LoadBalancerPort: 443,
				InstanceProtocol: awssdk.String("HTTP"),
				InstancePort:     awssdk.Int32(8080),
			}}}
		},
		reflect.TypeOf([]cloudwatchtypes.Dimension{}): func() interface{} {
			return []cloudwatchtypes.Dimension{{Name: awssdk.String("InstanceId"), Value: awssdk.String("i-1234")}}
		},
		reflect.TypeOf([]ecstypes.Attribute{}): func() interface{} {
			return []ecstypes.Attribute{{Name: awssdk.String("ecs.os-type"), Value: awssdk.String("linux")}}
		},
		reflect.TypeOf([]ecstypes.ContainerDefinition{}): func() interface{} {
			return []ecstypes.ContainerDefinition{{Name: awssdk.String("web"), Image: awssdk.String("nginx:latest")}}
		},
		reflect.TypeOf([]iamtypes.PolicyVersion{}): func() interface{} {
			return []iamtypes.PolicyVersion{{
				IsDefaultVersion: true,
				Document:         awssdk.String(url.QueryEscape(`{"Version": "2012-10-17"}`)),
			}}
		},
		reflect.TypeOf([]cloudformationtypes.Output{}): func() interface{} {
			return []cloudformationtypes.Output{{OutputKey: awssdk.String("Url"), OutputValue: awssdk.String("https://example.com")}}
		},
		reflect.TypeOf([]cloudformationtypes.Parameter{}): func() interface{} {
			return []cloudformationtypes.Parameter{{ParameterKey: awssdk.String("Env"), ParameterValue: awssdk.String("prod")}}
		},
		reflect.TypeOf(cloudfronttypes.Origins{}): func() interface{} {
			return cloudfronttypes.Origins{Items: []cloudfronttypes.Origin{{
				Id:         awssdk.String("origin-1"),
				DomainName: awssdk.String("origin.example.com"),
				OriginPath: awssdk.String("/assets"),
			}}}
		},
		reflect.TypeOf(cloudfronttypes.Aliases{}): func() interface{} {
			return cloudfronttypes.Aliases{Items: []string{"cdn.example.com"}}
		},
	}
)

// populateMappedFields fills the source fields awsResourcesDef reads for the
// given resource type, so that a missing property can only mean the extraction
// itself failed. Fields the shape does not have are skipped and reported back,
// since a resource type may be built from several shapes.
// fillPrototype copies a prototype into addressable storage, populates it and
// hands back the filled value. SDK v2 shapes reach NewResource by value, so the
// prototypes are values too and have to be boxed before they can be written to.
func fillPrototype(t *testing.T, rtype string, proto interface{}) (interface{}, []string) {
	t.Helper()

	box := reflect.New(reflect.TypeOf(proto))
	box.Elem().Set(reflect.ValueOf(proto))
	absent := populateMappedFields(t, rtype, box.Elem())
	return box.Elem().Interface(), absent
}

func populateMappedFields(t *testing.T, rtype string, elem reflect.Value) (absent []string) {
	t.Helper()

	for prop, trans := range awsResourcesDef[rtype] {
		if trans.transform == nil {
			continue // handled by TestMappedPropertiesWithoutTransform
		}

		field := elem.FieldByName(trans.name)
		if !field.IsValid() {
			absent = append(absent, prop)
			continue
		}
		if !field.CanSet() {
			t.Errorf("%s: source field %q of %s is not settable", rtype, trans.name, elem.Type())
			continue
		}
		if !field.IsZero() {
			continue // already set, typically the ID field
		}

		if override, ok := fieldOverrides[rtype+"."+trans.name]; ok {
			field.Set(reflect.ValueOf(override))
			continue
		}
		value, ok := synthesise(field.Type(), 0)
		if !ok {
			t.Errorf("%s: no synthetic value for source field %q of type %s",
				rtype, trans.name, field.Type())
			continue
		}
		field.Set(value)
	}
	return absent
}

// synthesise builds a non-zero value of the given type. Depth is bounded because
// some SDK shapes are deeply nested and one level below the mapped field is all
// the transforms ever look at.
func synthesise(typ reflect.Type, depth int) (reflect.Value, bool) {
	if synth, ok := typeSynths[typ]; ok {
		return reflect.ValueOf(synth()), true
	}
	// Each logical level costs several steps here (slice -> pointer -> struct ->
	// field -> pointer -> scalar), so the bound has to be generous. It exists
	// only to stop the walk on self-referential SDK shapes.
	if depth > 8 {
		return reflect.Value{}, false
	}

	switch typ.Kind() {
	case reflect.String:
		return reflect.ValueOf("a-value").Convert(typ), true
	case reflect.Bool:
		return reflect.ValueOf(true).Convert(typ), true
	case reflect.Int, reflect.Int32, reflect.Int64:
		return reflect.ValueOf(int64(42)).Convert(typ), true
	case reflect.Float32, reflect.Float64:
		return reflect.ValueOf(float64(42)).Convert(typ), true

	case reflect.Struct:
		if typ == reflect.TypeOf(time.Time{}) {
			return reflect.ValueOf(sampleTime), true
		}
		out := reflect.New(typ).Elem()
		for i := 0; i < typ.NumField(); i++ {
			f := out.Field(i)
			if !f.CanSet() || strings.HasPrefix(typ.Field(i).Name, "_") {
				continue
			}
			if v, ok := synthesise(f.Type(), depth+1); ok {
				f.Set(v)
			}
		}
		return out, true

	case reflect.Ptr:
		inner, ok := synthesise(typ.Elem(), depth+1)
		if !ok {
			return reflect.Value{}, false
		}
		out := reflect.New(typ.Elem())
		out.Elem().Set(inner)
		return out, true

	case reflect.Slice:
		elem, ok := synthesise(typ.Elem(), depth+1)
		if !ok {
			return reflect.Value{}, false
		}
		out := reflect.MakeSlice(typ, 0, 1)
		return reflect.Append(out, elem), true

	case reflect.Map:
		key, ok := synthesise(typ.Key(), depth+1)
		if !ok {
			return reflect.Value{}, false
		}
		val, ok := synthesise(typ.Elem(), depth+1)
		if !ok {
			return reflect.Value{}, false
		}
		out := reflect.MakeMap(typ)
		out.SetMapIndex(key, val)
		return out, true
	}

	return reflect.Value{}, false
}

func TestEveryResourceTypeHasAPrototype(t *testing.T) {
	for rtype, def := range awsResourcesDef {
		if len(def) == 0 {
			continue // cloud.Queue: its properties are set by the fetcher
		}
		if _, ok := sdkPrototypes[rtype]; !ok {
			t.Errorf("resource type %q has property mappings but no SDK prototype; add one to sdkPrototypes", rtype)
		}
	}
	for rtype := range sdkPrototypes {
		if _, ok := awsResourcesDef[rtype]; !ok {
			t.Errorf("sdkPrototypes has an entry for %q, which has no property mappings", rtype)
		}
	}
}

func TestNewResourceExtractsEveryMappedProperty(t *testing.T) {
	for rtype, def := range awsResourcesDef {
		if len(def) == 0 {
			continue
		}
		protos, ok := sdkPrototypes[rtype]
		if !ok {
			continue // reported by TestEveryResourceTypeHasAPrototype
		}

		t.Run(rtype, func(t *testing.T) {
			// A property counts as extracted when at least one of the shapes
			// this resource type is built from yields it.
			extracted := make(map[string]bool)

			for _, proto := range protos {
				source, absent := fillPrototype(t, rtype, proto())

				res, err := NewResource(source)
				if err != nil {
					t.Fatalf("NewResource(%T): %s", source, err)
				}
				if got, want := res.Type(), rtype; got != want {
					t.Errorf("%T: resource type: got %q, want %q", source, got, want)
				}
				if res.Id() == "" {
					t.Errorf("%T: resource id is empty; check how InitResource derives it", source)
				}

				skipped := make(map[string]bool, len(absent))
				for _, prop := range absent {
					skipped[prop] = true
				}

				for prop, value := range res.Properties() {
					if value != nil {
						extracted[prop] = true
					}
				}
				for prop, trans := range def {
					if trans.transform == nil || skipped[prop] {
						continue
					}
					if value, present := res.Properties()[prop]; present && value == nil {
						t.Errorf("%T: property %q (from source field %q) was extracted as nil",
							source, prop, trans.name)
					}
				}
			}

			for prop, trans := range def {
				if trans.transform == nil {
					continue
				}
				if !extracted[prop] {
					t.Errorf("property %q (from source field %q) is not extracted by any of the %d shape(s) "+
						"this resource type is built from", prop, trans.name, len(protos))
				}
			}
		})
	}
}

// TestMappedPropertiesWithoutTransform pins down the entries in awsResourcesDef
// that declare a source field but no transform and no fetch function. Such an
// entry is inert: NewResource skips it, and the property has to be filled in by
// the fetcher instead. The list is asserted so that a new inert entry, which
// would otherwise look like a working mapping, shows up as a failure.
func TestMappedPropertiesWithoutTransform(t *testing.T) {
	// Empty: the last inert entry, cloud.Record's Zone, now reads
	// RecordSetInZone.ZoneName. The test stays as the guard against a new one.
	expected := map[string]bool{}

	found := make(map[string]bool)
	for rtype, def := range awsResourcesDef {
		for prop, trans := range def {
			if trans.transform == nil && trans.fetch == nil {
				found[rtype+"."+prop] = true
			}
		}
	}

	for key := range found {
		if !expected[key] {
			t.Errorf("%s declares a source field but neither a transform nor a fetch function, so NewResource ignores it", key)
		}
	}
	for key := range expected {
		if !found[key] {
			t.Errorf("%s is listed as inert but now has a transform; remove it from the expected list", key)
		}
	}
}

// TestSafetyNetDetectsABrokenMapping checks that the net above actually catches
// the failure it exists for. Without this, a refactor that quietly stops
// extracting properties could leave the suite green.
func TestSafetyNetDetectsABrokenMapping(t *testing.T) {
	source, absent := fillPrototype(t, cloud.Instance, ec2types.Instance{InstanceId: awssdk.String("i-1234")})
	if len(absent) > 0 {
		t.Fatalf("ec2types.Instance is missing source fields for %v", absent)
	}

	res, err := NewResource(source)
	if err != nil {
		t.Fatalf("baseline NewResource: %s", err)
	}
	if _, ok := res.Properties()["Type"]; !ok {
		t.Fatal("baseline is already broken: instance Type was not extracted")
	}

	// Point the mapping at a field the SDK shape does not have, which is what a
	// field rename in a new SDK version looks like.
	original := awsResourcesDef[cloud.Instance]["Type"]
	awsResourcesDef[cloud.Instance]["Type"] = &propertyTransform{
		name:      "InstanceTypeRenamedUpstream",
		transform: original.transform,
	}
	defer func() { awsResourcesDef[cloud.Instance]["Type"] = original }()

	broken, err := NewResource(ec2types.Instance{InstanceId: awssdk.String("i-1234")})
	if err != nil {
		t.Fatalf("NewResource with a broken mapping: %s", err)
	}
	if _, ok := broken.Properties()["Type"]; ok {
		t.Fatal("a mapping pointing at a non-existent source field still produced a property; " +
			"the presence assertions in this file would not catch an SDK field rename")
	}
}
