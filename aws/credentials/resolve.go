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

// Package awscredentials resolves the AWS credentials every command runs with.
//
// Resolution itself is the SDK's: environment, shared config and credentials files,
// SSO, assume-role chains, container and instance roles, in that order. Three things
// are added on top.
//
// Temporary credentials are cached on disk, because a CLI is a new process every
// time and an assume-role profile with MFA would otherwise ask for a token code on
// every command.
//
// Credentials are retrieved once during start-up rather than lazily on the first API
// call, so a missing or broken profile is reported before anything else happens
// instead of surfacing halfway through a sync.
//
// When there are none at all and someone is watching, the access keys are asked for
// and written to ~/.aws/credentials, which is the file every other AWS tool reads.
package awscredentials

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials/stscreds"
	"github.com/aws/smithy-go/middleware"

	awsconfig "github.com/theazz/awless-ro/aws/config"
	"github.com/theazz/awless-ro/logger"
)

// Params describes what to resolve.
//
// Profile and Region arrive already decided: the precedence between the --aws-profile
// flag, AWS_PROFILE, AWS_DEFAULT_PROFILE and the stored configuration is settled in
// commands/hooks.go, and repeating it here would give us two answers to the same
// question.
type Params struct {
	Profile string
	Region  string

	// ProfileExplicit says whether Profile was actually chosen by somebody — the
	// --aws-profile flag, AWS_PROFILE, AWS_DEFAULT_PROFILE or a stored aws.profile
	// naming something other than the implicit default — rather than being the
	// default we fall back to. commands/hooks.go owns that question.
	//
	// It decides whether the shared-config profile is pinned. The SDK treats a
	// programmatically set profile as exclusive: it then resolves credentials from
	// that profile alone and never consults AWS_ACCESS_KEY_ID and
	// AWS_SECRET_ACCESS_KEY. Pinning the implicit "default" therefore broke the
	// arrangement every CI job and container uses — keys in the environment and no
	// ~/.aws at all — with either "failed to get shared config profile, default" or
	// a walk all the way down the chain to instance metadata. With nothing chosen
	// the option is omitted and the SDK's documented order applies: environment,
	// then shared config, then container and instance roles.
	ProfileExplicit bool

	// APIOptions are passed through to every client, used for the network monitor.
	APIOptions []func(*middleware.Stack) error

	// AllowPrompt permits asking for access keys when none were found. Commands
	// that are not user-facing leave it off.
	AllowPrompt bool

	// CacheDir holds the credentials cache. Empty disables caching.
	CacheDir string

	Log *logger.Logger
}

// Resolve builds the config every service client is created from.
func Resolve(ctx context.Context, p Params) (awssdk.Config, error) {
	log := p.Log
	if log == nil {
		log = logger.DiscardLogger
	}

	cfg, err := loadAndVerify(ctx, p, log)
	if err == nil {
		return cfg, nil
	}

	// Whether to offer to create a profile turns on whether one exists, not on what
	// the failure said. The SDK gives no usable signal here: with nothing
	// configured at all, the error that surfaces is the last link in the chain
	// failing, which reads "no EC2 IMDS role found". Matching that text would break
	// the day the wording changes, and it says nothing about whether the user has a
	// half-configured profile.
	//
	// The distinction matters, because offering to type access keys is bad advice
	// when the real problem is an expired SSO session or a role that will not
	// assume.
	if isConfigured(p.Profile, p.ProfileExplicit) {
		return cfg, fmt.Errorf("AWS credentials for profile %q are configured but cannot be used: %w", p.Profile, err)
	}

	if !p.AllowPrompt {
		return cfg, notFoundError(p.Profile, nil)
	}

	profile, perr := newProfile(newStdTerminal(), p.Profile)
	if perr != nil {
		return cfg, notFoundError(p.Profile, perr)
	}

	// Reloaded rather than patched: the stored profile may carry a region, and the
	// SDK is what knows how to read it. The profile just written is by definition a
	// choice, so it is pinned even if the run started with none.
	p.Profile = profile
	p.ProfileExplicit = true
	cfg, err = loadAndVerify(ctx, p, log)
	if err != nil {
		return cfg, fmt.Errorf("the credentials just stored for profile %q do not work: %w", profile, err)
	}
	return cfg, nil
}

// loadAndVerify resolves and then retrieves once, so that a broken profile is
// reported during start-up rather than surfacing halfway through a sync.
func loadAndVerify(ctx context.Context, p Params, log *logger.Logger) (awssdk.Config, error) {
	cfg, err := load(ctx, p)
	if err != nil {
		return cfg, err
	}
	cfg.Credentials = newDiskCache(cfg.Credentials, p.CacheDir, p.Profile, log)
	if _, err := cfg.Credentials.Retrieve(ctx); err != nil {
		return cfg, err
	}
	return cfg, nil
}

// isConfigured reports whether anything defines credentials for this profile: the
// environment, or a section in the shared config or credentials file.
//
// When the profile was explicitly asked for, only a section counts. Keys in the
// environment say nothing about it — resolution was pinned to that profile and never
// looked at them — so counting them would answer a request for a profile that does
// not exist with "configured but cannot be used" instead of naming it.
func isConfigured(profile string, explicit bool) bool {
	if !explicit && (os.Getenv("AWS_ACCESS_KEY_ID") != "" || os.Getenv("AWS_SECRET_ACCESS_KEY") != "") {
		return true
	}
	if profile == "" {
		profile = "default"
	}
	for _, known := range awsconfig.AllProfiles() {
		if known == profile {
			return true
		}
	}
	return false
}

func load(ctx context.Context, p Params) (awssdk.Config, error) {
	opts := []func(*config.LoadOptions) error{
		// MFA token codes are read from the terminal, the same way the AWS CLI does
		// it.
		config.WithAssumeRoleCredentialOptions(func(o *stscreds.AssumeRoleOptions) {
			o.TokenProvider = stscreds.StdinTokenProvider
		}),
	}
	// Only when a profile was chosen; see Params.ProfileExplicit for why.
	if p.ProfileExplicit && p.Profile != "" {
		opts = append(opts, config.WithSharedConfigProfile(p.Profile))
	}
	if p.Region != "" {
		opts = append(opts, config.WithRegion(p.Region))
	}
	if len(p.APIOptions) > 0 {
		opts = append(opts, config.WithAPIOptions(p.APIOptions))
	}
	return config.LoadDefaultConfig(ctx, opts...)
}

// notFoundError explains what to do about having no credentials.
//
// When something specific went wrong while asking for them, that is reported on its
// own. Adding the generic advice underneath would contradict it: the advice names the
// profile we started with, while the user may have just typed a different one.
func notFoundError(profile string, cause error) error {
	if cause != nil && !errors.Is(cause, ErrNoTerminal) {
		return cause
	}
	if profile == "" {
		profile = "default"
	}
	return fmt.Errorf("no AWS credentials found for profile %q.\n"+
		"Set them up with `aws configure --profile %s`, or point awless-ro at another profile with --aws-profile,\n"+
		"or export AWS_ACCESS_KEY_ID and AWS_SECRET_ACCESS_KEY", profile, profile)
}

// CacheDir is where temporary credentials are kept, under the awless-ro cache
// directory that config.InitAwlessEnv exports.
func CacheDir() string {
	base := os.Getenv("__AWLESS_CACHE")
	if base == "" {
		return ""
	}
	return filepath.Join(base, "credentials")
}
