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

package commands

import (
	"context"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"

	awsconfigv2 "github.com/aws/aws-sdk-go-v2/config"
	"github.com/spf13/cobra"
	"github.com/theazz/awless-ro/aws/services"
	"github.com/theazz/awless-ro/cloud"
	"github.com/theazz/awless-ro/config"
	"github.com/theazz/awless-ro/database"
	"github.com/theazz/awless-ro/logger"
	"github.com/theazz/awless-ro/sync"
)

func applyHooks(funcs ...func(*cobra.Command, []string) error) func(*cobra.Command, []string) {
	return func(cmd *cobra.Command, args []string) {
		// Past this point the command line parsed, so anything that goes wrong from
		// here is a failure of the work rather than a misuse of the command, and
		// printing the flag list over it only buries the message. cobra decides this
		// by looking at the flag when the error comes back, and this hook runs after
		// parsing and before the command body, which is exactly the boundary: a
		// mistyped flag still gets the usage text it needs.
		cmd.SilenceUsage = true

		for _, fn := range funcs {
			if err := fn(cmd, args); err != nil {
				exitOn(err)
			}
		}
	}
}

func initAwlessEnvHook(cmd *cobra.Command, args []string) error {
	// The profile is settled here as well as in applyRegionAndProfilePrecedence
	// below, because a first install resolves a region inside InitAwlessEnv and has
	// to resolve it through the profile that was asked for — otherwise
	// `awless-ro -p beta ...` on a fresh machine asks for a region interactively
	// while `[profile beta]` already names one, and stores the answer.
	//
	// The stored value is deliberately not consulted: the configuration is not
	// loaded yet. That loses nothing, because the only thing InitAwlessEnv does with
	// the profile happens when there is no stored configuration to consult. The
	// flags, by contrast, are already parsed — PersistentPreRun runs after parsing.
	// Everything else keeps coming from applyRegionAndProfilePrecedence, which runs
	// straight after with the configuration loaded.
	firstRunProfile, explicit, _ := chooseAWSProfile(awsProfileGlobalFlag, "")
	if !explicit {
		firstRunProfile = ""
	}

	if err := config.InitAwlessEnv(firstRunProfile); err != nil {
		return fmt.Errorf("cannot init awless-ro environment: %s", err)
	}

	return applyRegionAndProfilePrecedence()
}

var profileOverridenThrough, regionOverridenThrough string

// chooseAWSProfile settles which AWS profile a run uses, and whether that was a
// choice or a fallback.
//
// The order is the one the tool has always applied: the --aws-profile flag, then
// AWS_DEFAULT_PROFILE, then AWS_PROFILE, then the stored aws.profile. `through`
// describes the winner for the "profile precedence:" message and is empty when the
// value came from the stored configuration or from nothing at all.
//
// `explicit` is the interesting part. It is false only when nothing names a profile
// and the answer is the implicit "default". Everything else — including `-p default`
// and a stored aws.profile of some other name — counts as chosen, because the
// credential chain and the profile the run reports have to name the same thing: the
// profile decides the local graph directory, so disagreeing would store one account's
// resources under another account's name. For the implicit default the SDK's own
// order is accepted instead (environment keys first), which is both the documented
// AWS behaviour and the only way credentials in the environment can work at all.
func chooseAWSProfile(flag, stored string) (profile string, explicit bool, through string) {
	if flag != "" {
		return flag, true, "command flag"
	}
	if envProfile := os.Getenv("AWS_DEFAULT_PROFILE"); envProfile != "" {
		return envProfile, true, "AWS_DEFAULT_PROFILE variable"
	}
	if envProfile := os.Getenv("AWS_PROFILE"); envProfile != "" {
		return envProfile, true, "AWS_PROFILE variable"
	}
	if stored != "" && stored != config.DefaultAWSProfile {
		return stored, true, ""
	}
	return config.DefaultAWSProfile, false, ""
}

func applyRegionAndProfilePrecedence() error {
	chosen, _, through := chooseAWSProfile(awsProfileGlobalFlag, config.GetAWSProfile())
	if through != "" {
		if err := config.SetVolatile(config.ProfileConfigKey, chosen); err != nil {
			return err
		}
		profileOverridenThrough = through
	}

	profile := config.GetAWSProfile()

	if region, embedded, err := hasEmbeddedRegionInSharedConfigForProfile(profile); err == nil {
		if embedded {
			if e := config.SetVolatile(config.RegionConfigKey, region); e != nil {
				return e
			}
			regionOverridenThrough = fmt.Sprintf("profile '%s' (see AWS config files $HOME/.aws/{credentials,config})", profile)
		} else {
			regionOverridenThrough = ""
		}
	} else {
		return err
	}

	if awsRegionGlobalFlag != "" {
		if err := config.SetVolatile(config.RegionConfigKey, awsRegionGlobalFlag); err != nil {
			return err
		}
		regionOverridenThrough = "command flag"
	} else if envRegion := os.Getenv("AWS_DEFAULT_REGION"); envRegion != "" {
		if err := config.SetVolatile(config.RegionConfigKey, envRegion); err != nil {
			return err
		}
		regionOverridenThrough = "AWS_DEFAULT_REGION variable"
	}

	return nil
}

func notifyOnRegionOrProfilePrecedenceHook(cmd *cobra.Command, _ []string) error {
	applyRegionAndProfilePrecedence()

	if m := profileOverridenThrough; len(m) > 0 {
		logger.Infof("profile precedence: '%s' loaded through %s", config.GetAWSProfile(), m)
	}
	if m := regionOverridenThrough; len(m) > 0 {
		logger.Infof("region precedence: '%s' loaded through %s", config.GetAWSRegion(), m)
	}

	if cmd.Name() == "switch" {
		// switch refuses -p and -r (switchFlagsError), so an override here always
		// comes from the environment or from the active profile's own region.
		if requested, effective := switchRequestedProfile, config.GetAWSProfile(); requested != "" && requested != effective {
			logger.Warningf("switch saved profile '%s', but %s overrides it with '%s'; %s", requested, profileOverridenThrough, effective, switchOverrideAdvice("profile", profileOverridenThrough, requested))
		}
		if requested, effective := switchRequestedRegion, config.GetAWSRegion(); requested != "" && requested != effective {
			logger.Warningf("switch saved region '%s', but %s overrides it with '%s'; %s", requested, regionOverridenThrough, effective, switchOverrideAdvice("region", regionOverridenThrough, requested))
		}
	}

	return nil
}

// switchOverrideAdvice says what to do when source overrides the value switch just
// saved. Only unsetting a variable, or removing the region from a profile, makes the
// saved value itself take effect; exporting the variable or passing the flag sets the
// same value from somewhere else, so the advice does not claim they "use" it.
func switchOverrideAdvice(setting, source, requested string) string {
	flag := "--aws-" + setting
	value := shellQuoteArg(requested)
	switch source {
	case "AWS_PROFILE variable", "AWS_DEFAULT_PROFILE variable", "AWS_DEFAULT_REGION variable":
		variable := strings.TrimSuffix(source, " variable")
		return fmt.Sprintf("unset %s to use it, export %s=%s, or pass %s %s on each command", variable, variable, value, flag, value)
	default:
		// The region set in the active profile is applied on every run
		// (applyRegionAndProfilePrecedence, SetVolatile), so a region saved by switch
		// is never used together with that profile.
		return fmt.Sprintf("a region saved by switch is not used while that profile sets one: "+
			"remove `region` from the profile, export AWS_DEFAULT_REGION=%s, or pass %s %s on each command", value, flag, value)
	}
}

// shellSafeArg matches values a POSIX shell reads literally.
var shellSafeArg = regexp.MustCompile(`^[A-Za-z0-9_@%+=:,./-]+$`)

// shellQuoteArg quotes s, when it needs it, so that advice can be copied into a shell
// as is. Profile names come from the user's own ~/.aws/config, so this is about a
// correct copy-paste rather than untrusted input.
func shellQuoteArg(s string) string {
	if shellSafeArg.MatchString(s) {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

func initCloudServicesHook(cmd *cobra.Command, args []string) error {
	if localGlobalFlag {
		return nil
	}

	profile, region := config.GetAWSProfile(), config.GetAWSRegion()

	// Asked again rather than read off awsProfileExplicit, because `switch` changes
	// the stored profile in the command body and then runs this hook afterwards: the
	// answer from the pre-run hook would be about the profile that was in force
	// before the switch.
	_, profileExplicit, _ := chooseAWSProfile(awsProfileGlobalFlag, profile)

	logger.Verbosef("awless-ro %s - loading AWS session with profile '%s' and region '%s'", config.Version, profile, region)

	if err := awsservices.Init(profile, profileExplicit, region, config.GetConfigWithPrefix("aws."), logger.DefaultLogger, config.SetProfileCallback, networkMonitorFlag); err != nil {
		return err
	}

	if config.TriggerSyncOnConfigUpdate && !strings.HasPrefix(cmd.Name(), "sync") {
		var services []cloud.Service
		for _, s := range cloud.ServiceRegistry {
			services = append(services, s)
		}
		if !noSyncGlobalFlag {
			logger.Infof("Syncing new region '%s'... (disable with --no-sync global flag)", region)
			sync.NewSyncer(logger.DefaultLogger).Sync(services...)
		}
	}

	return nil
}

func includeHookIf(cond *bool, hook func(*cobra.Command, []string) error) func(*cobra.Command, []string) error {
	return func(c *cobra.Command, args []string) error {
		if *cond {
			return hook(c, args)
		}
		return nil
	}
}

func initSyncerHook(cmd *cobra.Command, args []string) error {
	if noSyncGlobalFlag {
		sync.DefaultSyncer = sync.NoOpSyncer()
	} else {
		sync.DefaultSyncer = sync.NewSyncer(logger.DefaultLogger)
	}
	return nil
}

func initLoggerHook(cmd *cobra.Command, args []string) error {
	var flag int
	if verboseGlobalFlag {
		flag = logger.VerboseF
	}
	if extraVerboseGlobalFlag {
		flag = flag | logger.ExtraVerboseF
	}

	logger.DefaultLogger.SetVerbose(flag)
	if silentGlobalFlag {
		logger.DefaultLogger = logger.DiscardLogger
	}
	return nil
}

func onVersionUpgrade(cmd *cobra.Command, args []string) error {
	var lastVersion string
	if derr := database.Execute(func(db *database.DB) (err error) {
		lastVersion, err = db.GetStringValue("current.version")
		return
	}); derr != nil {
		fmt.Printf("cannot verify stored version in db: %s\n", derr)
	}

	if config.IsSemverUpgrade(lastVersion, config.Version) {
		if err := database.Execute(func(db *database.DB) error {
			return db.SetStringValue("current.version", config.Version)
		}); err != nil {
			fmt.Printf("cannot store upgraded version in db: %s\n", err)
		}
		logger.Infof("You have just upgraded awless-ro from %s to %s", lastVersion, config.Version)
		logger.Infof("Check out %s latest features at https://github.com/theazz/awless-ro/blob/master/CHANGELOG.md", config.Version)
	}

	return nil
}

func networkMonitorHook(cmd *cobra.Command, args []string) error {
	if networkMonitorFlag {
		awsservices.DefaultNetworkMonitor.DisplayStats(os.Stderr)
	}
	return nil
}

func firstInstallDoneHook(cmd *cobra.Command, args []string) error {
	// Keyed off the first install itself, not off the sync flag. The flag used to
	// stand in for "this was the first run", which only worked while the first run
	// always synced.
	if config.AwlessFirstInstall {
		fmt.Fprintln(os.Stderr, "\nAll done. Enjoy!")
		fmt.Fprintln(os.Stderr, "You can review and configure awless-ro with `awless-ro config`")
		fmt.Fprintln(os.Stderr)
		fmt.Fprintf(os.Stderr, "Now running: `%s`\n", cmd.CommandPath())
	}
	return nil
}

// hasEmbeddedRegionInSharedConfigForProfile reports the region the named profile
// pins in ~/.aws/{config,credentials}, if any. It is loaded without credential
// resolution because only the region is wanted here; asking for credentials
// would prompt for an MFA code just to answer a question about a config file.
//
// A profile that is not defined is not a failure, it is the answer "no region".
// Treating it as an error made the very first run fail before it got anywhere near
// resolving credentials: with no ~/.aws at all, the SDK reports that the default
// profile does not exist, and the command died there instead of offering to create
// one.
func hasEmbeddedRegionInSharedConfigForProfile(profile string) (string, bool, error) {
	cfg, err := awsconfigv2.LoadDefaultConfig(context.Background(),
		awsconfigv2.WithSharedConfigProfile(profile),
	)
	if err != nil {
		// Matched by value, not by pointer: the SDK declares Error() on the value
		// receiver and returns the struct itself, so errors.As with a **T target
		// silently never matches.
		var missing awsconfigv2.SharedConfigProfileNotExistError
		if errors.As(err, &missing) {
			return "", false, nil
		}
		return "", false, fmt.Errorf("cannot check profile '%s' has embedded region in shared config file: %s", profile, err)
	}
	return cfg.Region, cfg.Region != "", nil
}
