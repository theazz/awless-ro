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

package awsimage

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"sync"
	"time"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
)

// DescribeImagesAPI is the one operation this package needs.
type DescribeImagesAPI interface {
	DescribeImages(context.Context, *ec2.DescribeImagesInput, ...func(*ec2.Options)) (*ec2.DescribeImagesOutput, error)
}

// Image is what a search returns, newest first.
type Image struct {
	ID                 string    `json:"id"`
	Owner              string    `json:"owner"`
	Name               string    `json:"name"`
	Created            time.Time `json:"created"`
	Architecture       string    `json:"architecture"`
	VirtualizationType string    `json:"virtualizationType"`
	Store              string    `json:"store"`
	Hypervisor         string    `json:"hypervisor"`
	Location           string    `json:"location"`
	Type               string    `json:"type"`
}

// Resolver searches for images. Results are remembered for the life of the process,
// which is all a CLI needs: a single command may ask the same question more than once,
// and a later command is a new process against a possibly newer catalogue.
type Resolver struct {
	api DescribeImagesAPI

	mu    sync.Mutex
	cache map[string][]Image
}

func NewResolver(api DescribeImagesAPI) *Resolver {
	return &Resolver{api: api, cache: make(map[string][]Image)}
}

// Search describes what to look for. Either Query or the Owner and Name pair is set.
type Search struct {
	// Query is a parsed vendor query.
	Query *Query

	// AccountID and NamePattern search an account directly, for a vendor this package
	// does not know or for one's own images.
	AccountID   string
	NamePattern string

	Arch, Virt, Store string
}

// ErrNoOwner rejects a search that names no account.
//
// This is not a usability decision. Anyone can publish a public AMI under any name,
// so a name-only search returns whatever a stranger has called "ubuntu-noble-latest".
// The whoAMI name confusion attack, disclosed in 2025, turned precisely that into
// code execution in accounts whose tooling fed the result to run-instances. awless-ro
// launches nothing, but it prints an id that a person will paste somewhere, so it
// must not be the link that goes wrong.
var ErrNoOwner = errors.New("an image search must name the account that publishes the images")

// ErrNoMatch reports that the search was valid and matched nothing.
var ErrNoMatch = errors.New("no image matched")

func (s Search) key() string {
	if s.Query != nil {
		return s.Query.String()
	}
	return fmt.Sprintf("%s|%s|%s|%s|%s", s.AccountID, s.NamePattern, s.Arch, s.Virt, s.Store)
}

func (s Search) accountID() string {
	if s.Query != nil {
		return s.Query.Owner.AccountID
	}
	return s.AccountID
}

func (s Search) namePattern() string {
	if s.Query != nil {
		return s.Query.NameFilter()
	}
	if s.NamePattern == "" {
		return "*"
	}
	return s.NamePattern
}

// order sorts newest first, which is what makes --latest-id mean anything.
//
// Two things complicate it. An image whose CreationDate did not parse sorts to the end
// rather than the front, so a malformed date cannot be mistaken for the newest image.
// And where a vendor publishes one release as several images, those are ranked among
// themselves by name rather than by the seconds between their publication: see
// Owner.Release for the case that made this necessary.
func (r *Resolver) order(images []Image, releaseOf func(string) string) {
	// A release is as new as its newest image, so that the group moves together
	// rather than being interleaved with a neighbouring release.
	releaseDate := make(map[string]time.Time)
	for _, img := range images {
		release := releaseOf(img.Name)
		if release == "" {
			continue
		}
		if img.Created.After(releaseDate[release]) {
			releaseDate[release] = img.Created
		}
	}

	// rank is the date an image is ordered by: its release's date where it belongs to
	// one, otherwise its own.
	rank := func(img Image) time.Time {
		if release := releaseOf(img.Name); release != "" {
			if d, ok := releaseDate[release]; ok {
				return d
			}
		}
		return img.Created
	}

	sort.SliceStable(images, func(i, j int) bool {
		a, b := images[i], images[j]
		if a.Created.IsZero() != b.Created.IsZero() {
			return !a.Created.IsZero()
		}
		if da, db := rank(a), rank(b); !da.Equal(db) {
			return da.After(db)
		}
		// Same release, or the same instant. Descending by name, which for a vendor
		// numbering its builds in the name prefers the higher one.
		return a.Name > b.Name
	})
}

// releaseOf groups images by release, for the vendors that need it. A search by
// account and name has no vendor, so nothing is grouped.
func (s Search) releaseOf(name string) string {
	if s.Query == nil || s.Query.Owner.Release == nil {
		return ""
	}
	return s.Query.Owner.Release(name)
}

// mainstreamFilter applies only to vendor queries. A search by account and name is
// the deliberate escape hatch from it: the caller has said which names they want.
func (s Search) mainstreamFilter() (*regexp.Regexp, error) {
	if s.Query == nil {
		return nil, nil
	}
	return s.Query.MainstreamFilter()
}

func (s Search) arch() string {
	if s.Query != nil {
		return s.Query.Arch
	}
	return orDefault(s.Arch, defaultArch)
}

func (s Search) virt() string {
	if s.Query != nil {
		return s.Query.Virt
	}
	return orDefault(s.Virt, defaultVirt)
}

func (s Search) store() string {
	if s.Query != nil {
		return s.Query.Store
	}
	return orDefault(s.Store, defaultStore)
}

func orDefault(given, fallback string) string {
	if given == "" {
		return fallback
	}
	return given
}

// Resolve runs the search. A search matching nothing is an error rather than an empty
// list, because the usual cause is a naming convention that has moved on, and silence
// gives the user nothing to act on.
func (r *Resolver) Resolve(ctx context.Context, s Search) ([]Image, error) {
	account := s.accountID()
	if account == "" {
		return nil, ErrNoOwner
	}

	key := s.key()
	r.mu.Lock()
	if cached, ok := r.cache[key]; ok {
		r.mu.Unlock()
		return cached, nil
	}
	r.mu.Unlock()

	input := &ec2.DescribeImagesInput{
		// Owners, not a filter on owner-id. Both narrow the result, but this is the
		// parameter AWS documents as the control, and reviewers look for it here.
		Owners: []string{account},
		Filters: []ec2types.Filter{
			{Name: awssdk.String("state"), Values: []string{"available"}},
			{Name: awssdk.String("is-public"), Values: []string{"true"}},
			{Name: awssdk.String("name"), Values: []string{s.namePattern()}},
			{Name: awssdk.String("architecture"), Values: []string{s.arch()}},
			{Name: awssdk.String("virtualization-type"), Values: []string{s.virt()}},
			{Name: awssdk.String("root-device-type"), Values: []string{s.store()}},
		},
	}

	// Built before the call, so a bad expression costs nothing.
	mainstream, err := s.mainstreamFilter()
	if err != nil {
		return nil, err
	}

	images := make([]Image, 0)
	var setAside int
	paginator := ec2.NewDescribeImagesPaginator(r.api, input)
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, err
		}
		for _, ami := range page.Images {
			name := awssdk.ToString(ami.Name)

			// Specialised builds share a prefix with the ordinary image and are
			// sometimes published minutes later, so they would win on date. See
			// Owner.Mainstream for what this excludes and why a glob cannot.
			if mainstream != nil && !mainstream.MatchString(name) {
				setAside++
				continue
			}

			created, _ := time.Parse(time.RFC3339, awssdk.ToString(ami.CreationDate))
			images = append(images, Image{
				ID:                 awssdk.ToString(ami.ImageId),
				Owner:              awssdk.ToString(ami.OwnerId),
				Name:               name,
				Created:            created,
				Architecture:       string(ami.Architecture),
				VirtualizationType: string(ami.VirtualizationType),
				Store:              string(ami.RootDeviceType),
				Hypervisor:         string(ami.Hypervisor),
				Location:           awssdk.ToString(ami.ImageLocation),
				Type:               string(ami.ImageType),
			})
		}
	}

	r.order(images, s.releaseOf)

	if len(images) == 0 {
		// Two different situations, and the user can act on only one of them, so they
		// are reported apart. Images set aside means the account is publishing under
		// this glob but nothing looked like the ordinary line — a convention that has
		// moved on, or a query for a release that only exists as specialised builds.
		if setAside > 0 {
			return images, fmt.Errorf("%w: account %s publishes %d image(s) matching %q, but none matching %q, "+
				"which is what the ordinary %s line looks like. The vendor may have changed how it names images, "+
				"or this release may only exist as specialised builds. "+
				"`--owner %s --name '%s'` searches without that restriction",
				ErrNoMatch, account, setAside, s.namePattern(), mainstream, s.Query.Owner.Name,
				account, s.namePattern())
		}
		return images, fmt.Errorf("%w for account %s with name pattern %q, architecture %s, %s, %s: "+
			"either nothing is published matching that, or the vendor changed how it names images",
			ErrNoMatch, account, s.namePattern(), s.arch(), s.virt(), s.store())
	}

	r.mu.Lock()
	r.cache[key] = images
	r.mu.Unlock()

	return images, nil
}
