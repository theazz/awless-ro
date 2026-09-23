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

// A full-sync benchmark: fetch three services off mocked AWS APIs, build their
// graphs and marshal them to N-Triples on disk.
//
// Upstream had this benchmark in sync/sync_bench_test.go, where it could never
// run: it built awsservices.Infra from outside the package, so the unexported
// fetcher field stayed nil and the first iteration panicked. `go test` without
// -bench only compiles benchmarks, which is why nobody noticed. Living inside
// the package fixes that, and lets the benchmark reuse the generated mocks
// instead of carrying its own copy of the AWS fixtures.
//
// The fixtures are generated from a size, so the benchmark can be pointed at a
// bigger account by bumping the constants rather than by pasting more literals.

import (
	"fmt"
	"os"
	"testing"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	autoscalingtypes "github.com/aws/aws-sdk-go-v2/service/autoscaling/types"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	ecrtypes "github.com/aws/aws-sdk-go-v2/service/ecr/types"
	elbv2types "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2/types"
	iamtypes "github.com/aws/aws-sdk-go-v2/service/iam/types"
	rdstypes "github.com/aws/aws-sdk-go-v2/service/rds/types"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"

	awsfetch "github.com/theazz/awless-ro/aws/fetch"
	"github.com/theazz/awless-ro/cloud"
	"github.com/theazz/awless-ro/fetch"
	"github.com/theazz/awless-ro/sync"
)

const (
	benchRegion  = "eu-west-1"
	benchProfile = "bench"

	benchVpcs               = 4
	benchSubnetsPerVpc      = 4
	benchInstancesPerSubnet = 8
	benchGroupsPerVpc       = 3
	benchLoadBalancers      = 6
	benchUsers              = 50
	benchGroups             = 10
	benchPolicies           = 20
	benchBuckets            = 10
	benchObjectsPerBucket   = 20
)

func BenchmarkSync(b *testing.B) {
	dir, err := os.MkdirTemp("", "awlessbench_")
	if err != nil {
		b.Fatal(err)
	}
	defer os.RemoveAll(dir)

	// repo.New() reads __AWLESS_HOME when the syncer is built, so this has to
	// be set first.
	b.Setenv("__AWLESS_HOME", dir)
	syncer := sync.NewSyncer()

	infra, access, storage := benchServices()

	// fetchTargetsAndAddRelations reaches for the package-level InfraService
	// rather than for the API it was handed, so the globals have to point at the
	// mocked services too. TestBuildInfraRdfGraph does the same.
	InfraService, AccessService, StorageService = infra, access, storage

	// One untimed pass, so that a benchmark measuring three empty graphs fails
	// loudly instead of reporting a flatteringly small number.
	graphs, err := syncer.Sync(infra, access, storage)
	if err != nil {
		b.Fatal(err)
	}
	for name, typ := range map[string]string{
		"infra":   cloud.Instance,
		"access":  cloud.User,
		"storage": cloud.Bucket,
	} {
		g, ok := graphs[name]
		if !ok {
			b.Fatalf("%s service produced no graph", name)
		}
		res, err := g.Find(cloud.NewQuery(typ))
		if err != nil {
			b.Fatal(err)
		}
		if len(res) == 0 {
			b.Fatalf("%s service produced no %s", name, typ)
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := syncer.Sync(infra, access, storage); err != nil {
			b.Fatal(err)
		}
	}
}

func benchServices() (*Infra, *Access, *Storage) {
	ec2Mock := benchEc2Mock()
	elbv2Mock := benchElbv2Mock()
	elbMock := &mockElb{}
	rdsMock := &mockRds{dbinstances: benchDBInstances()}
	ecrMock := &mockEcr{repositorys: benchRepositories()}
	ecsMock := &mockEcs{}
	acmMock := &mockAcm{}
	autoscalingMock := &mockAutoscaling{launchconfigurations: benchLaunchConfigurations()}

	infra := &Infra{
		Ec2API:         ec2Mock,
		Elbv2API:       elbv2Mock,
		ElbAPI:         elbMock,
		RdsAPI:         rdsMock,
		EcrAPI:         ecrMock,
		EcsAPI:         ecsMock,
		AcmAPI:         acmMock,
		AutoscalingAPI: autoscalingMock,
		region:         benchRegion,
		profile:        benchProfile,
		fetcher: fetch.NewFetcher(awsfetch.BuildInfraFetchFuncs(awsfetch.NewConfig(&awsfetch.AWSAPI{
			Ec2:         ec2Mock,
			Elbv2:       elbv2Mock,
			Elb:         elbMock,
			Rds:         rdsMock,
			Ecr:         ecrMock,
			Ecs:         ecsMock,
			Acm:         acmMock,
			Autoscaling: autoscalingMock,
		}))),
	}

	iamMock := benchIamMock()
	access := &Access{
		IamAPI:  iamMock,
		region:  "global",
		profile: benchProfile,
		fetcher: fetch.NewFetcher(awsfetch.BuildAccessFetchFuncs(awsfetch.NewConfig(&awsfetch.AWSAPI{
			Iam: iamMock,
		}))),
	}

	s3Mock := benchS3Mock()
	storage := &Storage{
		S3API:   s3Mock,
		region:  benchRegion,
		profile: benchProfile,
		fetcher: fetch.NewFetcher(awsfetch.BuildStorageFetchFuncs(awsfetch.NewConfig(&awsfetch.AWSAPI{
			S3: s3Mock,
		}))),
	}

	return infra, access, storage
}

func benchEc2Mock() *mockEc2 {
	m := &mockEc2{
		availabilityzones: []ec2types.AvailabilityZone{
			{ZoneName: awssdk.String(benchRegion + "a"), RegionName: awssdk.String(benchRegion), State: ec2types.AvailabilityZoneStateAvailable},
			{ZoneName: awssdk.String(benchRegion + "b"), RegionName: awssdk.String(benchRegion), State: ec2types.AvailabilityZoneStateAvailable},
		},
		keypairinfos: []ec2types.KeyPairInfo{{KeyName: awssdk.String("bench_key")}},
		images: []ec2types.Image{
			{
				ImageId:      awssdk.String("ami-bench"),
				Name:         awssdk.String("bench-image"),
				Architecture: ec2types.ArchitectureValuesX8664,
				Hypervisor:   ec2types.HypervisorTypeXen,
				CreationDate: awssdk.String("2010-04-01T12:05:01.000Z"),
			},
		},
	}

	for v := 0; v < benchVpcs; v++ {
		vpcID := fmt.Sprintf("vpc_%d", v)
		m.vpcs = append(m.vpcs, ec2types.Vpc{VpcId: awssdk.String(vpcID), CidrBlock: awssdk.String("10.0.0.0/16")})
		m.internetgateways = append(m.internetgateways, ec2types.InternetGateway{
			InternetGatewayId: awssdk.String(fmt.Sprintf("igw_%d", v)),
			Attachments:       []ec2types.InternetGatewayAttachment{{VpcId: awssdk.String(vpcID)}},
		})

		var groupIDs []string
		for g := 0; g < benchGroupsPerVpc; g++ {
			groupID := fmt.Sprintf("sg_%d_%d", v, g)
			groupIDs = append(groupIDs, groupID)
			m.securitygroups = append(m.securitygroups, ec2types.SecurityGroup{
				GroupId:   awssdk.String(groupID),
				GroupName: awssdk.String(groupID),
				VpcId:     awssdk.String(vpcID),
			})
		}

		for s := 0; s < benchSubnetsPerVpc; s++ {
			subnetID := fmt.Sprintf("sub_%d_%d", v, s)
			m.subnets = append(m.subnets, ec2types.Subnet{
				SubnetId:  awssdk.String(subnetID),
				VpcId:     awssdk.String(vpcID),
				CidrBlock: awssdk.String(fmt.Sprintf("10.0.%d.0/24", s)),
			})
			m.routetables = append(m.routetables, ec2types.RouteTable{
				RouteTableId: awssdk.String(fmt.Sprintf("rt_%d_%d", v, s)),
				VpcId:        awssdk.String(vpcID),
				Associations: []ec2types.RouteTableAssociation{{
					RouteTableId: awssdk.String(fmt.Sprintf("rt_%d_%d", v, s)),
					SubnetId:     awssdk.String(subnetID),
				}},
			})

			for i := 0; i < benchInstancesPerSubnet; i++ {
				id := fmt.Sprintf("inst_%d_%d_%d", v, s, i)
				m.instances = append(m.instances, ec2types.Instance{
					InstanceId:         awssdk.String(id),
					SubnetId:           awssdk.String(subnetID),
					VpcId:              awssdk.String(vpcID),
					ImageId:            awssdk.String("ami-bench"),
					KeyName:            awssdk.String("bench_key"),
					InstanceType:       ec2types.InstanceTypeT2Micro,
					Architecture:       ec2types.ArchitectureValuesX8664,
					Hypervisor:         ec2types.HypervisorTypeXen,
					RootDeviceType:     ec2types.DeviceTypeEbs,
					RootDeviceName:     awssdk.String("/dev/xvda"),
					PublicIpAddress:    awssdk.String("1.2.3.4"),
					PrivateIpAddress:   awssdk.String("10.0.0.1"),
					PublicDnsName:      awssdk.String(id + ".bench.dns"),
					State:              &ec2types.InstanceState{Name: ec2types.InstanceStateNameRunning},
					Placement:          &ec2types.Placement{AvailabilityZone: awssdk.String(benchRegion + "a")},
					IamInstanceProfile: &ec2types.IamInstanceProfile{Arn: awssdk.String("arn:instance:profile")},
					Tags:               []ec2types.Tag{{Key: awssdk.String("Name"), Value: awssdk.String(id + "-name")}},
					SecurityGroups: []ec2types.GroupIdentifier{
						{GroupId: awssdk.String(groupIDs[i%len(groupIDs)])},
					},
					NetworkInterfaces: []ec2types.InstanceNetworkInterface{
						{NetworkInterfaceId: awssdk.String("eni_" + id)},
					},
				})
				m.volumes = append(m.volumes, ec2types.Volume{
					VolumeId:         awssdk.String("vol_" + id),
					AvailabilityZone: awssdk.String(benchRegion + "a"),
					Size:             awssdk.Int32(8),
				})
			}
		}
	}

	return m
}

func benchElbv2Mock() *mockElbv2 {
	m := &mockElbv2{}
	var listeners []elbv2types.Listener

	for i := 0; i < benchLoadBalancers; i++ {
		arn := fmt.Sprintf("lb_%d", i)
		vpcID := fmt.Sprintf("vpc_%d", i%benchVpcs)
		m.loadbalancers = append(m.loadbalancers, elbv2types.LoadBalancer{
			LoadBalancerArn:  awssdk.String(arn),
			LoadBalancerName: awssdk.String(arn),
			VpcId:            awssdk.String(vpcID),
			SecurityGroups:   []string{fmt.Sprintf("sg_%d_0", i%benchVpcs)},
		})
		m.targetgroups = append(m.targetgroups, elbv2types.TargetGroup{
			TargetGroupArn:   awssdk.String(fmt.Sprintf("tg_%d", i)),
			VpcId:            awssdk.String(vpcID),
			LoadBalancerArns: []string{arn},
		})
		listeners = append(listeners, elbv2types.Listener{
			ListenerArn:     awssdk.String(fmt.Sprintf("list_%d", i)),
			LoadBalancerArn: awssdk.String(arn),
			Port:            awssdk.Int32(443),
		})
	}

	m.manualElbv2Mock = manualElbv2Mock{listeners: listeners}
	return m
}

func benchDBInstances() []rdstypes.DBInstance {
	var out []rdstypes.DBInstance
	for i := 0; i < benchVpcs; i++ {
		out = append(out, rdstypes.DBInstance{
			DBInstanceIdentifier: awssdk.String(fmt.Sprintf("db_%d", i)),
			DBInstanceClass:      awssdk.String("db.t2.micro"),
			Engine:               awssdk.String("postgres"),
		})
	}
	return out
}

func benchRepositories() []ecrtypes.Repository {
	var out []ecrtypes.Repository
	for i := 0; i < benchVpcs; i++ {
		out = append(out, ecrtypes.Repository{
			RepositoryName: awssdk.String(fmt.Sprintf("repo_%d", i)),
			RepositoryArn:  awssdk.String(fmt.Sprintf("arn:repo:%d", i)),
		})
	}
	return out
}

func benchLaunchConfigurations() []autoscalingtypes.LaunchConfiguration {
	var out []autoscalingtypes.LaunchConfiguration
	for i := 0; i < benchVpcs; i++ {
		out = append(out, autoscalingtypes.LaunchConfiguration{
			LaunchConfigurationName: awssdk.String(fmt.Sprintf("lc_%d", i)),
			ImageId:                 awssdk.String("ami-bench"),
			InstanceType:            awssdk.String("t2.micro"),
		})
	}
	return out
}

func benchIamMock() *mockIam {
	var policies []iamtypes.ManagedPolicyDetail
	for i := 0; i < benchPolicies; i++ {
		policies = append(policies, iamtypes.ManagedPolicyDetail{
			PolicyId:   awssdk.String(fmt.Sprintf("policy_%d", i)),
			PolicyName: awssdk.String(fmt.Sprintf("npolicy_%d", i)),
		})
	}

	var groups []iamtypes.GroupDetail
	for i := 0; i < benchGroups; i++ {
		groups = append(groups, iamtypes.GroupDetail{
			GroupId:                 awssdk.String(fmt.Sprintf("group_%d", i)),
			GroupName:               awssdk.String(fmt.Sprintf("ngroup_%d", i)),
			AttachedManagedPolicies: []iamtypes.AttachedPolicy{{PolicyName: awssdk.String(fmt.Sprintf("npolicy_%d", i%benchPolicies))}},
		})
	}

	var roles []iamtypes.RoleDetail
	for i := 0; i < benchGroups; i++ {
		roles = append(roles, iamtypes.RoleDetail{
			RoleId:                  awssdk.String(fmt.Sprintf("role_%d", i)),
			RoleName:                awssdk.String(fmt.Sprintf("nrole_%d", i)),
			AttachedManagedPolicies: []iamtypes.AttachedPolicy{{PolicyName: awssdk.String(fmt.Sprintf("npolicy_%d", i%benchPolicies))}},
		})
	}

	var users []iamtypes.User
	var userDetails []iamtypes.UserDetail
	for i := 0; i < benchUsers; i++ {
		id := fmt.Sprintf("usr_%d", i)
		users = append(users, iamtypes.User{UserId: awssdk.String(id), UserName: awssdk.String(id)})
		userDetails = append(userDetails, iamtypes.UserDetail{
			UserId:                  awssdk.String(id),
			UserName:                awssdk.String(id),
			GroupList:               []string{fmt.Sprintf("ngroup_%d", i%benchGroups)},
			AttachedManagedPolicies: []iamtypes.AttachedPolicy{{PolicyName: awssdk.String(fmt.Sprintf("npolicy_%d", i%benchPolicies))}},
			UserPolicyList:          []iamtypes.PolicyDetail{{PolicyName: awssdk.String(fmt.Sprintf("inline_%d", i))}},
		})
	}

	return &mockIam{manualIamMock: manualIamMock{
		users:                users,
		userdetails:          userDetails,
		groupdetails:         groups,
		roledetails:          roles,
		managedpolicydetails: policies,
	}}
}

func benchS3Mock() *mockS3 {
	buckets := map[string][]s3types.Bucket{}
	objects := map[string][]s3types.Object{}
	grants := map[string][]s3types.Grant{}

	for i := 0; i < benchBuckets; i++ {
		name := fmt.Sprintf("bucket_%d", i)
		buckets[benchRegion] = append(buckets[benchRegion], s3types.Bucket{Name: awssdk.String(name)})
		for o := 0; o < benchObjectsPerBucket; o++ {
			objects[name] = append(objects[name], s3types.Object{Key: awssdk.String(fmt.Sprintf("%s/obj_%d", name, o))})
		}
		grants[name] = []s3types.Grant{{
			Permission: s3types.PermissionRead,
			Grantee:    &s3types.Grantee{ID: awssdk.String(fmt.Sprintf("usr_%d", i%benchUsers))},
		}}
	}

	return &mockS3{manualS3Mock: manualS3Mock{buckets: buckets, objects: objects, grants: grants}}
}
