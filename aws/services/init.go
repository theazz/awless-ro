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

	awscredentials "github.com/theazz/awless-ro/aws/credentials"
	awsfetch "github.com/theazz/awless-ro/aws/fetch"
	"github.com/theazz/awless-ro/cloud"
	"github.com/theazz/awless-ro/logger"
)

var (
	AccessService, InfraService, StorageService, MessagingService, DnsService, LambdaService, MonitoringService, CdnService, CloudformationService cloud.Service
)

func Init(profile, region string, extraConf map[string]interface{}, log *logger.Logger, profileSetterCallback func(val string) error, enableNetworkMonitor bool) error {
	if region == "" {
		return errors.New("empty AWS region. Set it with `awless-ro config set aws.region`")
	}

	params := awscredentials.Params{
		Profile:     profile,
		Region:      region,
		CacheDir:    awscredentials.CacheDir(),
		AllowPrompt: true,
		Log:         log,
	}
	if enableNetworkMonitor {
		params.APIOptions = DefaultNetworkMonitor.APIOptions()
	}

	cfg, err := awscredentials.Resolve(context.Background(), params)
	if err != nil {
		return err
	}

	AccessService = NewAccess(cfg, profile, extraConf, log)
	InfraService = NewInfra(cfg, profile, extraConf, log)
	StorageService = NewStorage(cfg, profile, extraConf, log)
	MessagingService = NewMessaging(cfg, profile, extraConf, log)
	DnsService = NewDns(cfg, profile, extraConf, log)
	LambdaService = NewLambda(cfg, profile, extraConf, log)
	MonitoringService = NewMonitoring(cfg, profile, extraConf, log)
	CdnService = NewCdn(cfg, profile, extraConf, log)
	CloudformationService = NewCloudformation(cfg, profile, extraConf, log)

	cloud.ServiceRegistry[InfraService.Name()] = InfraService
	cloud.ServiceRegistry[AccessService.Name()] = AccessService
	cloud.ServiceRegistry[StorageService.Name()] = StorageService
	cloud.ServiceRegistry[MessagingService.Name()] = MessagingService
	cloud.ServiceRegistry[DnsService.Name()] = DnsService
	cloud.ServiceRegistry[LambdaService.Name()] = LambdaService
	cloud.ServiceRegistry[MonitoringService.Name()] = MonitoringService
	cloud.ServiceRegistry[CdnService.Name()] = CdnService
	cloud.ServiceRegistry[CloudformationService.Name()] = CloudformationService

	return nil
}

// EC2API hands out the EC2 client the infra service was built with, for the one
// command that needs to call EC2 directly instead of through a fetcher: searching for
// images is a question about the public AMI catalogue, not about this account's
// resources, so it has nothing to put in the graph.
//
// An accessor rather than letting callers reach through InfraService, so that the
// shape of Infra stays this package's business.
func EC2API() (awsfetch.Ec2API, error) {
	infra, ok := InfraService.(*Infra)
	if !ok {
		return nil, errors.New("AWS services are not initialised")
	}
	return infra.Ec2API, nil
}

func getBool(m map[string]interface{}, key string, def bool) bool {
	if b, ok := m[key].(bool); ok {
		return b
	}
	return def
}
