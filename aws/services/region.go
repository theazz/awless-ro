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
	"net/http"
	"os"
	"time"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/feature/ec2/imds"

	awsconfig "github.com/theazz/awless-ro/aws/config"
)

// regionSelector asks the user for a region. A variable so that a test can prove the
// interactive prompt is never reached, instead of hanging on stdin waiting for it.
var regionSelector = awsconfig.StdinRegionSelector

// ResolveRegionFromEnv works out a region to start from on first run: the one
// the environment or the shared config already names, failing that the one this
// machine runs in if it is an EC2 instance, and failing that it asks.
//
// profile is the profile the run chose, or empty when it chose none. It has to be
// passed in: without it, a first run of `awless-ro -p beta ...` could not see the
// region `[profile beta]` already names and asked for one interactively, then stored
// that answer. Only a chosen profile is passed, because pinning the implicit default
// would stop the environment from answering at all.
func ResolveRegionFromEnv(profile string) (region string) {
	ctx := context.Background()

	cfg, err := loadConfigForRegion(ctx, profile)
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
		region = regionSelector()
		fmt.Println()
	}

	return
}

// loadConfigForRegion reads just enough configuration to answer "which region", with
// a short timeout because the chain may try to reach instance metadata.
//
// A profile that does not exist is not a failure here, it is the answer "no region
// from the profile": a stale name in AWS_PROFILE or a typo after --aws-profile should
// still let the environment and the shared default answer. Same treatment as
// hasEmbeddedRegionInSharedConfigForProfile in commands/hooks.go, which asks the same
// question later in the run.
func loadConfigForRegion(ctx context.Context, profile string) (awssdk.Config, error) {
	opts := []func(*config.LoadOptions) error{
		config.WithHTTPClient(&http.Client{Timeout: 1 * time.Second}),
	}
	if profile == "" {
		return config.LoadDefaultConfig(ctx, opts...)
	}

	cfg, err := config.LoadDefaultConfig(ctx, append(opts, config.WithSharedConfigProfile(profile))...)
	if err == nil {
		return cfg, nil
	}
	// Matched by value, not by pointer: the SDK declares Error() on the value
	// receiver and returns the struct itself, so errors.As with a **T target
	// silently never matches.
	var missing config.SharedConfigProfileNotExistError
	if errors.As(err, &missing) {
		return config.LoadDefaultConfig(ctx, opts...)
	}
	return cfg, err
}

// Resolving credentials lives in aws/credentials. It used to be a builder here, but
// the interesting part of it — caching temporary credentials across processes,
// deciding whether a missing profile is worth offering to create — is a subject of
// its own and had nothing to do with constructing service clients.
