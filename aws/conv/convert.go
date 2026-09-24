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

// Package awsconv turns AWS SDK shapes into graph resources.
//
// The mapping itself lives in model.go as data; this file is the reflective
// machinery that applies it. AWS SDK v2 mixes three value semantics in the same
// struct, which is what the code below has to cope with:
//
//	Instance.InstanceId    *string              optional scalar, still a pointer
//	Instance.Placement     *Placement           nested shape, still a pointer
//	Instance.InstanceType  InstanceType         enum, a named string by value
//	Instance.Tags          []Tag                list of values, not of pointers
//
// Two consequences drive the design. First, absence cannot be tested with
// IsNil across the board, because calling it on a value kind panics; isAbsent
// dispatches on kind instead. Second, extracted values have to be normalised
// down to builtin types: the triple store matches property values by exact type,
// and a named enum type satisfies neither `case string` nor fmt.Stringer, so an
// un-normalised enum would fail to marshal into the graph.
package awsconv

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"reflect"
	"sync"
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
	"github.com/theazz/awless-ro/cloud/properties"
	"github.com/theazz/awless-ro/graph"
)

// InitResource creates an empty resource of the right type, with the identifier
// AWS uses for that shape.
func InitResource(source interface{}) (*graph.Resource, error) {
	var res *graph.Resource
	switch ss := source.(type) {
	// EC2
	case ec2types.Instance:
		res = graph.InitResource(cloud.Instance, awssdk.ToString(ss.InstanceId))
	case ec2types.Vpc:
		res = graph.InitResource(cloud.Vpc, awssdk.ToString(ss.VpcId))
	case ec2types.Subnet:
		res = graph.InitResource(cloud.Subnet, awssdk.ToString(ss.SubnetId))
	case ec2types.SecurityGroup:
		res = graph.InitResource(cloud.SecurityGroup, awssdk.ToString(ss.GroupId))
	case ec2types.KeyPairInfo:
		res = graph.InitResource(cloud.Keypair, awssdk.ToString(ss.KeyName))
	case ec2types.Volume:
		res = graph.InitResource(cloud.Volume, awssdk.ToString(ss.VolumeId))
	case ec2types.Image:
		res = graph.InitResource(cloud.Image, awssdk.ToString(ss.ImageId))
	case ec2types.ImportImageTask:
		res = graph.InitResource(cloud.ImportImageTask, awssdk.ToString(ss.ImportTaskId))
	case ec2types.InternetGateway:
		res = graph.InitResource(cloud.InternetGateway, awssdk.ToString(ss.InternetGatewayId))
	case ec2types.NatGateway:
		res = graph.InitResource(cloud.NatGateway, awssdk.ToString(ss.NatGatewayId))
	case ec2types.RouteTable:
		res = graph.InitResource(cloud.RouteTable, awssdk.ToString(ss.RouteTableId))
	case ec2types.AvailabilityZone:
		res = graph.InitResource(cloud.AvailabilityZone, awssdk.ToString(ss.ZoneName))
	case ec2types.Address:
		if awssdk.ToString(ss.AllocationId) != "" {
			res = graph.InitResource(cloud.ElasticIP, awssdk.ToString(ss.AllocationId))
		} else {
			res = graph.InitResource(cloud.ElasticIP, awssdk.ToString(ss.PublicIp))
		}
	case ec2types.Snapshot:
		res = graph.InitResource(cloud.Snapshot, awssdk.ToString(ss.SnapshotId))
	case ec2types.NetworkInterface:
		res = graph.InitResource(cloud.NetworkInterface, awssdk.ToString(ss.NetworkInterfaceId))
	// Loadbalancer
	case elbtypes.LoadBalancerDescription:
		res = graph.InitResource(cloud.ClassicLoadBalancer, awssdk.ToString(ss.LoadBalancerName))
	case elbv2types.LoadBalancer:
		res = graph.InitResource(cloud.LoadBalancer, awssdk.ToString(ss.LoadBalancerArn))
	case elbv2types.TargetGroup:
		res = graph.InitResource(cloud.TargetGroup, awssdk.ToString(ss.TargetGroupArn))
	case elbv2types.Listener:
		res = graph.InitResource(cloud.Listener, awssdk.ToString(ss.ListenerArn))
	// Database
	case rdstypes.DBInstance:
		res = graph.InitResource(cloud.Database, awssdk.ToString(ss.DBInstanceIdentifier))
	case rdstypes.DBSubnetGroup:
		res = graph.InitResource(cloud.DbSubnetGroup, awssdk.ToString(ss.DBSubnetGroupArn))
	// Autoscaling
	case autoscalingtypes.LaunchConfiguration:
		res = graph.InitResource(cloud.LaunchConfiguration, awssdk.ToString(ss.LaunchConfigurationARN))
	case autoscalingtypes.AutoScalingGroup:
		res = graph.InitResource(cloud.ScalingGroup, awssdk.ToString(ss.AutoScalingGroupARN))
	case autoscalingtypes.ScalingPolicy:
		res = graph.InitResource(cloud.ScalingPolicy, awssdk.ToString(ss.PolicyARN))
	// Container
	case ecrtypes.Repository:
		res = graph.InitResource(cloud.Repository, awssdk.ToString(ss.RepositoryArn))
	case ecstypes.Cluster:
		res = graph.InitResource(cloud.ContainerCluster, awssdk.ToString(ss.ClusterArn))
	case ecstypes.TaskDefinition:
		res = graph.InitResource(cloud.ContainerTask, awssdk.ToString(ss.TaskDefinitionArn))
	case ecstypes.Container:
		res = graph.InitResource(cloud.Container, awssdk.ToString(ss.ContainerArn))
	case ecstypes.ContainerInstance:
		res = graph.InitResource(cloud.ContainerInstance, awssdk.ToString(ss.ContainerInstanceArn))
	// ACM
	case acmtypes.CertificateSummary:
		res = graph.InitResource(cloud.Certificate, awssdk.ToString(ss.CertificateArn))
	// IAM
	case iamtypes.User:
		res = graph.InitResource(cloud.User, awssdk.ToString(ss.UserId))
	case iamtypes.UserDetail:
		res = graph.InitResource(cloud.User, awssdk.ToString(ss.UserId))
	case iamtypes.RoleDetail:
		res = graph.InitResource(cloud.Role, awssdk.ToString(ss.RoleId))
	case iamtypes.GroupDetail:
		res = graph.InitResource(cloud.Group, awssdk.ToString(ss.GroupId))
	case iamtypes.Policy:
		res = graph.InitResource(cloud.Policy, awssdk.ToString(ss.PolicyId))
	case iamtypes.ManagedPolicyDetail:
		res = graph.InitResource(cloud.Policy, awssdk.ToString(ss.PolicyId))
	case iamtypes.AccessKeyMetadata:
		res = graph.InitResource(cloud.AccessKey, awssdk.ToString(ss.AccessKeyId))
	case iamtypes.InstanceProfile:
		res = graph.InitResource(cloud.InstanceProfile, awssdk.ToString(ss.InstanceProfileId))
	case iamtypes.VirtualMFADevice:
		res = graph.InitResource(cloud.MFADevice, awssdk.ToString(ss.SerialNumber))
	// S3
	case s3types.Bucket:
		res = graph.InitResource(cloud.Bucket, awssdk.ToString(ss.Name))
	case s3types.Object:
		res = graph.InitResource(cloud.S3Object, awssdk.ToString(ss.Key))
	// SNS
	case snstypes.Subscription:
		res = graph.InitResource(cloud.Subscription, awssdk.ToString(ss.Endpoint))
	case snstypes.Topic:
		res = graph.InitResource(cloud.Topic, awssdk.ToString(ss.TopicArn))
	// DNS
	case route53types.HostedZone:
		res = graph.InitResource(cloud.Zone, awssdk.ToString(ss.Id))
	case route53types.ResourceRecordSet:
		id := HashFields(awssdk.ToString(ss.Name), string(ss.Type))
		res = graph.InitResource(cloud.Record, id)
	// Lambda
	case lambdatypes.FunctionConfiguration:
		res = graph.InitResource(cloud.Function, awssdk.ToString(ss.FunctionArn))
	// Monitoring
	case cloudwatchtypes.Metric:
		id := HashFields(awssdk.ToString(ss.Namespace), awssdk.ToString(ss.MetricName))
		res = graph.InitResource(cloud.Metric, id)
	case cloudwatchtypes.MetricAlarm:
		res = graph.InitResource(cloud.Alarm, awssdk.ToString(ss.AlarmArn))
	// CDN
	case cloudfronttypes.DistributionSummary:
		res = graph.InitResource(cloud.Distribution, awssdk.ToString(ss.Id))
	// Cloudformation
	case cloudformationtypes.Stack:
		res = graph.InitResource(cloud.Stack, awssdk.ToString(ss.StackId))
	default:
		return nil, fmt.Errorf("unknown type of resource %T", source)
	}
	return res, nil
}

// NewResource builds a resource and fills in every property model.go maps for
// its type. It accepts the shape either by value, which is how SDK v2 hands
// them over, or behind a pointer.
func NewResource(source interface{}) (*graph.Resource, error) {
	res, err := InitResource(source)
	if err != nil {
		return res, err
	}

	res.Properties()[properties.ID] = res.Id()

	value := reflect.ValueOf(source)
	if value.Kind() == reflect.Ptr {
		if value.IsNil() {
			return nil, fmt.Errorf("cannot fetch cloud resource: %T is a nil pointer", source)
		}
		value = value.Elem()
	}
	if !value.IsValid() || value.Kind() != reflect.Struct {
		return nil, fmt.Errorf("cannot fetch cloud resource: %T is not a struct", source)
	}
	nodeV := value

	type keyValResult struct {
		key string
		val interface{}
	}

	resultc := make(chan keyValResult)
	errc := make(chan error)

	var wg sync.WaitGroup
	for prop, trans := range awsResourcesDef[res.Type()] {
		wg.Add(1)
		go func(p string, t *propertyTransform) {
			defer wg.Done()
			if t.transform != nil {
				sourceField := nodeV.FieldByName(t.name)
				if sourceField.IsValid() && !isAbsent(sourceField) {
					val, err := t.transform(sourceField.Interface())
					if err == ErrTagNotFound {
						return
					}
					if err != nil {
						errc <- fmt.Errorf("type [%s]: prop '%v': %s", res.Type(), p, err)
					}
					resultc <- keyValResult{p, val}
				}
			}
			if t.fetch != nil {
				val, err := t.fetch(source)
				if err != nil {
					errc <- fmt.Errorf("type [%s]: prop '%v': %s", res.Type(), p, err)
				}
				resultc <- keyValResult{p, val}
			}
		}(prop, trans)
	}

	go func() {
		wg.Wait()
		close(errc)
		close(resultc)
	}()

	for {
		select {
		case e := <-errc:
			if e != nil {
				return res, e
			}
		case keyVal, ok := <-resultc:
			if !ok {
				return res, nil
			}
			res.Properties()[keyVal.key] = keyVal.val
		}
	}
}

// isAbsent reports whether a source field carries nothing worth extracting.
// Kinds that cannot be nil are compared against their zero value instead, since
// IsNil panics on them - and in SDK v2 enums and lists of shapes are exactly
// such kinds.
func isAbsent(v reflect.Value) bool {
	switch v.Kind() {
	case reflect.Ptr, reflect.Interface, reflect.Map, reflect.Slice, reflect.Func, reflect.Chan:
		return v.IsNil()
	default:
		return v.IsZero()
	}
}

var ErrTagNotFound = errors.New("aws tag key not found")

type propertyTransform struct {
	name      string
	transform transformFn
	fetch     fetchFn
}

type transformFn func(i interface{}) (interface{}, error)
type fetchFn func(i interface{}) (interface{}, error)

// normalise converts a reflected value to the builtin type the triple store
// understands. AWS SDK v2 enums are named string types, and the store matches
// literals by exact type, so passing one through unchanged means the property
// never reaches the graph.
func normalise(v reflect.Value) (interface{}, bool) {
	switch v.Kind() {
	case reflect.String:
		return v.String(), true
	case reflect.Bool:
		return v.Bool(), true
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return v.Int(), true
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return int64(v.Uint()), true
	case reflect.Float32, reflect.Float64:
		return v.Float(), true
	}
	return nil, false
}

var extractValueFn = func(i interface{}) (interface{}, error) {
	if i == nil {
		return nil, nil
	}
	iv := reflect.ValueOf(i)
	if iv.Kind() == reflect.Ptr {
		if iv.IsNil() {
			return nil, nil
		}
		iv = iv.Elem()
	}

	if v, ok := normalise(iv); ok {
		return v, nil
	}

	switch ii := iv.Interface().(type) {
	case time.Time:
		return ii, nil
	case []string:
		return ii, nil
	}

	if iv.Kind() == reflect.Slice && iv.Type().Elem().Kind() == reflect.String {
		out := make([]string, 0, iv.Len())
		for i := 0; i < iv.Len(); i++ {
			out = append(out, iv.Index(i).String())
		}
		return out, nil
	}

	return nil, fmt.Errorf("extract value: unsupported type %T", i)
}

var extractValueAsStringFn = func(i interface{}) (interface{}, error) {
	val, err := extractValueFn(i)
	return fmt.Sprint(val), err
}

// Extract time forcing timezone to UTC (friendlier when running tests in
// different timezones)
var extractTimeFn = func(i interface{}) (interface{}, error) {
	switch t := i.(type) {
	case time.Time:
		return t.UTC(), nil
	case *time.Time:
		if t == nil {
			return nil, nil
		}
		return t.UTC(), nil
	case *string:
		parsed, err := time.Parse("2006-01-02T15:04:05.000+0000", awssdk.ToString(t))
		if err != nil {
			return nil, err
		}
		return parsed.UTC(), nil
	case string:
		parsed, err := time.Parse("2006-01-02T15:04:05.000+0000", t)
		if err != nil {
			return nil, err
		}
		return parsed.UTC(), nil
	}
	return nil, fmt.Errorf("extract time: expected a time or a time string, got: %T", i)
}

// Extract time that has a Z directly after the time, which means UTC
// (https://en.wikipedia.org/wiki/ISO_8601#UTC)
var extractTimeWithZSuffixFn = func(i interface{}) (interface{}, error) {
	switch t := i.(type) {
	case time.Time:
		return t.UTC(), nil
	case *time.Time:
		if t == nil {
			return nil, nil
		}
		return t.UTC(), nil
	case *string:
		return time.Parse("2006-01-02T15:04:05.000Z", awssdk.ToString(t))
	case string:
		return time.Parse("2006-01-02T15:04:05.000Z", t)
	}
	return nil, fmt.Errorf("extract time: expected a time or a time string, got: %T", i)
}

var extractIpPermissionSliceFn = func(i interface{}) (interface{}, error) {
	perms, ok := i.([]ec2types.IpPermission)
	if !ok {
		return nil, fmt.Errorf("extract ip permission: not a permission slice but a %T", i)
	}
	var rules []*graph.FirewallRule
	for _, ipPerm := range perms {
		rule := &graph.FirewallRule{}

		protocol := awssdk.ToString(ipPerm.IpProtocol)
		switch protocol {
		case "-1":
			rule.Protocol = "any"
			rule.PortRange = graph.PortRange{Any: true}
		case "tcp", "udp", "icmp", "58":
			rule.Protocol = protocol
			fromPort := int64(awssdk.ToInt32(ipPerm.FromPort))
			toPort := int64(awssdk.ToInt32(ipPerm.ToPort))
			if fromPort == -1 || toPort == -1 {
				rule.PortRange = graph.PortRange{Any: true}
			} else {
				rule.PortRange = graph.PortRange{FromPort: fromPort, ToPort: toPort}
			}
		default:
			rule.Protocol = protocol
			rule.PortRange = graph.PortRange{Any: true}
		}
		for _, r := range ipPerm.IpRanges {
			_, net, err := net.ParseCIDR(awssdk.ToString(r.CidrIp))
			if err != nil {
				return rules, err
			}
			rule.IPRanges = append(rule.IPRanges, net)
		}
		for _, r := range ipPerm.Ipv6Ranges {
			_, net, err := net.ParseCIDR(awssdk.ToString(r.CidrIpv6))
			if err != nil {
				return rules, err
			}
			rule.IPRanges = append(rule.IPRanges, net)
		}
		for _, group := range ipPerm.UserIdGroupPairs {
			rule.Sources = append(rule.Sources, awssdk.ToString(group.GroupId))
		}

		rules = append(rules, rule)
	}
	return rules, nil
}

var extractNameValueFn = func(i interface{}) (interface{}, error) {
	dimensions, ok := i.([]cloudwatchtypes.Dimension)
	if !ok {
		return nil, fmt.Errorf("extract name value: not a dimension slice but a %T", i)
	}
	var nameValues []*graph.KeyValue
	for _, dimension := range dimensions {
		nameValues = append(nameValues, &graph.KeyValue{
			KeyName: awssdk.ToString(dimension.Name),
			Value:   awssdk.ToString(dimension.Value),
		})
	}
	return nameValues, nil
}

var extractECSAttributesFn = func(i interface{}) (interface{}, error) {
	attributes, ok := i.([]ecstypes.Attribute)
	if !ok {
		return nil, fmt.Errorf("extract ECS attributes: not an attribute slice but a %T", i)
	}
	var keyVals []*graph.KeyValue
	for _, attribute := range attributes {
		keyVals = append(keyVals, &graph.KeyValue{
			KeyName: awssdk.ToString(attribute.Name),
			Value:   awssdk.ToString(attribute.Value),
		})
	}
	return keyVals, nil
}

var extractRouteTableAssociationsFn = func(i interface{}) (interface{}, error) {
	assocs, ok := i.([]ec2types.RouteTableAssociation)
	if !ok {
		return nil, fmt.Errorf("extract route table associations: not an association slice but a %T", i)
	}
	var keyVals []*graph.KeyValue
	for _, assoc := range assocs {
		keyVals = append(keyVals, &graph.KeyValue{
			KeyName: awssdk.ToString(assoc.RouteTableAssociationId),
			Value:   awssdk.ToString(assoc.SubnetId),
		})
	}
	return keyVals, nil
}

// extractFieldFn reads one field of a nested shape. SDK v2 keeps nested shapes
// behind pointers but puts lists of shapes in by value, so both have to work.
var extractFieldFn = func(field string) transformFn {
	return func(i interface{}) (interface{}, error) {
		value := reflect.ValueOf(i)
		if value.Kind() == reflect.Ptr {
			if value.IsNil() {
				return nil, nil
			}
			value = value.Elem()
		}
		if value.Kind() != reflect.Struct {
			return nil, fmt.Errorf("extract field '%s': not a struct or a pointer to one but a %T", field, i)
		}

		structField := value.FieldByName(field)
		if !structField.IsValid() {
			return nil, fmt.Errorf("extract field: field not found: %s", field)
		}

		return extractValueFn(structField.Interface())
	}
}

var extractTagsFn = func(i interface{}) (interface{}, error) {
	var out []string
	switch tags := i.(type) {
	case []ec2types.Tag:
		for _, t := range tags {
			out = append(out, fmt.Sprintf("%s=%s", awssdk.ToString(t.Key), awssdk.ToString(t.Value)))
		}
	case []autoscalingtypes.TagDescription:
		for _, t := range tags {
			out = append(out, fmt.Sprintf("%s=%s", awssdk.ToString(t.Key), awssdk.ToString(t.Value)))
		}
	default:
		return nil, fmt.Errorf("extract tags: not a tag slice, but a %T", i)
	}

	return out, nil
}

var extractTagFn = func(key string) transformFn {
	return func(i interface{}) (interface{}, error) {
		tags, ok := i.([]ec2types.Tag)
		if !ok {
			return nil, fmt.Errorf("extract tag: not a tag slice, but a %T", i)
		}
		for _, t := range tags {
			if key == awssdk.ToString(t.Key) {
				return awssdk.ToString(t.Value), nil
			}
		}

		return nil, ErrTagNotFound
	}
}

// extractStringPointerSliceValues survives from the SDK v1 layout, where string
// lists were []*string. In v2 they are []string, and sometimes a slice of a
// named string type such as []cloudformationtypes.Capability, so the element is
// read reflectively rather than by type assertion.
var extractStringPointerSliceValues = func(i interface{}) (interface{}, error) {
	v := reflect.ValueOf(i)
	if v.Kind() != reflect.Slice {
		return nil, fmt.Errorf("extract string slice: not a slice but a %T", i)
	}

	out := make([]string, 0, v.Len())
	for idx := 0; idx < v.Len(); idx++ {
		elem := v.Index(idx)
		if elem.Kind() == reflect.Ptr {
			if elem.IsNil() {
				out = append(out, "")
				continue
			}
			elem = elem.Elem()
		}
		if elem.Kind() != reflect.String {
			return nil, fmt.Errorf("extract string slice: element %d is a %s, not a string", idx, elem.Kind())
		}
		out = append(out, elem.String())
	}
	return out, nil
}

var extractStringSliceValues = func(key string) transformFn {
	return func(i interface{}) (interface{}, error) {
		var res []string
		value := reflect.ValueOf(i)
		if value.Kind() != reflect.Slice {
			return nil, fmt.Errorf("extract slice: not a slice but a %T", i)
		}
		for idx := 0; idx < value.Len(); idx++ {
			e, err := extractFieldFn(key)(value.Index(idx).Interface())
			if err != nil {
				return nil, err
			}
			if e == nil {
				continue
			}
			str, ok := e.(string)
			if !ok {
				return nil, fmt.Errorf("extract string slice: not a string but a %T", e)
			}
			res = append(res, str)
		}

		return res, nil
	}
}

var extractClassicLoadbListenerDescriptionsFn = func(i interface{}) (interface{}, error) {
	listeners, ok := i.([]elbtypes.ListenerDescription)
	if !ok {
		return nil, fmt.Errorf("extract classic loadb listener descriptions: unexpected type %T", i)
	}
	var out []string
	for _, d := range listeners {
		if list := d.Listener; list != nil {
			// LoadBalancerPort is a required member and so comes by value, while
			// InstancePort is optional and stays behind a pointer.
			out = append(out, fmt.Sprintf("%s:%d:%s:%d",
				awssdk.ToString(list.Protocol),
				list.LoadBalancerPort,
				awssdk.ToString(list.InstanceProtocol),
				awssdk.ToInt32(list.InstancePort),
			))
		}
	}
	return out, nil
}

var extractRoutesSliceFn = func(i interface{}) (interface{}, error) {
	awsRoutes, ok := i.([]ec2types.Route)
	if !ok {
		return nil, fmt.Errorf("extract route: not a route slice but a %T", i)
	}
	var routes []*graph.Route
	for _, r := range awsRoutes {
		route := &graph.Route{}
		var err error
		if notEmpty(r.DestinationCidrBlock) {
			if _, route.Destination, err = net.ParseCIDR(awssdk.ToString(r.DestinationCidrBlock)); err != nil {
				return nil, err
			}
		}
		if notEmpty(r.DestinationIpv6CidrBlock) {
			if _, route.DestinationIPv6, err = net.ParseCIDR(awssdk.ToString(r.DestinationIpv6CidrBlock)); err != nil {
				return nil, err
			}
		}
		if notEmpty(r.DestinationPrefixListId) {
			route.DestinationPrefixListId = awssdk.ToString(r.DestinationPrefixListId)
		}
		if notEmpty(r.EgressOnlyInternetGatewayId) {
			route.Targets = append(route.Targets, &graph.RouteTarget{
				Type: graph.EgressOnlyInternetGatewayTarget, Ref: awssdk.ToString(r.EgressOnlyInternetGatewayId),
			})
		}
		if notEmpty(r.GatewayId) {
			route.Targets = append(route.Targets, &graph.RouteTarget{
				Type: graph.GatewayTarget, Ref: awssdk.ToString(r.GatewayId),
			})
		}
		if notEmpty(r.InstanceId) {
			route.Targets = append(route.Targets, &graph.RouteTarget{
				Type: graph.InstanceTarget, Ref: awssdk.ToString(r.InstanceId), Owner: awssdk.ToString(r.InstanceOwnerId),
			})
		}
		if notEmpty(r.NatGatewayId) {
			route.Targets = append(route.Targets, &graph.RouteTarget{
				Type: graph.NatTarget, Ref: awssdk.ToString(r.NatGatewayId),
			})
		}
		if notEmpty(r.NetworkInterfaceId) {
			route.Targets = append(route.Targets, &graph.RouteTarget{
				Type: graph.NetworkInterfaceTarget, Ref: awssdk.ToString(r.NetworkInterfaceId),
			})
		}
		if notEmpty(r.VpcPeeringConnectionId) {
			route.Targets = append(route.Targets, &graph.RouteTarget{
				Type: graph.VpcPeeringConnectionTarget, Ref: awssdk.ToString(r.VpcPeeringConnectionId),
			})
		}
		routes = append(routes, route)
	}
	return routes, nil
}

var extractHasATrueBoolInStructSliceFn = func(key string) transformFn {
	return func(i interface{}) (interface{}, error) {
		var res bool
		value := reflect.ValueOf(i)
		if value.Kind() != reflect.Slice {
			return nil, fmt.Errorf("extract true bool: not a slice but a %T", i)
		}
		for idx := 0; idx < value.Len(); idx++ {
			e, err := extractFieldFn(key)(value.Index(idx).Interface())
			if err != nil {
				return res, err
			}
			if e == nil {
				continue // empty field
			}
			b, ok := e.(bool)
			if !ok {
				return nil, fmt.Errorf("extract true bool: the field %s is not a boolean, but has type: %T", key, e)
			}
			if b {
				res = true
			}
		}
		return res, nil
	}
}

var extractDistributionOriginFn = func(i interface{}) (interface{}, error) {
	awsOrigins, ok := i.(*cloudfronttypes.Origins)
	if !ok {
		return nil, fmt.Errorf("extract origins: not an origins pointer but a %T", i)
	}
	var origins []*graph.DistributionOrigin
	for _, o := range awsOrigins.Items {
		origin := &graph.DistributionOrigin{
			ID:         awssdk.ToString(o.Id),
			PublicDNS:  awssdk.ToString(o.DomainName),
			PathPrefix: awssdk.ToString(o.OriginPath),
		}
		if o.S3OriginConfig != nil && awssdk.ToString(o.S3OriginConfig.OriginAccessIdentity) != "" {
			origin.OriginType = "s3"
			origin.Config = awssdk.ToString(o.S3OriginConfig.OriginAccessIdentity)
		}

		origins = append(origins, origin)
	}
	return origins, nil
}

var extractStackOutputsFn = func(i interface{}) (interface{}, error) {
	outputs, ok := i.([]cloudformationtypes.Output)
	if !ok {
		return nil, fmt.Errorf("extract outputs: not an output slice but a %T", i)
	}
	var keyVals []*graph.KeyValue
	for _, out := range outputs {
		keyVals = append(keyVals, &graph.KeyValue{
			KeyName: awssdk.ToString(out.OutputKey),
			Value:   awssdk.ToString(out.OutputValue),
		})
	}
	return keyVals, nil
}

var extractStackParametersFn = func(i interface{}) (interface{}, error) {
	params, ok := i.([]cloudformationtypes.Parameter)
	if !ok {
		return nil, fmt.Errorf("extract parameters: not a parameter slice but a %T", i)
	}
	var keyVals []*graph.KeyValue
	for _, out := range params {
		keyVals = append(keyVals, &graph.KeyValue{
			KeyName: awssdk.ToString(out.ParameterKey),
			Value:   awssdk.ToString(out.ParameterValue),
		})
	}
	return keyVals, nil
}

var extractContainersImagesFn = func(i interface{}) (interface{}, error) {
	definitions, ok := i.([]ecstypes.ContainerDefinition)
	if !ok {
		return nil, fmt.Errorf("extract containers images: not a container definition slice but a %T", i)
	}
	var keyVals []*graph.KeyValue
	for _, out := range definitions {
		keyVals = append(keyVals, &graph.KeyValue{
			KeyName: awssdk.ToString(out.Name),
			Value:   awssdk.ToString(out.Image),
		})
	}
	return keyVals, nil
}

func extractDocumentDefaultVersion(i interface{}) (interface{}, error) {
	versions, ok := i.([]iamtypes.PolicyVersion)
	if !ok {
		return nil, fmt.Errorf("extract default version of document: not a policy version slice but a %T", i)
	}
	for _, version := range versions {
		if version.IsDefaultVersion {
			docStr := awssdk.ToString(version.Document)
			if str, err := url.QueryUnescape(docStr); err == nil {
				var buff bytes.Buffer
				err = json.Compact(&buff, []byte(str))
				return buff.String(), err
			}
			return docStr, nil
		}
	}
	return "", nil
}

func extractURLEncodedJson(i interface{}) (interface{}, error) {
	s, ok := i.(*string)
	if !ok {
		return nil, fmt.Errorf("extract URL-encoded JSON: not a *string but a %T", i)
	}
	docStr := awssdk.ToString(s)
	if str, err := url.QueryUnescape(docStr); err == nil {
		var buff bytes.Buffer
		err = json.Compact(&buff, []byte(str))
		return buff.String(), err
	}
	return docStr, nil
}

func notEmpty(str *string) bool {
	return awssdk.ToString(str) != ""
}

// HashFields builds an identifier for a resource AWS gives no id of its own: a DNS
// record, which is identified by its name and type together, and a CloudWatch metric,
// by its namespace and name. The result is the node's identity in the graph, so two
// different resources hashing alike do not merely look similar, they merge.
//
// Two things here are deliberate and both were wrong before.
//
// Fields are separated by a NUL, which cannot appear in an AWS name. Without one the
// fields were concatenated, so HashFields("ab", "c") and HashFields("a", "bc") were
// the same value — and for metrics that is not a contrivance, because both fields are
// free-form: the namespace AWS/EC2 with metric CPUUtilization collided with the
// namespace AWS/EC2C and metric PUUtilization.
//
// The digest is SHA-256 truncated rather than adler32. adler32 is a checksum meant to
// detect damage in a stream, gives 32 bits, and disperses short strings particularly
// badly; the two together made accidental collisions likelier than the identifier
// length suggested. Six bytes is ample here: these are per-account resource counts,
// not internet scale.
func HashFields(fields ...interface{}) string {
	h := sha256.New()
	for _, field := range fields {
		fmt.Fprintf(h, "%v\x00", field)
	}
	return "awls-" + hex.EncodeToString(h.Sum(nil)[:6])
}
