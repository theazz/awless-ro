package awsconfig

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"text/tabwriter"

	"github.com/chzyer/readline"
)

var AWSHomeDir = func() string {
	var home string
	if runtime.GOOS == "windows" { // Windows
		home = os.Getenv("USERPROFILE")
	} else {
		home = os.Getenv("HOME")
	}
	return filepath.Join(home, ".aws")
}

func ParseRegion(i string) (interface{}, error) {
	if !IsValidRegion(i) {
		return i, fmt.Errorf("'%s' is not a valid region", i)
	}
	return i, nil
}

func ParseInstanceType(i string) (interface{}, error) {
	if !isValidInstanceType(i) {
		return i, fmt.Errorf("'%s' is not a valid instance type", i)
	}
	return i, nil
}

func StdinRegionSelector() string {
	var regionItems []readline.PrefixCompleterInterface
	for _, r := range SuggestedRegions {
		regionItems = append(regionItems, readline.PcItem(r))
	}
	var regionCompleter = readline.NewPrefixCompleter(regionItems...)

	fmt.Println("Please enter one region: (Ctrl+C to quit, Tab for completion)")
	var region string
	rl, err := readline.NewEx(&readline.Config{
		Prompt:       "> ",
		AutoComplete: regionCompleter,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "error while selecting region: %s", err)
		return ""
	}
	defer rl.Close()

	for !IsValidRegion(region) {
		line, err := rl.Readline()
		if err == readline.ErrInterrupt || err == io.EOF {
			os.Exit(1)
		} else if err != nil {
			fmt.Fprintf(os.Stderr, "error while selecting region: %s", err)
			return ""
		}

		region = strings.TrimSpace(line)
		if !IsValidRegion(region) {
			fmt.Fprintf(os.Stderr, "'%s' is not a valid region\n", region)
		}
	}

	return region
}

func StdinInstanceTypeSelector() string {
	fmt.Println("Please choose one instance type")
	fmt.Println()
	fmt.Println("Here are few examples:")

	var instanceType string
	t := tabwriter.NewWriter(os.Stdout, 0, 0, 1, ' ', 0)
	fmt.Fprintln(t, "\tinstance type\tvCPU\tMemory (GiB)")
	fmt.Fprintln(t, "\tt2.nano\t1\t0.5")
	fmt.Fprintln(t, "\tt2.micro\t1\t1")
	fmt.Fprintln(t, "\tt2.small\t1\t2")
	fmt.Fprintln(t, "\tt2.medium\t2\t4")
	fmt.Fprintln(t, "\tt2.large\t2\t8")
	fmt.Fprintln(t, "\tt2.xlarge\t4\t16")
	fmt.Fprintln(t, "\tt2.2xlarge\t8\t32")
	fmt.Fprintln(t, "\tm4.large\t2\t8")
	fmt.Fprintln(t, "\tm4.xlarge\t4\t16")
	fmt.Fprintln(t, "\tc4.large\t2\t3.75")
	fmt.Fprintln(t, "\tc4.xlarge\t4\t7.5")
	fmt.Fprintln(t, "\t...")
	t.Flush()

	fmt.Println()
	fmt.Print("Value ? > ")
	fmt.Scan(&instanceType)
	for !isValidInstanceType(instanceType) {
		fmt.Printf("'%s' is not a valid instance type\n", instanceType)
		fmt.Print("Value ? > ")
		fmt.Scan(&instanceType)
	}
	return instanceType
}

// regionPrefixes enumerates the geography and partition prefixes AWS uses in
// region codes. Validation is deliberately format-based rather than a lookup in
// a curated list of region codes: AWS launches regions faster than we release,
// and for a read-only tool an unknown-but-well-formed region simply surfaces an
// AWS-side error, whereas rejecting it makes the tool unusable in that region.
//
// Prefixes change very rarely, so enumerating them still catches typos such as
// "aa-test-1" while accepting every region within a known partition.
var regionPrefixes = []string{
	// commercial geographies
	"us", "eu", "ap", "sa", "ca", "af", "me", "il", "mx",
	// China
	"cn",
	// GovCloud
	"us-gov",
	// US and EU ISO partitions
	"us-iso", "us-isob", "us-isof", "eu-isoe",
	// European Sovereign Cloud
	"eusc-de",
}

var regionFormat = regexp.MustCompile(
	`^(` + strings.Join(regionPrefixes, "|") + `)-[a-z]+-\d+$`,
)

// SuggestedRegions lists commercial AWS regions offered as completions and in
// the interactive region selector. It is a convenience list, not a validation
// whitelist: IsValidRegion accepts any well-formed region code, so a region
// missing here still works. GovCloud, China and ISO partitions are omitted
// because they need separate credentials and endpoints.
var SuggestedRegions = []string{
	"af-south-1",
	"ap-east-1",
	"ap-east-2",
	"ap-northeast-1",
	"ap-northeast-2",
	"ap-northeast-3",
	"ap-south-1",
	"ap-south-2",
	"ap-southeast-1",
	"ap-southeast-2",
	"ap-southeast-3",
	"ap-southeast-4",
	"ap-southeast-5",
	"ap-southeast-7",
	"ca-central-1",
	"ca-west-1",
	"eu-central-1",
	"eu-central-2",
	"eu-north-1",
	"eu-south-1",
	"eu-south-2",
	"eu-west-1",
	"eu-west-2",
	"eu-west-3",
	"il-central-1",
	"me-central-1",
	"me-south-1",
	"mx-central-1",
	"sa-east-1",
	"us-east-1",
	"us-east-2",
	"us-west-1",
	"us-west-2",
}

// IsValidRegion reports whether given looks like an AWS region code.
func IsValidRegion(given string) bool {
	return regionFormat.MatchString(given)
}

func isValidInstanceType(given string) bool {
	return regexp.MustCompile("\\w+\\.\\w+").MatchString(given)
}

func IsValidProfile(given string) bool {
	return stringInSlice(given, AllProfiles())
}

var profileNameRegex = regexp.MustCompile(`\[(.*)\]`)

// AllProfiles reads AWSHomeDir at call time rather than through a copy taken at
// package initialisation. The copy meant that replacing AWSHomeDir — which is a
// variable precisely so it can be replaced — moved some lookups and not this one, so
// two places in the same run disagreed about where ~/.aws was.
func AllProfiles() (profiles []string) {
	awsHome := AWSHomeDir()
	files := []string{filepath.Join(awsHome, "config"), filepath.Join(awsHome, "credentials")}
	for _, f := range files {
		if _, err := os.Stat(f); err != nil {
			continue
		}
		out, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		matches := profileNameRegex.FindAllSubmatch(out, -1)
		for _, match := range matches {
			profile := string(match[1])
			profile = strings.TrimSpace(profile)
			profile = strings.TrimPrefix(profile, "profile ")
			profile = strings.TrimSpace(profile)
			if profile != "" {
				profiles = append(profiles, profile)
			}
		}
	}
	return profiles
}

func stringInSlice(s string, slice []string) bool {
	for _, v := range slice {
		if v == s {
			return true
		}
	}
	return false
}
