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
	"fmt"
	"net/http"
	"os"
	"time"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials/stscreds"
	"github.com/aws/aws-sdk-go-v2/feature/ec2/imds"

	awsconfig "github.com/theazz/awless-ro/aws/config"
	"github.com/theazz/awless-ro/logger"
)

// ResolveRegionFromEnv works out a region to start from on first run: the one
// the environment or the shared config already names, failing that the one this
// machine runs in if it is an EC2 instance, and failing that it asks.
func ResolveRegionFromEnv() (region string) {
	ctx := context.Background()

	cfg, err := config.LoadDefaultConfig(ctx,
		config.WithHTTPClient(&http.Client{Timeout: 1 * time.Second}),
	)
	if err == nil {
		region = cfg.Region
	}

	if awsconfig.IsValidRegion(region) {
		fmt.Fprintf(os.Stderr, "Found existing AWS region '%s'. Setting it as your default region.\n", region)
	} else if err == nil {
		out, merr := imds.NewFromConfig(cfg).GetRegion(ctx, &imds.GetRegionInput{})
		if merr == nil && awsconfig.IsValidRegion(out.Region) {
			fmt.Fprintf(os.Stderr, "Found AWS region '%s' from local EC2 instance metadata. Setting it as your default region.\n", out.Region)
			region = out.Region
		}
	}

	if !awsconfig.IsValidRegion(region) {
		region = awsconfig.StdinRegionSelector()
		fmt.Println()
	}

	return
}

// configResolver builds the aws.Config every service client is created from.
type configResolver struct {
	region, profile           string
	logger                    *logger.Logger
	enableNetworkMonitor      bool
	enableCredentialResolvers bool
}

func newConfigResolver() *configResolver {
	return &configResolver{logger: logger.DiscardLogger}
}

func (s *configResolver) withRegion(region string) *configResolver {
	s.region = region
	return s
}

func (s *configResolver) withProfile(profile string) *configResolver {
	s.profile = profile
	return s
}

// withCredentialResolvers makes resolve verify up front that credentials can
// actually be obtained, so a missing or broken profile is reported before any
// command starts fetching instead of surfacing mid-sync.
func (s *configResolver) withCredentialResolvers() *configResolver {
	s.enableCredentialResolvers = true
	return s
}

func (s *configResolver) withLogger(l *logger.Logger) *configResolver {
	s.logger = l
	return s
}

func (s *configResolver) withNetworkMonitor(enable bool) *configResolver {
	s.enableNetworkMonitor = enable
	return s
}

func (s *configResolver) resolve() (awssdk.Config, error) {
	ctx := context.Background()

	opts := []func(*config.LoadOptions) error{
		// The shared config files are what carry named profiles, assume-role
		// chains and SSO sessions, and they are the normal way users configure
		// AWS, so they are always consulted.
		config.WithSharedConfigProfile(s.profile),
		// MFA codes are read from the terminal, the same as the AWS CLI does.
		config.WithAssumeRoleCredentialOptions(func(o *stscreds.AssumeRoleOptions) {
			o.TokenProvider = stscreds.StdinTokenProvider
		}),
	}
	if s.region != "" {
		opts = append(opts, config.WithRegion(s.region))
	}
	if s.enableNetworkMonitor {
		opts = append(opts, config.WithAPIOptions(DefaultNetworkMonitor.APIOptions()))
	}

	cfg, err := config.LoadDefaultConfig(ctx, opts...)
	if err != nil {
		return cfg, err
	}

	if s.enableCredentialResolvers {
		if cfg.Credentials == nil {
			return cfg, fmt.Errorf("no AWS credentials found for profile '%s'", s.profile)
		}
		if _, err := cfg.Credentials.Retrieve(ctx); err != nil {
			return cfg, err
		}
	}

	return cfg, nil
}
