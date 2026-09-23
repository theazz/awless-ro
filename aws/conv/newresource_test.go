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

// This file is the safety net for the AWS SDK v2 migration described in
// .kiro/specs/awless-ro/design.md, decision D3.
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

	awssdk "github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/acm"
	"github.com/aws/aws-sdk-go/service/autoscaling"
	"github.com/aws/aws-sdk-go/service/cloudformation"
	"github.com/aws/aws-sdk-go/service/cloudfront"
	"github.com/aws/aws-sdk-go/service/cloudwatch"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/aws/aws-sdk-go/service/ecr"
	"github.com/aws/aws-sdk-go/service/ecs"
	"github.com/aws/aws-sdk-go/service/elb"
	"github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/aws/aws-sdk-go/service/iam"
	"github.com/aws/aws-sdk-go/service/lambda"
	"github.com/aws/aws-sdk-go/service/rds"
	"github.com/aws/aws-sdk-go/service/route53"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/sns"

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
	cloud.Instance:         {func() interface{} { return &ec2.Instance{InstanceId: awssdk.String("i-1234")} }},
	cloud.Vpc:              {func() interface{} { return &ec2.Vpc{VpcId: awssdk.String("vpc-1234")} }},
	cloud.Subnet:           {func() interface{} { return &ec2.Subnet{SubnetId: awssdk.String("subnet-1234")} }},
	cloud.SecurityGroup:    {func() interface{} { return &ec2.SecurityGroup{GroupId: awssdk.String("sg-1234")} }},
	cloud.Keypair:          {func() interface{} { return &ec2.KeyPairInfo{KeyName: awssdk.String("my-key")} }},
	cloud.Volume:           {func() interface{} { return &ec2.Volume{VolumeId: awssdk.String("vol-1234")} }},
	cloud.Snapshot:         {func() interface{} { return &ec2.Snapshot{SnapshotId: awssdk.String("snap-1234")} }},
	cloud.Image:            {func() interface{} { return &ec2.Image{ImageId: awssdk.String("ami-1234")} }},
	cloud.ImportImageTask:  {func() interface{} { return &ec2.ImportImageTask{ImportTaskId: awssdk.String("import-1234")} }},
	cloud.InternetGateway:  {func() interface{} { return &ec2.InternetGateway{InternetGatewayId: awssdk.String("igw-1234")} }},
	cloud.NatGateway:       {func() interface{} { return &ec2.NatGateway{NatGatewayId: awssdk.String("nat-1234")} }},
	cloud.RouteTable:       {func() interface{} { return &ec2.RouteTable{RouteTableId: awssdk.String("rtb-1234")} }},
	cloud.AvailabilityZone: {func() interface{} { return &ec2.AvailabilityZone{ZoneName: awssdk.String("eu-west-1a")} }},
	cloud.ElasticIP:        {func() interface{} { return &ec2.Address{AllocationId: awssdk.String("eipalloc-1234")} }},
	cloud.NetworkInterface: {func() interface{} {
		return &ec2.NetworkInterface{NetworkInterfaceId: awssdk.String("eni-1234")}
	}},

	// Load balancing
	cloud.ClassicLoadBalancer: {func() interface{} {
		return &elb.LoadBalancerDescription{LoadBalancerName: awssdk.String("my-classic-lb")}
	}},
	cloud.LoadBalancer: {func() interface{} {
		return &elbv2.LoadBalancer{LoadBalancerArn: awssdk.String("arn:aws:elasticloadbalancing:lb/1")}
	}},
	cloud.TargetGroup: {func() interface{} {
		return &elbv2.TargetGroup{TargetGroupArn: awssdk.String("arn:aws:elasticloadbalancing:tg/1")}
	}},
	cloud.Listener: {func() interface{} {
		return &elbv2.Listener{ListenerArn: awssdk.String("arn:aws:elasticloadbalancing:listener/1")}
	}},

	// RDS
	cloud.Database:      {func() interface{} { return &rds.DBInstance{DBInstanceIdentifier: awssdk.String("my-db")} }},
	cloud.DbSubnetGroup: {func() interface{} { return &rds.DBSubnetGroup{DBSubnetGroupArn: awssdk.String("arn:aws:rds:subgrp/1")} }},

	// Autoscaling
	cloud.LaunchConfiguration: {func() interface{} {
		return &autoscaling.LaunchConfiguration{LaunchConfigurationARN: awssdk.String("arn:aws:autoscaling:lc/1")}
	}},
	cloud.ScalingGroup: {func() interface{} {
		return &autoscaling.Group{AutoScalingGroupARN: awssdk.String("arn:aws:autoscaling:asg/1")}
	}},
	cloud.ScalingPolicy: {func() interface{} {
		return &autoscaling.ScalingPolicy{PolicyARN: awssdk.String("arn:aws:autoscaling:policy/1")}
	}},

	// Containers
	cloud.Repository:       {func() interface{} { return &ecr.Repository{RepositoryArn: awssdk.String("arn:aws:ecr:repo/1")} }},
	cloud.ContainerCluster: {func() interface{} { return &ecs.Cluster{ClusterArn: awssdk.String("arn:aws:ecs:cluster/1")} }},
	cloud.ContainerTask: {func() interface{} {
		return &ecs.TaskDefinition{TaskDefinitionArn: awssdk.String("arn:aws:ecs:task-definition/1")}
	}},
	cloud.Container: {func() interface{} { return &ecs.Container{ContainerArn: awssdk.String("arn:aws:ecs:container/1")} }},
	cloud.ContainerInstance: {func() interface{} {
		return &ecs.ContainerInstance{ContainerInstanceArn: awssdk.String("arn:aws:ecs:container-instance/1")}
	}},

	// ACM
	cloud.Certificate: {func() interface{} {
		return &acm.CertificateSummary{CertificateArn: awssdk.String("arn:aws:acm:certificate/1")}
	}},

	// IAM
	//
	// Users come back as two shapes that carry different fields: UserDetail from
	// GetAccountAuthorizationDetails has the inline policies, User from
	// ListUsers has PasswordLastUsed.
	cloud.User: {
		func() interface{} { return &iam.UserDetail{UserId: awssdk.String("AIDUSER1")} },
		func() interface{} { return &iam.User{UserId: awssdk.String("AIDUSER1")} },
	},
	cloud.Role:  {func() interface{} { return &iam.RoleDetail{RoleId: awssdk.String("AROLE1")} }},
	cloud.Group: {func() interface{} { return &iam.GroupDetail{GroupId: awssdk.String("AGROUP1")} }},
	cloud.Policy: {func() interface{} {
		return &iam.ManagedPolicyDetail{PolicyId: awssdk.String("APOLICY1")}
	}},
	cloud.AccessKey: {func() interface{} {
		return &iam.AccessKeyMetadata{AccessKeyId: awssdk.String("AKIAEXAMPLE1")}
	}},
	cloud.InstanceProfile: {func() interface{} {
		return &iam.InstanceProfile{InstanceProfileId: awssdk.String("AIPROFILE1")}
	}},
	cloud.MFADevice: {func() interface{} {
		return &iam.VirtualMFADevice{SerialNumber: awssdk.String("arn:aws:iam::mfa/user")}
	}},

	// S3
	cloud.Bucket:   {func() interface{} { return &s3.Bucket{Name: awssdk.String("my-bucket")} }},
	cloud.S3Object: {func() interface{} { return &s3.Object{Key: awssdk.String("my/object")} }},

	// Messaging
	cloud.Subscription: {func() interface{} { return &sns.Subscription{Endpoint: awssdk.String("me@example.com")} }},
	cloud.Topic:        {func() interface{} { return &sns.Topic{TopicArn: awssdk.String("arn:aws:sns:topic/1")} }},

	// DNS
	cloud.Zone: {func() interface{} { return &route53.HostedZone{Id: awssdk.String("/hostedzone/Z1")} }},
	cloud.Record: {func() interface{} {
		return &route53.ResourceRecordSet{Name: awssdk.String("example.com."), Type: awssdk.String("A")}
	}},

	// Lambda
	cloud.Function: {func() interface{} {
		return &lambda.FunctionConfiguration{FunctionArn: awssdk.String("arn:aws:lambda:function/1")}
	}},

	// Monitoring
	cloud.Metric: {func() interface{} {
		return &cloudwatch.Metric{Namespace: awssdk.String("AWS/EC2"), MetricName: awssdk.String("CPUUtilization")}
	}},
	cloud.Alarm: {func() interface{} {
		return &cloudwatch.MetricAlarm{AlarmArn: awssdk.String("arn:aws:cloudwatch:alarm/1")}
	}},

	// CDN
	cloud.Distribution: {func() interface{} {
		return &cloudfront.DistributionSummary{Id: awssdk.String("E1DISTRIBUTION")}
	}},

	// CloudFormation
	cloud.Stack: {func() interface{} {
		return &cloudformation.Stack{StackId: awssdk.String("arn:aws:cloudformation:stack/1")}
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
		reflect.TypeOf([]*ec2.Tag{}): func() interface{} {
			return []*ec2.Tag{{Key: awssdk.String("Name"), Value: awssdk.String("a-name")}}
		},
		reflect.TypeOf([]*autoscaling.TagDescription{}): func() interface{} {
			return []*autoscaling.TagDescription{{Key: awssdk.String("Name"), Value: awssdk.String("a-name")}}
		},
		reflect.TypeOf([]*ec2.IpPermission{}): func() interface{} {
			return []*ec2.IpPermission{{
				IpProtocol: awssdk.String("tcp"),
				FromPort:   awssdk.Int64(22),
				ToPort:     awssdk.Int64(22),
				IpRanges:   []*ec2.IpRange{{CidrIp: awssdk.String("10.0.0.0/24")}},
			}}
		},
		reflect.TypeOf([]*ec2.Route{}): func() interface{} {
			return []*ec2.Route{{
				DestinationCidrBlock: awssdk.String("0.0.0.0/0"),
				GatewayId:            awssdk.String("igw-1234"),
			}}
		},
		reflect.TypeOf([]*ec2.RouteTableAssociation{}): func() interface{} {
			return []*ec2.RouteTableAssociation{{
				Main:                    awssdk.Bool(true),
				RouteTableAssociationId: awssdk.String("rtbassoc-1234"),
				SubnetId:                awssdk.String("subnet-1234"),
			}}
		},
		reflect.TypeOf([]*elb.ListenerDescription{}): func() interface{} {
			return []*elb.ListenerDescription{{Listener: &elb.Listener{
				Protocol:         awssdk.String("HTTPS"),
				LoadBalancerPort: awssdk.Int64(443),
				InstanceProtocol: awssdk.String("HTTP"),
				InstancePort:     awssdk.Int64(8080),
			}}}
		},
		reflect.TypeOf([]*cloudwatch.Dimension{}): func() interface{} {
			return []*cloudwatch.Dimension{{Name: awssdk.String("InstanceId"), Value: awssdk.String("i-1234")}}
		},
		reflect.TypeOf([]*ecs.Attribute{}): func() interface{} {
			return []*ecs.Attribute{{Name: awssdk.String("ecs.os-type"), Value: awssdk.String("linux")}}
		},
		reflect.TypeOf([]*ecs.ContainerDefinition{}): func() interface{} {
			return []*ecs.ContainerDefinition{{Name: awssdk.String("web"), Image: awssdk.String("nginx:latest")}}
		},
		reflect.TypeOf([]*iam.PolicyVersion{}): func() interface{} {
			return []*iam.PolicyVersion{{
				IsDefaultVersion: awssdk.Bool(true),
				Document:         awssdk.String(url.QueryEscape(`{"Version": "2012-10-17"}`)),
			}}
		},
		reflect.TypeOf([]*cloudformation.Output{}): func() interface{} {
			return []*cloudformation.Output{{OutputKey: awssdk.String("Url"), OutputValue: awssdk.String("https://example.com")}}
		},
		reflect.TypeOf([]*cloudformation.Parameter{}): func() interface{} {
			return []*cloudformation.Parameter{{ParameterKey: awssdk.String("Env"), ParameterValue: awssdk.String("prod")}}
		},
		reflect.TypeOf(&cloudfront.Origins{}): func() interface{} {
			return &cloudfront.Origins{Items: []*cloudfront.Origin{{
				Id:         awssdk.String("origin-1"),
				DomainName: awssdk.String("origin.example.com"),
				OriginPath: awssdk.String("/assets"),
			}}}
		},
		reflect.TypeOf(&cloudfront.Aliases{}): func() interface{} {
			return &cloudfront.Aliases{Items: awssdk.StringSlice([]string{"cdn.example.com"})}
		},
	}
)

// populateMappedFields fills the source fields awsResourcesDef reads for the
// given resource type, so that a missing property can only mean the extraction
// itself failed. Fields the shape does not have are skipped and reported back,
// since a resource type may be built from several shapes.
func populateMappedFields(t *testing.T, rtype string, source interface{}) (absent []string) {
	t.Helper()

	elem := reflect.ValueOf(source).Elem()

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
			t.Errorf("%s: source field %q is not settable", rtype, trans.name)
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
				source := proto()
				absent := populateMappedFields(t, rtype, source)

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
	expected := map[string]bool{
		cloud.Record + ".Zone": true,
	}

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
	source := &ec2.Instance{InstanceId: awssdk.String("i-1234")}
	if absent := populateMappedFields(t, cloud.Instance, source); len(absent) > 0 {
		t.Fatalf("ec2.Instance is missing source fields for %v", absent)
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

	broken, err := NewResource(&ec2.Instance{InstanceId: awssdk.String("i-1234")})
	if err != nil {
		t.Fatalf("NewResource with a broken mapping: %s", err)
	}
	if _, ok := broken.Properties()["Type"]; ok {
		t.Fatal("a mapping pointing at a non-existent source field still produced a property; " +
			"the presence assertions in this file would not catch an SDK field rename")
	}
}
