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

import (
	"context"
	"errors"
	"fmt"
	"os"
	"reflect"
	"strings"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	autoscalingtypes "github.com/aws/aws-sdk-go-v2/service/autoscaling/types"
	cloudwatchtypes "github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
	"github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2"
	elbv2types "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2/types"
	iamtypes "github.com/aws/aws-sdk-go-v2/service/iam/types"
	"github.com/theazz/awless-ro/aws/conv"
	"github.com/theazz/awless-ro/cloud"
	"github.com/theazz/awless-ro/graph"
	"github.com/theazz/awless-ro/triplestore"
)

const (
	PARENT_OF = iota // default
	APPLIES_ON
	DEPENDING_ON
)

type funcBuilder struct {
	parent                              string
	fieldName, listName, stringListName string
	relation                            int
}

type addParentFn func(*graph.Graph, triplestore.RDFGraph, string, interface{}) error

var addParentsFns = map[string][]addParentFn{
	// Infra
	cloud.Subnet: {
		funcBuilder{parent: cloud.Vpc, fieldName: "VpcId"}.build(),
	},
	cloud.Instance: {
		funcBuilder{parent: cloud.Subnet, fieldName: "SubnetId"}.build(),
		funcBuilder{parent: cloud.SecurityGroup, fieldName: "GroupId", listName: "SecurityGroups", relation: APPLIES_ON}.build(),
		funcBuilder{parent: cloud.Keypair, fieldName: "KeyName", relation: APPLIES_ON}.build(),
	},
	cloud.SecurityGroup: {
		funcBuilder{parent: cloud.Vpc, fieldName: "VpcId"}.build(),
	},
	cloud.InternetGateway: {
		addRegionParent,
		funcBuilder{parent: cloud.Vpc, fieldName: "VpcId", listName: "Attachments", relation: DEPENDING_ON}.build(),
	},
	cloud.NatGateway: {
		addRegionParent,
		funcBuilder{parent: cloud.Vpc, fieldName: "VpcId"}.build(),
		funcBuilder{parent: cloud.Subnet, fieldName: "SubnetId", relation: DEPENDING_ON}.build(),
	},
	cloud.RouteTable: {
		funcBuilder{parent: cloud.Subnet, fieldName: "SubnetId", listName: "Associations", relation: DEPENDING_ON}.build(),
		funcBuilder{parent: cloud.Vpc, fieldName: "VpcId"}.build(),
	},
	cloud.Volume: {
		funcBuilder{parent: cloud.AvailabilityZone, fieldName: "AvailabilityZone"}.build(),
		funcBuilder{parent: cloud.Instance, fieldName: "InstanceId", listName: "Attachments", relation: DEPENDING_ON}.build(),
	},
	cloud.ElasticIP: {
		addRegionParent,
		funcBuilder{parent: cloud.Instance, fieldName: "InstanceId", relation: DEPENDING_ON}.build(),
	},
	cloud.Snapshot: {
		addRegionParent,
		funcBuilder{parent: cloud.Volume, fieldName: "VolumeId", relation: DEPENDING_ON}.build(),
	},
	cloud.NetworkInterface: {
		funcBuilder{parent: cloud.Subnet, fieldName: "SubnetId", relation: PARENT_OF}.build(),
		funcBuilder{parent: cloud.SecurityGroup, fieldName: "GroupId", listName: "Groups", relation: APPLIES_ON}.build(),
		funcBuilder{parent: cloud.Instance, fieldName: "Attachment.InstanceId", relation: DEPENDING_ON}.build(),
	},
	// Loadbalancer
	cloud.LoadBalancer: {
		funcBuilder{parent: cloud.Vpc, fieldName: "VpcId"}.build(),
		funcBuilder{parent: cloud.Subnet, fieldName: "SubnetId", listName: "AvailabilityZones", relation: DEPENDING_ON}.build(),
		funcBuilder{parent: cloud.AvailabilityZone, fieldName: "ZoneName", listName: "AvailabilityZones", relation: DEPENDING_ON}.build(),
		funcBuilder{parent: cloud.SecurityGroup, stringListName: "SecurityGroups", relation: APPLIES_ON}.build(),
	},
	cloud.ClassicLoadBalancer: {
		funcBuilder{parent: cloud.Vpc, fieldName: "VPCId"}.build(),
		funcBuilder{parent: cloud.Subnet, stringListName: "Subnets", relation: DEPENDING_ON}.build(),
		funcBuilder{parent: cloud.AvailabilityZone, stringListName: "AvailabilityZones", relation: DEPENDING_ON}.build(),
		funcBuilder{parent: cloud.SecurityGroup, stringListName: "SecurityGroups", relation: APPLIES_ON}.build(),
	},
	cloud.Listener: {
		funcBuilder{parent: cloud.LoadBalancer, fieldName: "LoadBalancerArn"}.build(),
	},
	cloud.TargetGroup: {
		funcBuilder{parent: cloud.Vpc, fieldName: "VpcId"}.build(),
		funcBuilder{parent: cloud.LoadBalancer, stringListName: "LoadBalancerArns", relation: APPLIES_ON}.build(),
		fetchTargetsAndAddRelations,
	},
	// Database
	cloud.Database: {
		funcBuilder{parent: cloud.AvailabilityZone, fieldName: "AvailabilityZone"}.build(),
		funcBuilder{parent: cloud.SecurityGroup, listName: "VpcSecurityGroups", fieldName: "VpcSecurityGroupId", relation: APPLIES_ON}.build(),
	},
	// Autoscaling
	cloud.LaunchConfiguration: {
		addRegionParent,
		funcBuilder{parent: cloud.Keypair, fieldName: "KeyName", relation: APPLIES_ON}.build(),
	},
	cloud.ScalingGroup: {
		addRegionParent,
		funcBuilder{parent: cloud.AvailabilityZone, stringListName: "AvailabilityZones", relation: APPLIES_ON}.build(),
		funcBuilder{parent: cloud.Instance, fieldName: "InstanceId", listName: "Instances", relation: DEPENDING_ON}.build(),
		funcBuilder{parent: cloud.TargetGroup, stringListName: "TargetGroupARNs", relation: DEPENDING_ON}.build(),
		addScalingGroupSubnets,
	},
	// Container
	cloud.ContainerInstance: {
		funcBuilder{parent: cloud.Instance, fieldName: "Ec2InstanceId", relation: APPLIES_ON}.build(),
	},
	cloud.Subscription: {
		funcBuilder{parent: cloud.Topic, fieldName: "TopicArn"}.build(),
	},
	cloud.Vpc:              {addRegionParent},
	cloud.AvailabilityZone: {addRegionParent},
	cloud.Keypair:          {addRegionParent},
	cloud.Image:            {addRegionParent},
	cloud.Repository:       {addRegionParent},
	cloud.ContainerCluster: {addRegionParent},
	cloud.ContainerTask:    {addRegionParent},
	cloud.Certificate:      {addRegionParent},
	cloud.User:             {userAddGroupsRelations, addManagedPoliciesRelations},
	cloud.Role:             {addManagedPoliciesRelations},
	cloud.Group:            {addManagedPoliciesRelations},
	cloud.Bucket:           {addRegionParent},
	cloud.Function:         {addRegionParent},
	cloud.Topic:            {addRegionParent},
	cloud.Alarm:            {addRegionParent, addAlarmMetric},
	cloud.Metric:           {addRegionParent},
	cloud.Stack:            {addRegionParent},
	cloud.MFADevice: {
		funcBuilder{parent: cloud.User, fieldName: "User.UserId", relation: DEPENDING_ON}.build(),
	},
}

func (fb funcBuilder) build() addParentFn {
	switch {
	case fb.listName != "":
		return fb.addRelationListWithField()
	case fb.stringListName != "":
		return fb.addRelationListWithStringField()
	default:
		return fb.addRelationWithField()
	}
}

func (fb funcBuilder) addRelationWithField() addParentFn {
	return func(g *graph.Graph, snap triplestore.RDFGraph, region string, i interface{}) error {
		field, found, err := structFieldByPath(i, fb.fieldName)
		if err != nil {
			return err
		}
		if !found {
			return nil
		}
		id, err := stringFromField(field)
		if err != nil {
			return fmt.Errorf("add parent at %s: %s", fb.fieldName, err)
		}
		if id == "" {
			return nil
		}

		res, err := awsconv.InitResource(i)
		if err != nil {
			return err
		}

		parent := graph.InitResource(fb.parent, id)
		return addRelation(g, parent, res, fb.relation)
	}
}

// structFieldByPath walks a dotted field path such as "Attachment.InstanceId".
// It replaces SDK v1's awsutil.ValuesAtPath, which has no v2 equivalent, and it
// reports a nil link along the way as "not found" rather than as an error.
func structFieldByPath(i interface{}, path string) (reflect.Value, bool, error) {
	value := reflect.ValueOf(i)
	for _, name := range strings.Split(path, ".") {
		if value.Kind() == reflect.Ptr {
			if value.IsNil() {
				return reflect.Value{}, false, nil
			}
			value = value.Elem()
		}
		if value.Kind() != reflect.Struct {
			return reflect.Value{}, false, fmt.Errorf("field path %q: %s is not a struct", path, value.Kind())
		}
		value = value.FieldByName(name)
		if !value.IsValid() {
			return reflect.Value{}, false, fmt.Errorf("field path %q: no field named %q", path, name)
		}
	}
	return value, true, nil
}

// stringFromField reads an identifier out of a field. SDK v2 leaves optional
// scalars as pointers but carries enums as named string types by value, so both
// shapes have to be accepted.
func stringFromField(v reflect.Value) (string, error) {
	if v.Kind() == reflect.Ptr {
		if v.IsNil() {
			return "", nil
		}
		v = v.Elem()
	}
	if v.Kind() != reflect.String {
		return "", fmt.Errorf("%s is not a string", v.Kind())
	}
	return v.String(), nil
}

func (fb funcBuilder) addRelationListWithStringField() addParentFn {
	return func(g *graph.Graph, snap triplestore.RDFGraph, region string, i interface{}) error {
		structField, err := verifyValidStructField(i, fb.stringListName)
		if err != nil {
			return err
		}

		res, err := awsconv.InitResource(i)
		if err != nil {
			return err
		}

		if !structField.IsValid() || structField.Kind() != reflect.Slice {
			return fmt.Errorf("add parent to %s: field not a slice: %T", res.Id(), structField.Kind())
		}

		for i := 0; i < structField.Len(); i++ {
			id, err := stringFromField(structField.Index(i))
			if err != nil {
				return fmt.Errorf("add parent to %s: %s", res.Id(), err)
			}
			if id == "" {
				continue
			}
			parent := graph.InitResource(fb.parent, id)

			if err = addRelation(g, parent, res, fb.relation); err != nil {
				return err
			}
		}
		return nil
	}
}

func (fb funcBuilder) addRelationListWithField() addParentFn {
	return func(g *graph.Graph, snap triplestore.RDFGraph, region string, i interface{}) error {
		structField, err := verifyValidStructField(i, fb.listName)
		if err != nil {
			return err
		}

		res, err := awsconv.InitResource(i)
		if err != nil {
			return err
		}

		if !structField.IsValid() || structField.Kind() != reflect.Slice {
			return fmt.Errorf("add parent to %s: field not a slice: %T", res.Id(), structField.Kind())
		}

		// SDK v2 lists hold shapes by value, where v1 held pointers to them.
		for i := 0; i < structField.Len(); i++ {
			listValue := structField.Index(i)
			if listValue.Kind() == reflect.Ptr {
				if listValue.IsNil() {
					continue
				}
				listValue = listValue.Elem()
			}
			if listValue.Kind() != reflect.Struct {
				return fmt.Errorf("add parent to %s: list element is a %s, not a struct", res.Id(), listValue.Kind())
			}
			listStructField := listValue.FieldByName(fb.fieldName)
			if !listStructField.IsValid() {
				return fmt.Errorf("add parent to %s: no field named %q on %s", res.Id(), fb.fieldName, listValue.Type())
			}
			id, err := stringFromField(listStructField)
			if err != nil {
				return fmt.Errorf("add parent to %s: field %q: %s", res.Id(), fb.fieldName, err)
			}
			if id == "" {
				continue
			}
			parent := graph.InitResource(fb.parent, id)

			if err = addRelation(g, parent, res, fb.relation); err != nil {
				return err
			}
		}
		return nil
	}
}

func verifyValidStructField(i interface{}, name string) (reflect.Value, error) {
	value := reflect.ValueOf(i)
	if value.Kind() == reflect.Ptr {
		if value.IsNil() {
			return reflect.Value{}, fmt.Errorf("%T is a nil pointer", i)
		}
		value = value.Elem()
	}
	if value.Kind() != reflect.Struct {
		return reflect.Value{}, fmt.Errorf("%T is not a struct", i)
	}

	structField := value.FieldByName(name)
	if !structField.IsValid() {
		return reflect.Value{}, fmt.Errorf("invalid field %s: ", name)
	}

	return structField, nil
}

func addRelation(g *graph.Graph, first, other *graph.Resource, relation int) error {
	switch relation {
	case PARENT_OF:
		g.AddParentRelation(first, other)
	case APPLIES_ON:
		g.AddAppliesOnRelation(first, other)
	case DEPENDING_ON:
		g.AddAppliesOnRelation(other, first)
	default:
		return errors.New("unknown relation type")
	}
	return nil
}

func addRegionParent(g *graph.Graph, snap triplestore.RDFGraph, region string, i interface{}) error {
	res, err := awsconv.InitResource(i)
	if err != nil {
		return err
	}
	g.AddParentRelation(graph.InitResource(cloud.Region, region), res)
	return nil
}

func addManagedPoliciesRelations(g *graph.Graph, snap triplestore.RDFGraph, region string, i interface{}) error {
	res, err := awsconv.InitResource(i)
	if err != nil {
		return err
	}
	structField, err := verifyValidStructField(i, "AttachedManagedPolicies")
	if err != nil {
		return fmt.Errorf("add parent to %s: %s", res.Id(), err)
	}
	policies, ok := structField.Interface().([]iamtypes.AttachedPolicy)
	if !ok {
		return fmt.Errorf("add parent to %s: not a valid attached policy list: %T", res.Id(), structField.Interface())
	}

	for _, policy := range policies {
		policies, err := graph.ResolveResourcesWithProp(snap, cloud.Policy, "Name", awssdk.ToString(policy.PolicyName))
		if err != nil {
			return err
		}
		if len(policies) != 1 {
			fmt.Fprintf(os.Stderr, "add parent to '%s/%s': unknown policy named '%s'. Ignoring it.\n", res.Type(), res.Id(), awssdk.ToString(policy.PolicyName))
			return nil
		}
		g.AddAppliesOnRelation(policies[0], res)
	}
	return nil
}

func userAddGroupsRelations(g *graph.Graph, snap triplestore.RDFGraph, region string, i interface{}) error {
	user, ok := i.(iamtypes.UserDetail)
	if !ok {
		return fmt.Errorf("aws fetch: not a user, but a %T", i)
	}
	n, err := awsconv.InitResource(user)
	if err != nil {
		return err
	}

	for _, group := range user.GroupList {
		groupName := group
		resources, err := graph.ResolveResourcesWithProp(snap, cloud.Group, "Name", groupName)
		if err != nil {
			return err
		}
		switch len(resources) {
		case 0:
			fmt.Fprintf(os.Stderr, "no group with name %s found for user %s\n", groupName, n.Id())
		case 1:
			g.AddAppliesOnRelation(resources[0], n)
		default:
			fmt.Fprintf(os.Stderr, "multiple groups with name %s found for user %s:%v\n", groupName, n.Id(), resources)
		}
	}
	return nil
}

func fetchTargetsAndAddRelations(g *graph.Graph, snap triplestore.RDFGraph, region string, i interface{}) error {
	group, ok := i.(elbv2types.TargetGroup)
	if !ok {
		return fmt.Errorf("add targets relation: not a target group, but a %T", i)
	}
	parent, err := awsconv.InitResource(group)
	if err != nil {
		return err
	}

	targets, err := InfraService.(*Infra).DescribeTargetHealth(context.Background(), &elasticloadbalancingv2.DescribeTargetHealthInput{TargetGroupArn: group.TargetGroupArn})
	if err != nil {
		return err
	}

	for _, t := range targets.TargetHealthDescriptions {
		n := graph.InitResource(cloud.Instance, awssdk.ToString(t.Target.Id))
		err = g.AddAppliesOnRelation(parent, n)
		if err != nil {
			return err
		}
	}
	return nil
}

func addScalingGroupSubnets(g *graph.Graph, snap triplestore.RDFGraph, region string, i interface{}) error {
	group, ok := i.(autoscalingtypes.AutoScalingGroup)
	if !ok {
		return fmt.Errorf("add autoscaling group relation: not a autoscaling group, but a %T", i)
	}
	parent, err := awsconv.InitResource(group)
	if err != nil {
		return err
	}
	if subnets := awssdk.ToString(group.VPCZoneIdentifier); subnets != "" {
		splits := strings.Split(subnets, ",")
		for _, split := range splits {
			n := graph.InitResource(cloud.Subnet, split)
			err = g.AddAppliesOnRelation(parent, n)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func addAlarmMetric(g *graph.Graph, snap triplestore.RDFGraph, region string, i interface{}) error {
	alarm, ok := i.(cloudwatchtypes.MetricAlarm)
	if !ok {
		return fmt.Errorf("add alarm metric relation: not a alarm, but a %T", i)
	}
	parent, err := awsconv.InitResource(alarm)
	if err != nil {
		return err
	}
	if namespace, metric := awssdk.ToString(alarm.Namespace), awssdk.ToString(alarm.MetricName); namespace != "" && metric != "" {
		id := awsconv.HashFields(namespace, metric)
		n := graph.InitResource(cloud.Metric, id)
		err = g.AddAppliesOnRelation(parent, n)
		if err != nil {
			return err
		}
	}
	return nil
}
