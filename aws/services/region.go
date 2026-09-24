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

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/feature/ec2/imds"

	awsconfig "github.com/theazz/awless-ro/aws/config"
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

// Resolving credentials lives in aws/credentials. It used to be a builder here, but
// the interesting part of it — caching temporary credentials across processes,
// deciding whether a missing profile is worth offering to create — is a subject of
// its own and had nothing to do with constructing service clients.
