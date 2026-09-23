// Auto generated implementation for the AWS cloud service

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

import (
	"context"
	"errors"
	"sync"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
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
	"github.com/aws/aws-sdk-go-v2/service/ecs"
	ecstypes "github.com/aws/aws-sdk-go-v2/service/ecs/types"
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
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/aws-sdk-go-v2/service/sns"
	snstypes "github.com/aws/aws-sdk-go-v2/service/sns/types"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/aws/aws-sdk-go-v2/service/sts"

	"github.com/theazz/awless-ro/aws/fetch"
	"github.com/theazz/awless-ro/cloud"
	"github.com/theazz/awless-ro/fetch"
	"github.com/theazz/awless-ro/graph"
	"github.com/theazz/awless-ro/logger"
	tstore "github.com/wallix/triplestore"
)

var ServiceNames = []string{
	"infra",
	"access",
	"storage",
	"messaging",
	"dns",
	"lambda",
	"monitoring",
	"cdn",
	"cloudformation",
}

var ResourceTypes = []string{
	"instance",
	"subnet",
	"vpc",
	"keypair",
	"securitygroup",
	"volume",
	"internetgateway",
	"natgateway",
	"routetable",
	"availabilityzone",
	"image",
	"importimagetask",
	"elasticip",
	"snapshot",
	"networkinterface",
	"classicloadbalancer",
	"loadbalancer",
	"targetgroup",
	"listener",
	"database",
	"dbsubnetgroup",
	"launchconfiguration",
	"scalinggroup",
	"scalingpolicy",
	"repository",
	"containercluster",
	"containertask",
	"container",
	"containerinstance",
	"certificate",
	"user",
	"group",
	"role",
	"policy",
	"accesskey",
	"instanceprofile",
	"mfadevice",
	"bucket",
	"s3object",
	"subscription",
	"topic",
	"queue",
	"zone",
	"record",
	"function",
	"metric",
	"alarm",
	"distribution",
	"stack",
}

var ServicePerAPI = map[string]string{
	"ec2":            "infra",
	"elbv2":          "infra",
	"elb":            "infra",
	"rds":            "infra",
	"autoscaling":    "infra",
	"ecr":            "infra",
	"ecs":            "infra",
	"acm":            "infra",
	"iam":            "access",
	"sts":            "access",
	"s3":             "storage",
	"sns":            "messaging",
	"sqs":            "messaging",
	"route53":        "dns",
	"lambda":         "lambda",
	"cloudwatch":     "monitoring",
	"cloudfront":     "cdn",
	"cloudformation": "cloudformation",
}

var ServicePerResourceType = map[string]string{
	"instance":            "infra",
	"subnet":              "infra",
	"vpc":                 "infra",
	"keypair":             "infra",
	"securitygroup":       "infra",
	"volume":              "infra",
	"internetgateway":     "infra",
	"natgateway":          "infra",
	"routetable":          "infra",
	"availabilityzone":    "infra",
	"image":               "infra",
	"importimagetask":     "infra",
	"elasticip":           "infra",
	"snapshot":            "infra",
	"networkinterface":    "infra",
	"classicloadbalancer": "infra",
	"loadbalancer":        "infra",
	"targetgroup":         "infra",
	"listener":            "infra",
	"database":            "infra",
	"dbsubnetgroup":       "infra",
	"launchconfiguration": "infra",
	"scalinggroup":        "infra",
	"scalingpolicy":       "infra",
	"repository":          "infra",
	"containercluster":    "infra",
	"containertask":       "infra",
	"container":           "infra",
	"containerinstance":   "infra",
	"certificate":         "infra",
	"user":                "access",
	"group":               "access",
	"role":                "access",
	"policy":              "access",
	"accesskey":           "access",
	"instanceprofile":     "access",
	"mfadevice":           "access",
	"bucket":              "storage",
	"s3object":            "storage",
	"subscription":        "messaging",
	"topic":               "messaging",
	"queue":               "messaging",
	"zone":                "dns",
	"record":              "dns",
	"function":            "lambda",
	"metric":              "monitoring",
	"alarm":               "monitoring",
	"distribution":        "cdn",
	"stack":               "cloudformation",
}

var APIPerResourceType = map[string]string{
	"instance":            "ec2",
	"subnet":              "ec2",
	"vpc":                 "ec2",
	"keypair":             "ec2",
	"securitygroup":       "ec2",
	"volume":              "ec2",
	"internetgateway":     "ec2",
	"natgateway":          "ec2",
	"routetable":          "ec2",
	"availabilityzone":    "ec2",
	"image":               "ec2",
	"importimagetask":     "ec2",
	"elasticip":           "ec2",
	"snapshot":            "ec2",
	"networkinterface":    "ec2",
	"classicloadbalancer": "elb",
	"loadbalancer":        "elbv2",
	"targetgroup":         "elbv2",
	"listener":            "elbv2",
	"database":            "rds",
	"dbsubnetgroup":       "rds",
	"launchconfiguration": "autoscaling",
	"scalinggroup":        "autoscaling",
	"scalingpolicy":       "autoscaling",
	"repository":          "ecr",
	"containercluster":    "ecs",
	"containertask":       "ecs",
	"container":           "ecs",
	"containerinstance":   "ecs",
	"certificate":         "acm",
	"user":                "iam",
	"group":               "iam",
	"role":                "iam",
	"policy":              "iam",
	"accesskey":           "iam",
	"instanceprofile":     "iam",
	"mfadevice":           "iam",
	"bucket":              "s3",
	"s3object":            "s3",
	"subscription":        "sns",
	"topic":               "sns",
	"queue":               "sqs",
	"zone":                "route53",
	"record":              "route53",
	"function":            "lambda",
	"metric":              "cloudwatch",
	"alarm":               "cloudwatch",
	"distribution":        "cloudfront",
	"stack":               "cloudformation",
}

type Infra struct {
	fetcher         fetch.Fetcher
	region, profile string
	config          map[string]interface{}
	log             *logger.Logger
	awsfetch.Ec2API
	awsfetch.Elbv2API
	awsfetch.ElbAPI
	awsfetch.RdsAPI
	awsfetch.AutoscalingAPI
	awsfetch.EcrAPI
	awsfetch.EcsAPI
	awsfetch.AcmAPI
}

func NewInfra(cfg awssdk.Config, profile string, extraConf map[string]interface{}, log *logger.Logger) cloud.Service {
	region := cfg.Region
	ec2API := ec2.NewFromConfig(cfg)
	elbv2API := elasticloadbalancingv2.NewFromConfig(cfg)
	elbAPI := elasticloadbalancing.NewFromConfig(cfg)
	rdsAPI := rds.NewFromConfig(cfg)
	autoscalingAPI := autoscaling.NewFromConfig(cfg)
	ecrAPI := ecr.NewFromConfig(cfg)
	ecsAPI := ecs.NewFromConfig(cfg)
	acmAPI := acm.NewFromConfig(cfg)

	fetchConfig := awsfetch.NewConfig(&awsfetch.AWSAPI{
		Ec2:         ec2API,
		Elbv2:       elbv2API,
		Elb:         elbAPI,
		Rds:         rdsAPI,
		Autoscaling: autoscalingAPI,
		Ecr:         ecrAPI,
		Ecs:         ecsAPI,
		Acm:         acmAPI,
	})
	fetchConfig.Extra = extraConf
	fetchConfig.Log = log

	return &Infra{
		Ec2API:         ec2API,
		Elbv2API:       elbv2API,
		ElbAPI:         elbAPI,
		RdsAPI:         rdsAPI,
		AutoscalingAPI: autoscalingAPI,
		EcrAPI:         ecrAPI,
		EcsAPI:         ecsAPI,
		AcmAPI:         acmAPI,
		fetcher:        fetch.NewFetcher(awsfetch.BuildInfraFetchFuncs(fetchConfig)),
		config:         extraConf,
		region:         region,
		profile:        profile,
		log:            log,
	}
}

func (s *Infra) Name() string {
	return "infra"
}

func (s *Infra) Region() string {
	return s.region
}

func (s *Infra) Profile() string {
	return s.profile
}

func (s *Infra) ResourceTypes() []string {
	return []string{
		"instance",
		"subnet",
		"vpc",
		"keypair",
		"securitygroup",
		"volume",
		"internetgateway",
		"natgateway",
		"routetable",
		"availabilityzone",
		"image",
		"importimagetask",
		"elasticip",
		"snapshot",
		"networkinterface",
		"classicloadbalancer",
		"loadbalancer",
		"targetgroup",
		"listener",
		"database",
		"dbsubnetgroup",
		"launchconfiguration",
		"scalinggroup",
		"scalingpolicy",
		"repository",
		"containercluster",
		"containertask",
		"container",
		"containerinstance",
		"certificate",
	}
}

func (s *Infra) Fetch(ctx context.Context) (cloud.GraphAPI, error) {
	if s.IsSyncDisabled() {
		return graph.NewGraph(), nil
	}

	allErrors := new(fetch.Error)

	gph, err := s.fetcher.Fetch(context.WithValue(ctx, "region", s.region))
	defer s.fetcher.Reset()

	for _, e := range *fetch.WrapError(err) {
		switch {
		case e == nil:
			continue
		case awsfetch.IsAccessDenied(e):
			allErrors.Add(cloud.ErrFetchAccessDenied)
		default:
			allErrors.Add(e)
		}
	}

	if err := gph.AddResource(graph.InitResource(cloud.Region, s.region)); err != nil {
		return gph, err
	}

	snap := gph.AsRDFGraphSnaphot()

	errc := make(chan error)
	var wg sync.WaitGroup
	if getBool(s.config, "aws.infra.instance.sync", true) {
		list, err := s.fetcher.Get("instance_objects")
		if err != nil {
			return gph, err
		}
		objects, ok := list.([]ec2types.Instance)
		if !ok {
			return gph, errors.New("cannot cast to '[]ec2types.Instance' type from fetch context")
		}
		for _, r := range objects {
			for _, fn := range addParentsFns["instance"] {
				wg.Add(1)
				go func(f addParentFn, snap tstore.RDFGraph, region string, res ec2types.Instance) {
					defer wg.Done()
					if err := f(gph, snap, region, res); err != nil {
						errc <- err
					}
				}(fn, snap, s.region, r)
			}
		}
	}
	if getBool(s.config, "aws.infra.subnet.sync", true) {
		list, err := s.fetcher.Get("subnet_objects")
		if err != nil {
			return gph, err
		}
		objects, ok := list.([]ec2types.Subnet)
		if !ok {
			return gph, errors.New("cannot cast to '[]ec2types.Subnet' type from fetch context")
		}
		for _, r := range objects {
			for _, fn := range addParentsFns["subnet"] {
				wg.Add(1)
				go func(f addParentFn, snap tstore.RDFGraph, region string, res ec2types.Subnet) {
					defer wg.Done()
					if err := f(gph, snap, region, res); err != nil {
						errc <- err
					}
				}(fn, snap, s.region, r)
			}
		}
	}
	if getBool(s.config, "aws.infra.vpc.sync", true) {
		list, err := s.fetcher.Get("vpc_objects")
		if err != nil {
			return gph, err
		}
		objects, ok := list.([]ec2types.Vpc)
		if !ok {
			return gph, errors.New("cannot cast to '[]ec2types.Vpc' type from fetch context")
		}
		for _, r := range objects {
			for _, fn := range addParentsFns["vpc"] {
				wg.Add(1)
				go func(f addParentFn, snap tstore.RDFGraph, region string, res ec2types.Vpc) {
					defer wg.Done()
					if err := f(gph, snap, region, res); err != nil {
						errc <- err
					}
				}(fn, snap, s.region, r)
			}
		}
	}
	if getBool(s.config, "aws.infra.keypair.sync", true) {
		list, err := s.fetcher.Get("keypair_objects")
		if err != nil {
			return gph, err
		}
		objects, ok := list.([]ec2types.KeyPairInfo)
		if !ok {
			return gph, errors.New("cannot cast to '[]ec2types.KeyPairInfo' type from fetch context")
		}
		for _, r := range objects {
			for _, fn := range addParentsFns["keypair"] {
				wg.Add(1)
				go func(f addParentFn, snap tstore.RDFGraph, region string, res ec2types.KeyPairInfo) {
					defer wg.Done()
					if err := f(gph, snap, region, res); err != nil {
						errc <- err
					}
				}(fn, snap, s.region, r)
			}
		}
	}
	if getBool(s.config, "aws.infra.securitygroup.sync", true) {
		list, err := s.fetcher.Get("securitygroup_objects")
		if err != nil {
			return gph, err
		}
		objects, ok := list.([]ec2types.SecurityGroup)
		if !ok {
			return gph, errors.New("cannot cast to '[]ec2types.SecurityGroup' type from fetch context")
		}
		for _, r := range objects {
			for _, fn := range addParentsFns["securitygroup"] {
				wg.Add(1)
				go func(f addParentFn, snap tstore.RDFGraph, region string, res ec2types.SecurityGroup) {
					defer wg.Done()
					if err := f(gph, snap, region, res); err != nil {
						errc <- err
					}
				}(fn, snap, s.region, r)
			}
		}
	}
	if getBool(s.config, "aws.infra.volume.sync", true) {
		list, err := s.fetcher.Get("volume_objects")
		if err != nil {
			return gph, err
		}
		objects, ok := list.([]ec2types.Volume)
		if !ok {
			return gph, errors.New("cannot cast to '[]ec2types.Volume' type from fetch context")
		}
		for _, r := range objects {
			for _, fn := range addParentsFns["volume"] {
				wg.Add(1)
				go func(f addParentFn, snap tstore.RDFGraph, region string, res ec2types.Volume) {
					defer wg.Done()
					if err := f(gph, snap, region, res); err != nil {
						errc <- err
					}
				}(fn, snap, s.region, r)
			}
		}
	}
	if getBool(s.config, "aws.infra.internetgateway.sync", true) {
		list, err := s.fetcher.Get("internetgateway_objects")
		if err != nil {
			return gph, err
		}
		objects, ok := list.([]ec2types.InternetGateway)
		if !ok {
			return gph, errors.New("cannot cast to '[]ec2types.InternetGateway' type from fetch context")
		}
		for _, r := range objects {
			for _, fn := range addParentsFns["internetgateway"] {
				wg.Add(1)
				go func(f addParentFn, snap tstore.RDFGraph, region string, res ec2types.InternetGateway) {
					defer wg.Done()
					if err := f(gph, snap, region, res); err != nil {
						errc <- err
					}
				}(fn, snap, s.region, r)
			}
		}
	}
	if getBool(s.config, "aws.infra.natgateway.sync", true) {
		list, err := s.fetcher.Get("natgateway_objects")
		if err != nil {
			return gph, err
		}
		objects, ok := list.([]ec2types.NatGateway)
		if !ok {
			return gph, errors.New("cannot cast to '[]ec2types.NatGateway' type from fetch context")
		}
		for _, r := range objects {
			for _, fn := range addParentsFns["natgateway"] {
				wg.Add(1)
				go func(f addParentFn, snap tstore.RDFGraph, region string, res ec2types.NatGateway) {
					defer wg.Done()
					if err := f(gph, snap, region, res); err != nil {
						errc <- err
					}
				}(fn, snap, s.region, r)
			}
		}
	}
	if getBool(s.config, "aws.infra.routetable.sync", true) {
		list, err := s.fetcher.Get("routetable_objects")
		if err != nil {
			return gph, err
		}
		objects, ok := list.([]ec2types.RouteTable)
		if !ok {
			return gph, errors.New("cannot cast to '[]ec2types.RouteTable' type from fetch context")
		}
		for _, r := range objects {
			for _, fn := range addParentsFns["routetable"] {
				wg.Add(1)
				go func(f addParentFn, snap tstore.RDFGraph, region string, res ec2types.RouteTable) {
					defer wg.Done()
					if err := f(gph, snap, region, res); err != nil {
						errc <- err
					}
				}(fn, snap, s.region, r)
			}
		}
	}
	if getBool(s.config, "aws.infra.availabilityzone.sync", true) {
		list, err := s.fetcher.Get("availabilityzone_objects")
		if err != nil {
			return gph, err
		}
		objects, ok := list.([]ec2types.AvailabilityZone)
		if !ok {
			return gph, errors.New("cannot cast to '[]ec2types.AvailabilityZone' type from fetch context")
		}
		for _, r := range objects {
			for _, fn := range addParentsFns["availabilityzone"] {
				wg.Add(1)
				go func(f addParentFn, snap tstore.RDFGraph, region string, res ec2types.AvailabilityZone) {
					defer wg.Done()
					if err := f(gph, snap, region, res); err != nil {
						errc <- err
					}
				}(fn, snap, s.region, r)
			}
		}
	}
	if getBool(s.config, "aws.infra.image.sync", true) {
		list, err := s.fetcher.Get("image_objects")
		if err != nil {
			return gph, err
		}
		objects, ok := list.([]ec2types.Image)
		if !ok {
			return gph, errors.New("cannot cast to '[]ec2types.Image' type from fetch context")
		}
		for _, r := range objects {
			for _, fn := range addParentsFns["image"] {
				wg.Add(1)
				go func(f addParentFn, snap tstore.RDFGraph, region string, res ec2types.Image) {
					defer wg.Done()
					if err := f(gph, snap, region, res); err != nil {
						errc <- err
					}
				}(fn, snap, s.region, r)
			}
		}
	}
	if getBool(s.config, "aws.infra.importimagetask.sync", true) {
		list, err := s.fetcher.Get("importimagetask_objects")
		if err != nil {
			return gph, err
		}
		objects, ok := list.([]ec2types.ImportImageTask)
		if !ok {
			return gph, errors.New("cannot cast to '[]ec2types.ImportImageTask' type from fetch context")
		}
		for _, r := range objects {
			for _, fn := range addParentsFns["importimagetask"] {
				wg.Add(1)
				go func(f addParentFn, snap tstore.RDFGraph, region string, res ec2types.ImportImageTask) {
					defer wg.Done()
					if err := f(gph, snap, region, res); err != nil {
						errc <- err
					}
				}(fn, snap, s.region, r)
			}
		}
	}
	if getBool(s.config, "aws.infra.elasticip.sync", true) {
		list, err := s.fetcher.Get("elasticip_objects")
		if err != nil {
			return gph, err
		}
		objects, ok := list.([]ec2types.Address)
		if !ok {
			return gph, errors.New("cannot cast to '[]ec2types.Address' type from fetch context")
		}
		for _, r := range objects {
			for _, fn := range addParentsFns["elasticip"] {
				wg.Add(1)
				go func(f addParentFn, snap tstore.RDFGraph, region string, res ec2types.Address) {
					defer wg.Done()
					if err := f(gph, snap, region, res); err != nil {
						errc <- err
					}
				}(fn, snap, s.region, r)
			}
		}
	}
	if getBool(s.config, "aws.infra.snapshot.sync", true) {
		list, err := s.fetcher.Get("snapshot_objects")
		if err != nil {
			return gph, err
		}
		objects, ok := list.([]ec2types.Snapshot)
		if !ok {
			return gph, errors.New("cannot cast to '[]ec2types.Snapshot' type from fetch context")
		}
		for _, r := range objects {
			for _, fn := range addParentsFns["snapshot"] {
				wg.Add(1)
				go func(f addParentFn, snap tstore.RDFGraph, region string, res ec2types.Snapshot) {
					defer wg.Done()
					if err := f(gph, snap, region, res); err != nil {
						errc <- err
					}
				}(fn, snap, s.region, r)
			}
		}
	}
	if getBool(s.config, "aws.infra.networkinterface.sync", true) {
		list, err := s.fetcher.Get("networkinterface_objects")
		if err != nil {
			return gph, err
		}
		objects, ok := list.([]ec2types.NetworkInterface)
		if !ok {
			return gph, errors.New("cannot cast to '[]ec2types.NetworkInterface' type from fetch context")
		}
		for _, r := range objects {
			for _, fn := range addParentsFns["networkinterface"] {
				wg.Add(1)
				go func(f addParentFn, snap tstore.RDFGraph, region string, res ec2types.NetworkInterface) {
					defer wg.Done()
					if err := f(gph, snap, region, res); err != nil {
						errc <- err
					}
				}(fn, snap, s.region, r)
			}
		}
	}
	if getBool(s.config, "aws.infra.classicloadbalancer.sync", true) {
		list, err := s.fetcher.Get("classicloadbalancer_objects")
		if err != nil {
			return gph, err
		}
		objects, ok := list.([]elbtypes.LoadBalancerDescription)
		if !ok {
			return gph, errors.New("cannot cast to '[]elbtypes.LoadBalancerDescription' type from fetch context")
		}
		for _, r := range objects {
			for _, fn := range addParentsFns["classicloadbalancer"] {
				wg.Add(1)
				go func(f addParentFn, snap tstore.RDFGraph, region string, res elbtypes.LoadBalancerDescription) {
					defer wg.Done()
					if err := f(gph, snap, region, res); err != nil {
						errc <- err
					}
				}(fn, snap, s.region, r)
			}
		}
	}
	if getBool(s.config, "aws.infra.loadbalancer.sync", true) {
		list, err := s.fetcher.Get("loadbalancer_objects")
		if err != nil {
			return gph, err
		}
		objects, ok := list.([]elbv2types.LoadBalancer)
		if !ok {
			return gph, errors.New("cannot cast to '[]elbv2types.LoadBalancer' type from fetch context")
		}
		for _, r := range objects {
			for _, fn := range addParentsFns["loadbalancer"] {
				wg.Add(1)
				go func(f addParentFn, snap tstore.RDFGraph, region string, res elbv2types.LoadBalancer) {
					defer wg.Done()
					if err := f(gph, snap, region, res); err != nil {
						errc <- err
					}
				}(fn, snap, s.region, r)
			}
		}
	}
	if getBool(s.config, "aws.infra.targetgroup.sync", true) {
		list, err := s.fetcher.Get("targetgroup_objects")
		if err != nil {
			return gph, err
		}
		objects, ok := list.([]elbv2types.TargetGroup)
		if !ok {
			return gph, errors.New("cannot cast to '[]elbv2types.TargetGroup' type from fetch context")
		}
		for _, r := range objects {
			for _, fn := range addParentsFns["targetgroup"] {
				wg.Add(1)
				go func(f addParentFn, snap tstore.RDFGraph, region string, res elbv2types.TargetGroup) {
					defer wg.Done()
					if err := f(gph, snap, region, res); err != nil {
						errc <- err
					}
				}(fn, snap, s.region, r)
			}
		}
	}
	if getBool(s.config, "aws.infra.listener.sync", true) {
		list, err := s.fetcher.Get("listener_objects")
		if err != nil {
			return gph, err
		}
		objects, ok := list.([]elbv2types.Listener)
		if !ok {
			return gph, errors.New("cannot cast to '[]elbv2types.Listener' type from fetch context")
		}
		for _, r := range objects {
			for _, fn := range addParentsFns["listener"] {
				wg.Add(1)
				go func(f addParentFn, snap tstore.RDFGraph, region string, res elbv2types.Listener) {
					defer wg.Done()
					if err := f(gph, snap, region, res); err != nil {
						errc <- err
					}
				}(fn, snap, s.region, r)
			}
		}
	}
	if getBool(s.config, "aws.infra.database.sync", true) {
		list, err := s.fetcher.Get("database_objects")
		if err != nil {
			return gph, err
		}
		objects, ok := list.([]rdstypes.DBInstance)
		if !ok {
			return gph, errors.New("cannot cast to '[]rdstypes.DBInstance' type from fetch context")
		}
		for _, r := range objects {
			for _, fn := range addParentsFns["database"] {
				wg.Add(1)
				go func(f addParentFn, snap tstore.RDFGraph, region string, res rdstypes.DBInstance) {
					defer wg.Done()
					if err := f(gph, snap, region, res); err != nil {
						errc <- err
					}
				}(fn, snap, s.region, r)
			}
		}
	}
	if getBool(s.config, "aws.infra.dbsubnetgroup.sync", true) {
		list, err := s.fetcher.Get("dbsubnetgroup_objects")
		if err != nil {
			return gph, err
		}
		objects, ok := list.([]rdstypes.DBSubnetGroup)
		if !ok {
			return gph, errors.New("cannot cast to '[]rdstypes.DBSubnetGroup' type from fetch context")
		}
		for _, r := range objects {
			for _, fn := range addParentsFns["dbsubnetgroup"] {
				wg.Add(1)
				go func(f addParentFn, snap tstore.RDFGraph, region string, res rdstypes.DBSubnetGroup) {
					defer wg.Done()
					if err := f(gph, snap, region, res); err != nil {
						errc <- err
					}
				}(fn, snap, s.region, r)
			}
		}
	}
	if getBool(s.config, "aws.infra.launchconfiguration.sync", true) {
		list, err := s.fetcher.Get("launchconfiguration_objects")
		if err != nil {
			return gph, err
		}
		objects, ok := list.([]autoscalingtypes.LaunchConfiguration)
		if !ok {
			return gph, errors.New("cannot cast to '[]autoscalingtypes.LaunchConfiguration' type from fetch context")
		}
		for _, r := range objects {
			for _, fn := range addParentsFns["launchconfiguration"] {
				wg.Add(1)
				go func(f addParentFn, snap tstore.RDFGraph, region string, res autoscalingtypes.LaunchConfiguration) {
					defer wg.Done()
					if err := f(gph, snap, region, res); err != nil {
						errc <- err
					}
				}(fn, snap, s.region, r)
			}
		}
	}
	if getBool(s.config, "aws.infra.scalinggroup.sync", true) {
		list, err := s.fetcher.Get("scalinggroup_objects")
		if err != nil {
			return gph, err
		}
		objects, ok := list.([]autoscalingtypes.AutoScalingGroup)
		if !ok {
			return gph, errors.New("cannot cast to '[]autoscalingtypes.AutoScalingGroup' type from fetch context")
		}
		for _, r := range objects {
			for _, fn := range addParentsFns["scalinggroup"] {
				wg.Add(1)
				go func(f addParentFn, snap tstore.RDFGraph, region string, res autoscalingtypes.AutoScalingGroup) {
					defer wg.Done()
					if err := f(gph, snap, region, res); err != nil {
						errc <- err
					}
				}(fn, snap, s.region, r)
			}
		}
	}
	if getBool(s.config, "aws.infra.scalingpolicy.sync", true) {
		list, err := s.fetcher.Get("scalingpolicy_objects")
		if err != nil {
			return gph, err
		}
		objects, ok := list.([]autoscalingtypes.ScalingPolicy)
		if !ok {
			return gph, errors.New("cannot cast to '[]autoscalingtypes.ScalingPolicy' type from fetch context")
		}
		for _, r := range objects {
			for _, fn := range addParentsFns["scalingpolicy"] {
				wg.Add(1)
				go func(f addParentFn, snap tstore.RDFGraph, region string, res autoscalingtypes.ScalingPolicy) {
					defer wg.Done()
					if err := f(gph, snap, region, res); err != nil {
						errc <- err
					}
				}(fn, snap, s.region, r)
			}
		}
	}
	if getBool(s.config, "aws.infra.repository.sync", true) {
		list, err := s.fetcher.Get("repository_objects")
		if err != nil {
			return gph, err
		}
		objects, ok := list.([]ecrtypes.Repository)
		if !ok {
			return gph, errors.New("cannot cast to '[]ecrtypes.Repository' type from fetch context")
		}
		for _, r := range objects {
			for _, fn := range addParentsFns["repository"] {
				wg.Add(1)
				go func(f addParentFn, snap tstore.RDFGraph, region string, res ecrtypes.Repository) {
					defer wg.Done()
					if err := f(gph, snap, region, res); err != nil {
						errc <- err
					}
				}(fn, snap, s.region, r)
			}
		}
	}
	if getBool(s.config, "aws.infra.containercluster.sync", true) {
		list, err := s.fetcher.Get("containercluster_objects")
		if err != nil {
			return gph, err
		}
		objects, ok := list.([]ecstypes.Cluster)
		if !ok {
			return gph, errors.New("cannot cast to '[]ecstypes.Cluster' type from fetch context")
		}
		for _, r := range objects {
			for _, fn := range addParentsFns["containercluster"] {
				wg.Add(1)
				go func(f addParentFn, snap tstore.RDFGraph, region string, res ecstypes.Cluster) {
					defer wg.Done()
					if err := f(gph, snap, region, res); err != nil {
						errc <- err
					}
				}(fn, snap, s.region, r)
			}
		}
	}
	if getBool(s.config, "aws.infra.containertask.sync", true) {
		list, err := s.fetcher.Get("containertask_objects")
		if err != nil {
			return gph, err
		}
		objects, ok := list.([]ecstypes.TaskDefinition)
		if !ok {
			return gph, errors.New("cannot cast to '[]ecstypes.TaskDefinition' type from fetch context")
		}
		for _, r := range objects {
			for _, fn := range addParentsFns["containertask"] {
				wg.Add(1)
				go func(f addParentFn, snap tstore.RDFGraph, region string, res ecstypes.TaskDefinition) {
					defer wg.Done()
					if err := f(gph, snap, region, res); err != nil {
						errc <- err
					}
				}(fn, snap, s.region, r)
			}
		}
	}
	if getBool(s.config, "aws.infra.container.sync", true) {
		list, err := s.fetcher.Get("container_objects")
		if err != nil {
			return gph, err
		}
		objects, ok := list.([]ecstypes.Container)
		if !ok {
			return gph, errors.New("cannot cast to '[]ecstypes.Container' type from fetch context")
		}
		for _, r := range objects {
			for _, fn := range addParentsFns["container"] {
				wg.Add(1)
				go func(f addParentFn, snap tstore.RDFGraph, region string, res ecstypes.Container) {
					defer wg.Done()
					if err := f(gph, snap, region, res); err != nil {
						errc <- err
					}
				}(fn, snap, s.region, r)
			}
		}
	}
	if getBool(s.config, "aws.infra.containerinstance.sync", true) {
		list, err := s.fetcher.Get("containerinstance_objects")
		if err != nil {
			return gph, err
		}
		objects, ok := list.([]ecstypes.ContainerInstance)
		if !ok {
			return gph, errors.New("cannot cast to '[]ecstypes.ContainerInstance' type from fetch context")
		}
		for _, r := range objects {
			for _, fn := range addParentsFns["containerinstance"] {
				wg.Add(1)
				go func(f addParentFn, snap tstore.RDFGraph, region string, res ecstypes.ContainerInstance) {
					defer wg.Done()
					if err := f(gph, snap, region, res); err != nil {
						errc <- err
					}
				}(fn, snap, s.region, r)
			}
		}
	}
	if getBool(s.config, "aws.infra.certificate.sync", true) {
		list, err := s.fetcher.Get("certificate_objects")
		if err != nil {
			return gph, err
		}
		objects, ok := list.([]acmtypes.CertificateSummary)
		if !ok {
			return gph, errors.New("cannot cast to '[]acmtypes.CertificateSummary' type from fetch context")
		}
		for _, r := range objects {
			for _, fn := range addParentsFns["certificate"] {
				wg.Add(1)
				go func(f addParentFn, snap tstore.RDFGraph, region string, res acmtypes.CertificateSummary) {
					defer wg.Done()
					if err := f(gph, snap, region, res); err != nil {
						errc <- err
					}
				}(fn, snap, s.region, r)
			}
		}
	}

	go func() {
		wg.Wait()
		close(errc)
	}()

	for err := range errc {
		if err != nil {
			allErrors.Add(err)
		}
	}

	if allErrors.Any() {
		return gph, allErrors
	}

	return gph, nil
}

func (s *Infra) FetchByType(ctx context.Context, t string) (cloud.GraphAPI, error) {
	defer s.fetcher.Reset()
	return s.fetcher.FetchByType(context.WithValue(ctx, "region", s.region), t)
}

func (s *Infra) IsSyncDisabled() bool {
	return !getBool(s.config, "aws.infra.sync", true)
}

type Access struct {
	fetcher         fetch.Fetcher
	region, profile string
	config          map[string]interface{}
	log             *logger.Logger
	awsfetch.IamAPI
	awsfetch.StsAPI
}

func NewAccess(cfg awssdk.Config, profile string, extraConf map[string]interface{}, log *logger.Logger) cloud.Service {
	region := "global"
	iamAPI := iam.NewFromConfig(cfg)
	stsAPI := sts.NewFromConfig(cfg)

	fetchConfig := awsfetch.NewConfig(&awsfetch.AWSAPI{
		Iam: iamAPI,
		Sts: stsAPI,
	})
	fetchConfig.Extra = extraConf
	fetchConfig.Log = log

	return &Access{
		IamAPI:  iamAPI,
		StsAPI:  stsAPI,
		fetcher: fetch.NewFetcher(awsfetch.BuildAccessFetchFuncs(fetchConfig)),
		config:  extraConf,
		region:  region,
		profile: profile,
		log:     log,
	}
}

func (s *Access) Name() string {
	return "access"
}

func (s *Access) Region() string {
	return s.region
}

func (s *Access) Profile() string {
	return s.profile
}

func (s *Access) ResourceTypes() []string {
	return []string{
		"user",
		"group",
		"role",
		"policy",
		"accesskey",
		"instanceprofile",
		"mfadevice",
	}
}

func (s *Access) Fetch(ctx context.Context) (cloud.GraphAPI, error) {
	if s.IsSyncDisabled() {
		return graph.NewGraph(), nil
	}

	allErrors := new(fetch.Error)

	gph, err := s.fetcher.Fetch(context.WithValue(ctx, "region", s.region))
	defer s.fetcher.Reset()

	for _, e := range *fetch.WrapError(err) {
		switch {
		case e == nil:
			continue
		case awsfetch.IsAccessDenied(e):
			allErrors.Add(cloud.ErrFetchAccessDenied)
		default:
			allErrors.Add(e)
		}
	}

	if err := gph.AddResource(graph.InitResource(cloud.Region, s.region)); err != nil {
		return gph, err
	}

	snap := gph.AsRDFGraphSnaphot()

	errc := make(chan error)
	var wg sync.WaitGroup
	if getBool(s.config, "aws.access.user.sync", true) {
		list, err := s.fetcher.Get("user_objects")
		if err != nil {
			return gph, err
		}
		objects, ok := list.([]iamtypes.UserDetail)
		if !ok {
			return gph, errors.New("cannot cast to '[]iamtypes.UserDetail' type from fetch context")
		}
		for _, r := range objects {
			for _, fn := range addParentsFns["user"] {
				wg.Add(1)
				go func(f addParentFn, snap tstore.RDFGraph, region string, res iamtypes.UserDetail) {
					defer wg.Done()
					if err := f(gph, snap, region, res); err != nil {
						errc <- err
					}
				}(fn, snap, s.region, r)
			}
		}
	}
	if getBool(s.config, "aws.access.group.sync", true) {
		list, err := s.fetcher.Get("group_objects")
		if err != nil {
			return gph, err
		}
		objects, ok := list.([]iamtypes.GroupDetail)
		if !ok {
			return gph, errors.New("cannot cast to '[]iamtypes.GroupDetail' type from fetch context")
		}
		for _, r := range objects {
			for _, fn := range addParentsFns["group"] {
				wg.Add(1)
				go func(f addParentFn, snap tstore.RDFGraph, region string, res iamtypes.GroupDetail) {
					defer wg.Done()
					if err := f(gph, snap, region, res); err != nil {
						errc <- err
					}
				}(fn, snap, s.region, r)
			}
		}
	}
	if getBool(s.config, "aws.access.role.sync", true) {
		list, err := s.fetcher.Get("role_objects")
		if err != nil {
			return gph, err
		}
		objects, ok := list.([]iamtypes.RoleDetail)
		if !ok {
			return gph, errors.New("cannot cast to '[]iamtypes.RoleDetail' type from fetch context")
		}
		for _, r := range objects {
			for _, fn := range addParentsFns["role"] {
				wg.Add(1)
				go func(f addParentFn, snap tstore.RDFGraph, region string, res iamtypes.RoleDetail) {
					defer wg.Done()
					if err := f(gph, snap, region, res); err != nil {
						errc <- err
					}
				}(fn, snap, s.region, r)
			}
		}
	}
	if getBool(s.config, "aws.access.policy.sync", true) {
		list, err := s.fetcher.Get("policy_objects")
		if err != nil {
			return gph, err
		}
		objects, ok := list.([]iamtypes.Policy)
		if !ok {
			return gph, errors.New("cannot cast to '[]iamtypes.Policy' type from fetch context")
		}
		for _, r := range objects {
			for _, fn := range addParentsFns["policy"] {
				wg.Add(1)
				go func(f addParentFn, snap tstore.RDFGraph, region string, res iamtypes.Policy) {
					defer wg.Done()
					if err := f(gph, snap, region, res); err != nil {
						errc <- err
					}
				}(fn, snap, s.region, r)
			}
		}
	}
	if getBool(s.config, "aws.access.accesskey.sync", true) {
		list, err := s.fetcher.Get("accesskey_objects")
		if err != nil {
			return gph, err
		}
		objects, ok := list.([]iamtypes.AccessKeyMetadata)
		if !ok {
			return gph, errors.New("cannot cast to '[]iamtypes.AccessKeyMetadata' type from fetch context")
		}
		for _, r := range objects {
			for _, fn := range addParentsFns["accesskey"] {
				wg.Add(1)
				go func(f addParentFn, snap tstore.RDFGraph, region string, res iamtypes.AccessKeyMetadata) {
					defer wg.Done()
					if err := f(gph, snap, region, res); err != nil {
						errc <- err
					}
				}(fn, snap, s.region, r)
			}
		}
	}
	if getBool(s.config, "aws.access.instanceprofile.sync", true) {
		list, err := s.fetcher.Get("instanceprofile_objects")
		if err != nil {
			return gph, err
		}
		objects, ok := list.([]iamtypes.InstanceProfile)
		if !ok {
			return gph, errors.New("cannot cast to '[]iamtypes.InstanceProfile' type from fetch context")
		}
		for _, r := range objects {
			for _, fn := range addParentsFns["instanceprofile"] {
				wg.Add(1)
				go func(f addParentFn, snap tstore.RDFGraph, region string, res iamtypes.InstanceProfile) {
					defer wg.Done()
					if err := f(gph, snap, region, res); err != nil {
						errc <- err
					}
				}(fn, snap, s.region, r)
			}
		}
	}
	if getBool(s.config, "aws.access.mfadevice.sync", true) {
		list, err := s.fetcher.Get("mfadevice_objects")
		if err != nil {
			return gph, err
		}
		objects, ok := list.([]iamtypes.VirtualMFADevice)
		if !ok {
			return gph, errors.New("cannot cast to '[]iamtypes.VirtualMFADevice' type from fetch context")
		}
		for _, r := range objects {
			for _, fn := range addParentsFns["mfadevice"] {
				wg.Add(1)
				go func(f addParentFn, snap tstore.RDFGraph, region string, res iamtypes.VirtualMFADevice) {
					defer wg.Done()
					if err := f(gph, snap, region, res); err != nil {
						errc <- err
					}
				}(fn, snap, s.region, r)
			}
		}
	}

	go func() {
		wg.Wait()
		close(errc)
	}()

	for err := range errc {
		if err != nil {
			allErrors.Add(err)
		}
	}

	if allErrors.Any() {
		return gph, allErrors
	}

	return gph, nil
}

func (s *Access) FetchByType(ctx context.Context, t string) (cloud.GraphAPI, error) {
	defer s.fetcher.Reset()
	return s.fetcher.FetchByType(context.WithValue(ctx, "region", s.region), t)
}

func (s *Access) IsSyncDisabled() bool {
	return !getBool(s.config, "aws.access.sync", true)
}

type Storage struct {
	fetcher         fetch.Fetcher
	region, profile string
	config          map[string]interface{}
	log             *logger.Logger
	awsfetch.S3API
}

func NewStorage(cfg awssdk.Config, profile string, extraConf map[string]interface{}, log *logger.Logger) cloud.Service {
	region := cfg.Region
	s3API := s3.NewFromConfig(cfg)

	fetchConfig := awsfetch.NewConfig(&awsfetch.AWSAPI{
		S3: s3API,
	})
	fetchConfig.Extra = extraConf
	fetchConfig.Log = log

	return &Storage{
		S3API:   s3API,
		fetcher: fetch.NewFetcher(awsfetch.BuildStorageFetchFuncs(fetchConfig)),
		config:  extraConf,
		region:  region,
		profile: profile,
		log:     log,
	}
}

func (s *Storage) Name() string {
	return "storage"
}

func (s *Storage) Region() string {
	return s.region
}

func (s *Storage) Profile() string {
	return s.profile
}

func (s *Storage) ResourceTypes() []string {
	return []string{
		"bucket",
		"s3object",
	}
}

func (s *Storage) Fetch(ctx context.Context) (cloud.GraphAPI, error) {
	if s.IsSyncDisabled() {
		return graph.NewGraph(), nil
	}

	allErrors := new(fetch.Error)

	gph, err := s.fetcher.Fetch(context.WithValue(ctx, "region", s.region))
	defer s.fetcher.Reset()

	for _, e := range *fetch.WrapError(err) {
		switch {
		case e == nil:
			continue
		case awsfetch.IsAccessDenied(e):
			allErrors.Add(cloud.ErrFetchAccessDenied)
		default:
			allErrors.Add(e)
		}
	}

	if err := gph.AddResource(graph.InitResource(cloud.Region, s.region)); err != nil {
		return gph, err
	}

	snap := gph.AsRDFGraphSnaphot()

	errc := make(chan error)
	var wg sync.WaitGroup
	if getBool(s.config, "aws.storage.bucket.sync", true) {
		list, err := s.fetcher.Get("bucket_objects")
		if err != nil {
			return gph, err
		}
		objects, ok := list.([]s3types.Bucket)
		if !ok {
			return gph, errors.New("cannot cast to '[]s3types.Bucket' type from fetch context")
		}
		for _, r := range objects {
			for _, fn := range addParentsFns["bucket"] {
				wg.Add(1)
				go func(f addParentFn, snap tstore.RDFGraph, region string, res s3types.Bucket) {
					defer wg.Done()
					if err := f(gph, snap, region, res); err != nil {
						errc <- err
					}
				}(fn, snap, s.region, r)
			}
		}
	}
	if getBool(s.config, "aws.storage.s3object.sync", true) {
		list, err := s.fetcher.Get("s3object_objects")
		if err != nil {
			return gph, err
		}
		objects, ok := list.([]s3types.Object)
		if !ok {
			return gph, errors.New("cannot cast to '[]s3types.Object' type from fetch context")
		}
		for _, r := range objects {
			for _, fn := range addParentsFns["s3object"] {
				wg.Add(1)
				go func(f addParentFn, snap tstore.RDFGraph, region string, res s3types.Object) {
					defer wg.Done()
					if err := f(gph, snap, region, res); err != nil {
						errc <- err
					}
				}(fn, snap, s.region, r)
			}
		}
	}

	go func() {
		wg.Wait()
		close(errc)
	}()

	for err := range errc {
		if err != nil {
			allErrors.Add(err)
		}
	}

	if allErrors.Any() {
		return gph, allErrors
	}

	return gph, nil
}

func (s *Storage) FetchByType(ctx context.Context, t string) (cloud.GraphAPI, error) {
	defer s.fetcher.Reset()
	return s.fetcher.FetchByType(context.WithValue(ctx, "region", s.region), t)
}

func (s *Storage) IsSyncDisabled() bool {
	return !getBool(s.config, "aws.storage.sync", true)
}

type Messaging struct {
	fetcher         fetch.Fetcher
	region, profile string
	config          map[string]interface{}
	log             *logger.Logger
	awsfetch.SnsAPI
	awsfetch.SqsAPI
}

func NewMessaging(cfg awssdk.Config, profile string, extraConf map[string]interface{}, log *logger.Logger) cloud.Service {
	region := cfg.Region
	snsAPI := sns.NewFromConfig(cfg)
	sqsAPI := sqs.NewFromConfig(cfg)

	fetchConfig := awsfetch.NewConfig(&awsfetch.AWSAPI{
		Sns: snsAPI,
		Sqs: sqsAPI,
	})
	fetchConfig.Extra = extraConf
	fetchConfig.Log = log

	return &Messaging{
		SnsAPI:  snsAPI,
		SqsAPI:  sqsAPI,
		fetcher: fetch.NewFetcher(awsfetch.BuildMessagingFetchFuncs(fetchConfig)),
		config:  extraConf,
		region:  region,
		profile: profile,
		log:     log,
	}
}

func (s *Messaging) Name() string {
	return "messaging"
}

func (s *Messaging) Region() string {
	return s.region
}

func (s *Messaging) Profile() string {
	return s.profile
}

func (s *Messaging) ResourceTypes() []string {
	return []string{
		"subscription",
		"topic",
		"queue",
	}
}

func (s *Messaging) Fetch(ctx context.Context) (cloud.GraphAPI, error) {
	if s.IsSyncDisabled() {
		return graph.NewGraph(), nil
	}

	allErrors := new(fetch.Error)

	gph, err := s.fetcher.Fetch(context.WithValue(ctx, "region", s.region))
	defer s.fetcher.Reset()

	for _, e := range *fetch.WrapError(err) {
		switch {
		case e == nil:
			continue
		case awsfetch.IsAccessDenied(e):
			allErrors.Add(cloud.ErrFetchAccessDenied)
		default:
			allErrors.Add(e)
		}
	}

	if err := gph.AddResource(graph.InitResource(cloud.Region, s.region)); err != nil {
		return gph, err
	}

	snap := gph.AsRDFGraphSnaphot()

	errc := make(chan error)
	var wg sync.WaitGroup
	if getBool(s.config, "aws.messaging.subscription.sync", true) {
		list, err := s.fetcher.Get("subscription_objects")
		if err != nil {
			return gph, err
		}
		objects, ok := list.([]snstypes.Subscription)
		if !ok {
			return gph, errors.New("cannot cast to '[]snstypes.Subscription' type from fetch context")
		}
		for _, r := range objects {
			for _, fn := range addParentsFns["subscription"] {
				wg.Add(1)
				go func(f addParentFn, snap tstore.RDFGraph, region string, res snstypes.Subscription) {
					defer wg.Done()
					if err := f(gph, snap, region, res); err != nil {
						errc <- err
					}
				}(fn, snap, s.region, r)
			}
		}
	}
	if getBool(s.config, "aws.messaging.topic.sync", true) {
		list, err := s.fetcher.Get("topic_objects")
		if err != nil {
			return gph, err
		}
		objects, ok := list.([]snstypes.Topic)
		if !ok {
			return gph, errors.New("cannot cast to '[]snstypes.Topic' type from fetch context")
		}
		for _, r := range objects {
			for _, fn := range addParentsFns["topic"] {
				wg.Add(1)
				go func(f addParentFn, snap tstore.RDFGraph, region string, res snstypes.Topic) {
					defer wg.Done()
					if err := f(gph, snap, region, res); err != nil {
						errc <- err
					}
				}(fn, snap, s.region, r)
			}
		}
	}
	if getBool(s.config, "aws.messaging.queue.sync", true) {
		list, err := s.fetcher.Get("queue_objects")
		if err != nil {
			return gph, err
		}
		objects, ok := list.([]string)
		if !ok {
			return gph, errors.New("cannot cast to '[]string' type from fetch context")
		}
		for _, r := range objects {
			for _, fn := range addParentsFns["queue"] {
				wg.Add(1)
				go func(f addParentFn, snap tstore.RDFGraph, region string, res string) {
					defer wg.Done()
					if err := f(gph, snap, region, res); err != nil {
						errc <- err
					}
				}(fn, snap, s.region, r)
			}
		}
	}

	go func() {
		wg.Wait()
		close(errc)
	}()

	for err := range errc {
		if err != nil {
			allErrors.Add(err)
		}
	}

	if allErrors.Any() {
		return gph, allErrors
	}

	return gph, nil
}

func (s *Messaging) FetchByType(ctx context.Context, t string) (cloud.GraphAPI, error) {
	defer s.fetcher.Reset()
	return s.fetcher.FetchByType(context.WithValue(ctx, "region", s.region), t)
}

func (s *Messaging) IsSyncDisabled() bool {
	return !getBool(s.config, "aws.messaging.sync", true)
}

type Dns struct {
	fetcher         fetch.Fetcher
	region, profile string
	config          map[string]interface{}
	log             *logger.Logger
	awsfetch.Route53API
}

func NewDns(cfg awssdk.Config, profile string, extraConf map[string]interface{}, log *logger.Logger) cloud.Service {
	region := "global"
	route53API := route53.NewFromConfig(cfg)

	fetchConfig := awsfetch.NewConfig(&awsfetch.AWSAPI{
		Route53: route53API,
	})
	fetchConfig.Extra = extraConf
	fetchConfig.Log = log

	return &Dns{
		Route53API: route53API,
		fetcher:    fetch.NewFetcher(awsfetch.BuildDnsFetchFuncs(fetchConfig)),
		config:     extraConf,
		region:     region,
		profile:    profile,
		log:        log,
	}
}

func (s *Dns) Name() string {
	return "dns"
}

func (s *Dns) Region() string {
	return s.region
}

func (s *Dns) Profile() string {
	return s.profile
}

func (s *Dns) ResourceTypes() []string {
	return []string{
		"zone",
		"record",
	}
}

func (s *Dns) Fetch(ctx context.Context) (cloud.GraphAPI, error) {
	if s.IsSyncDisabled() {
		return graph.NewGraph(), nil
	}

	allErrors := new(fetch.Error)

	gph, err := s.fetcher.Fetch(context.WithValue(ctx, "region", s.region))
	defer s.fetcher.Reset()

	for _, e := range *fetch.WrapError(err) {
		switch {
		case e == nil:
			continue
		case awsfetch.IsAccessDenied(e):
			allErrors.Add(cloud.ErrFetchAccessDenied)
		default:
			allErrors.Add(e)
		}
	}

	if err := gph.AddResource(graph.InitResource(cloud.Region, s.region)); err != nil {
		return gph, err
	}

	snap := gph.AsRDFGraphSnaphot()

	errc := make(chan error)
	var wg sync.WaitGroup
	if getBool(s.config, "aws.dns.zone.sync", true) {
		list, err := s.fetcher.Get("zone_objects")
		if err != nil {
			return gph, err
		}
		objects, ok := list.([]route53types.HostedZone)
		if !ok {
			return gph, errors.New("cannot cast to '[]route53types.HostedZone' type from fetch context")
		}
		for _, r := range objects {
			for _, fn := range addParentsFns["zone"] {
				wg.Add(1)
				go func(f addParentFn, snap tstore.RDFGraph, region string, res route53types.HostedZone) {
					defer wg.Done()
					if err := f(gph, snap, region, res); err != nil {
						errc <- err
					}
				}(fn, snap, s.region, r)
			}
		}
	}
	if getBool(s.config, "aws.dns.record.sync", true) {
		list, err := s.fetcher.Get("record_objects")
		if err != nil {
			return gph, err
		}
		objects, ok := list.([]route53types.ResourceRecordSet)
		if !ok {
			return gph, errors.New("cannot cast to '[]route53types.ResourceRecordSet' type from fetch context")
		}
		for _, r := range objects {
			for _, fn := range addParentsFns["record"] {
				wg.Add(1)
				go func(f addParentFn, snap tstore.RDFGraph, region string, res route53types.ResourceRecordSet) {
					defer wg.Done()
					if err := f(gph, snap, region, res); err != nil {
						errc <- err
					}
				}(fn, snap, s.region, r)
			}
		}
	}

	go func() {
		wg.Wait()
		close(errc)
	}()

	for err := range errc {
		if err != nil {
			allErrors.Add(err)
		}
	}

	if allErrors.Any() {
		return gph, allErrors
	}

	return gph, nil
}

func (s *Dns) FetchByType(ctx context.Context, t string) (cloud.GraphAPI, error) {
	defer s.fetcher.Reset()
	return s.fetcher.FetchByType(context.WithValue(ctx, "region", s.region), t)
}

func (s *Dns) IsSyncDisabled() bool {
	return !getBool(s.config, "aws.dns.sync", true)
}

type Lambda struct {
	fetcher         fetch.Fetcher
	region, profile string
	config          map[string]interface{}
	log             *logger.Logger
	awsfetch.LambdaAPI
}

func NewLambda(cfg awssdk.Config, profile string, extraConf map[string]interface{}, log *logger.Logger) cloud.Service {
	region := cfg.Region
	lambdaAPI := lambda.NewFromConfig(cfg)

	fetchConfig := awsfetch.NewConfig(&awsfetch.AWSAPI{
		Lambda: lambdaAPI,
	})
	fetchConfig.Extra = extraConf
	fetchConfig.Log = log

	return &Lambda{
		LambdaAPI: lambdaAPI,
		fetcher:   fetch.NewFetcher(awsfetch.BuildLambdaFetchFuncs(fetchConfig)),
		config:    extraConf,
		region:    region,
		profile:   profile,
		log:       log,
	}
}

func (s *Lambda) Name() string {
	return "lambda"
}

func (s *Lambda) Region() string {
	return s.region
}

func (s *Lambda) Profile() string {
	return s.profile
}

func (s *Lambda) ResourceTypes() []string {
	return []string{
		"function",
	}
}

func (s *Lambda) Fetch(ctx context.Context) (cloud.GraphAPI, error) {
	if s.IsSyncDisabled() {
		return graph.NewGraph(), nil
	}

	allErrors := new(fetch.Error)

	gph, err := s.fetcher.Fetch(context.WithValue(ctx, "region", s.region))
	defer s.fetcher.Reset()

	for _, e := range *fetch.WrapError(err) {
		switch {
		case e == nil:
			continue
		case awsfetch.IsAccessDenied(e):
			allErrors.Add(cloud.ErrFetchAccessDenied)
		default:
			allErrors.Add(e)
		}
	}

	if err := gph.AddResource(graph.InitResource(cloud.Region, s.region)); err != nil {
		return gph, err
	}

	snap := gph.AsRDFGraphSnaphot()

	errc := make(chan error)
	var wg sync.WaitGroup
	if getBool(s.config, "aws.lambda.function.sync", true) {
		list, err := s.fetcher.Get("function_objects")
		if err != nil {
			return gph, err
		}
		objects, ok := list.([]lambdatypes.FunctionConfiguration)
		if !ok {
			return gph, errors.New("cannot cast to '[]lambdatypes.FunctionConfiguration' type from fetch context")
		}
		for _, r := range objects {
			for _, fn := range addParentsFns["function"] {
				wg.Add(1)
				go func(f addParentFn, snap tstore.RDFGraph, region string, res lambdatypes.FunctionConfiguration) {
					defer wg.Done()
					if err := f(gph, snap, region, res); err != nil {
						errc <- err
					}
				}(fn, snap, s.region, r)
			}
		}
	}

	go func() {
		wg.Wait()
		close(errc)
	}()

	for err := range errc {
		if err != nil {
			allErrors.Add(err)
		}
	}

	if allErrors.Any() {
		return gph, allErrors
	}

	return gph, nil
}

func (s *Lambda) FetchByType(ctx context.Context, t string) (cloud.GraphAPI, error) {
	defer s.fetcher.Reset()
	return s.fetcher.FetchByType(context.WithValue(ctx, "region", s.region), t)
}

func (s *Lambda) IsSyncDisabled() bool {
	return !getBool(s.config, "aws.lambda.sync", true)
}

type Monitoring struct {
	fetcher         fetch.Fetcher
	region, profile string
	config          map[string]interface{}
	log             *logger.Logger
	awsfetch.CloudwatchAPI
}

func NewMonitoring(cfg awssdk.Config, profile string, extraConf map[string]interface{}, log *logger.Logger) cloud.Service {
	region := cfg.Region
	cloudwatchAPI := cloudwatch.NewFromConfig(cfg)

	fetchConfig := awsfetch.NewConfig(&awsfetch.AWSAPI{
		Cloudwatch: cloudwatchAPI,
	})
	fetchConfig.Extra = extraConf
	fetchConfig.Log = log

	return &Monitoring{
		CloudwatchAPI: cloudwatchAPI,
		fetcher:       fetch.NewFetcher(awsfetch.BuildMonitoringFetchFuncs(fetchConfig)),
		config:        extraConf,
		region:        region,
		profile:       profile,
		log:           log,
	}
}

func (s *Monitoring) Name() string {
	return "monitoring"
}

func (s *Monitoring) Region() string {
	return s.region
}

func (s *Monitoring) Profile() string {
	return s.profile
}

func (s *Monitoring) ResourceTypes() []string {
	return []string{
		"metric",
		"alarm",
	}
}

func (s *Monitoring) Fetch(ctx context.Context) (cloud.GraphAPI, error) {
	if s.IsSyncDisabled() {
		return graph.NewGraph(), nil
	}

	allErrors := new(fetch.Error)

	gph, err := s.fetcher.Fetch(context.WithValue(ctx, "region", s.region))
	defer s.fetcher.Reset()

	for _, e := range *fetch.WrapError(err) {
		switch {
		case e == nil:
			continue
		case awsfetch.IsAccessDenied(e):
			allErrors.Add(cloud.ErrFetchAccessDenied)
		default:
			allErrors.Add(e)
		}
	}

	if err := gph.AddResource(graph.InitResource(cloud.Region, s.region)); err != nil {
		return gph, err
	}

	snap := gph.AsRDFGraphSnaphot()

	errc := make(chan error)
	var wg sync.WaitGroup
	if getBool(s.config, "aws.monitoring.metric.sync", true) {
		list, err := s.fetcher.Get("metric_objects")
		if err != nil {
			return gph, err
		}
		objects, ok := list.([]cloudwatchtypes.Metric)
		if !ok {
			return gph, errors.New("cannot cast to '[]cloudwatchtypes.Metric' type from fetch context")
		}
		for _, r := range objects {
			for _, fn := range addParentsFns["metric"] {
				wg.Add(1)
				go func(f addParentFn, snap tstore.RDFGraph, region string, res cloudwatchtypes.Metric) {
					defer wg.Done()
					if err := f(gph, snap, region, res); err != nil {
						errc <- err
					}
				}(fn, snap, s.region, r)
			}
		}
	}
	if getBool(s.config, "aws.monitoring.alarm.sync", true) {
		list, err := s.fetcher.Get("alarm_objects")
		if err != nil {
			return gph, err
		}
		objects, ok := list.([]cloudwatchtypes.MetricAlarm)
		if !ok {
			return gph, errors.New("cannot cast to '[]cloudwatchtypes.MetricAlarm' type from fetch context")
		}
		for _, r := range objects {
			for _, fn := range addParentsFns["alarm"] {
				wg.Add(1)
				go func(f addParentFn, snap tstore.RDFGraph, region string, res cloudwatchtypes.MetricAlarm) {
					defer wg.Done()
					if err := f(gph, snap, region, res); err != nil {
						errc <- err
					}
				}(fn, snap, s.region, r)
			}
		}
	}

	go func() {
		wg.Wait()
		close(errc)
	}()

	for err := range errc {
		if err != nil {
			allErrors.Add(err)
		}
	}

	if allErrors.Any() {
		return gph, allErrors
	}

	return gph, nil
}

func (s *Monitoring) FetchByType(ctx context.Context, t string) (cloud.GraphAPI, error) {
	defer s.fetcher.Reset()
	return s.fetcher.FetchByType(context.WithValue(ctx, "region", s.region), t)
}

func (s *Monitoring) IsSyncDisabled() bool {
	return !getBool(s.config, "aws.monitoring.sync", true)
}

type Cdn struct {
	fetcher         fetch.Fetcher
	region, profile string
	config          map[string]interface{}
	log             *logger.Logger
	awsfetch.CloudfrontAPI
}

func NewCdn(cfg awssdk.Config, profile string, extraConf map[string]interface{}, log *logger.Logger) cloud.Service {
	region := "global"
	cloudfrontAPI := cloudfront.NewFromConfig(cfg)

	fetchConfig := awsfetch.NewConfig(&awsfetch.AWSAPI{
		Cloudfront: cloudfrontAPI,
	})
	fetchConfig.Extra = extraConf
	fetchConfig.Log = log

	return &Cdn{
		CloudfrontAPI: cloudfrontAPI,
		fetcher:       fetch.NewFetcher(awsfetch.BuildCdnFetchFuncs(fetchConfig)),
		config:        extraConf,
		region:        region,
		profile:       profile,
		log:           log,
	}
}

func (s *Cdn) Name() string {
	return "cdn"
}

func (s *Cdn) Region() string {
	return s.region
}

func (s *Cdn) Profile() string {
	return s.profile
}

func (s *Cdn) ResourceTypes() []string {
	return []string{
		"distribution",
	}
}

func (s *Cdn) Fetch(ctx context.Context) (cloud.GraphAPI, error) {
	if s.IsSyncDisabled() {
		return graph.NewGraph(), nil
	}

	allErrors := new(fetch.Error)

	gph, err := s.fetcher.Fetch(context.WithValue(ctx, "region", s.region))
	defer s.fetcher.Reset()

	for _, e := range *fetch.WrapError(err) {
		switch {
		case e == nil:
			continue
		case awsfetch.IsAccessDenied(e):
			allErrors.Add(cloud.ErrFetchAccessDenied)
		default:
			allErrors.Add(e)
		}
	}

	if err := gph.AddResource(graph.InitResource(cloud.Region, s.region)); err != nil {
		return gph, err
	}

	snap := gph.AsRDFGraphSnaphot()

	errc := make(chan error)
	var wg sync.WaitGroup
	if getBool(s.config, "aws.cdn.distribution.sync", true) {
		list, err := s.fetcher.Get("distribution_objects")
		if err != nil {
			return gph, err
		}
		objects, ok := list.([]cloudfronttypes.DistributionSummary)
		if !ok {
			return gph, errors.New("cannot cast to '[]cloudfronttypes.DistributionSummary' type from fetch context")
		}
		for _, r := range objects {
			for _, fn := range addParentsFns["distribution"] {
				wg.Add(1)
				go func(f addParentFn, snap tstore.RDFGraph, region string, res cloudfronttypes.DistributionSummary) {
					defer wg.Done()
					if err := f(gph, snap, region, res); err != nil {
						errc <- err
					}
				}(fn, snap, s.region, r)
			}
		}
	}

	go func() {
		wg.Wait()
		close(errc)
	}()

	for err := range errc {
		if err != nil {
			allErrors.Add(err)
		}
	}

	if allErrors.Any() {
		return gph, allErrors
	}

	return gph, nil
}

func (s *Cdn) FetchByType(ctx context.Context, t string) (cloud.GraphAPI, error) {
	defer s.fetcher.Reset()
	return s.fetcher.FetchByType(context.WithValue(ctx, "region", s.region), t)
}

func (s *Cdn) IsSyncDisabled() bool {
	return !getBool(s.config, "aws.cdn.sync", true)
}

type Cloudformation struct {
	fetcher         fetch.Fetcher
	region, profile string
	config          map[string]interface{}
	log             *logger.Logger
	awsfetch.CloudformationAPI
}

func NewCloudformation(cfg awssdk.Config, profile string, extraConf map[string]interface{}, log *logger.Logger) cloud.Service {
	region := cfg.Region
	cloudformationAPI := cloudformation.NewFromConfig(cfg)

	fetchConfig := awsfetch.NewConfig(&awsfetch.AWSAPI{
		Cloudformation: cloudformationAPI,
	})
	fetchConfig.Extra = extraConf
	fetchConfig.Log = log

	return &Cloudformation{
		CloudformationAPI: cloudformationAPI,
		fetcher:           fetch.NewFetcher(awsfetch.BuildCloudformationFetchFuncs(fetchConfig)),
		config:            extraConf,
		region:            region,
		profile:           profile,
		log:               log,
	}
}

func (s *Cloudformation) Name() string {
	return "cloudformation"
}

func (s *Cloudformation) Region() string {
	return s.region
}

func (s *Cloudformation) Profile() string {
	return s.profile
}

func (s *Cloudformation) ResourceTypes() []string {
	return []string{
		"stack",
	}
}

func (s *Cloudformation) Fetch(ctx context.Context) (cloud.GraphAPI, error) {
	if s.IsSyncDisabled() {
		return graph.NewGraph(), nil
	}

	allErrors := new(fetch.Error)

	gph, err := s.fetcher.Fetch(context.WithValue(ctx, "region", s.region))
	defer s.fetcher.Reset()

	for _, e := range *fetch.WrapError(err) {
		switch {
		case e == nil:
			continue
		case awsfetch.IsAccessDenied(e):
			allErrors.Add(cloud.ErrFetchAccessDenied)
		default:
			allErrors.Add(e)
		}
	}

	if err := gph.AddResource(graph.InitResource(cloud.Region, s.region)); err != nil {
		return gph, err
	}

	snap := gph.AsRDFGraphSnaphot()

	errc := make(chan error)
	var wg sync.WaitGroup
	if getBool(s.config, "aws.cloudformation.stack.sync", true) {
		list, err := s.fetcher.Get("stack_objects")
		if err != nil {
			return gph, err
		}
		objects, ok := list.([]cloudformationtypes.Stack)
		if !ok {
			return gph, errors.New("cannot cast to '[]cloudformationtypes.Stack' type from fetch context")
		}
		for _, r := range objects {
			for _, fn := range addParentsFns["stack"] {
				wg.Add(1)
				go func(f addParentFn, snap tstore.RDFGraph, region string, res cloudformationtypes.Stack) {
					defer wg.Done()
					if err := f(gph, snap, region, res); err != nil {
						errc <- err
					}
				}(fn, snap, s.region, r)
			}
		}
	}

	go func() {
		wg.Wait()
		close(errc)
	}()

	for err := range errc {
		if err != nil {
			allErrors.Add(err)
		}
	}

	if allErrors.Any() {
		return gph, allErrors
	}

	return gph, nil
}

func (s *Cloudformation) FetchByType(ctx context.Context, t string) (cloud.GraphAPI, error) {
	defer s.fetcher.Reset()
	return s.fetcher.FetchByType(context.WithValue(ctx, "region", s.region), t)
}

func (s *Cloudformation) IsSyncDisabled() bool {
	return !getBool(s.config, "aws.cloudformation.sync", true)
}
