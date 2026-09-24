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

	images := make([]Image, 0)
	paginator := ec2.NewDescribeImagesPaginator(r.api, input)
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, err
		}
		for _, ami := range page.Images {
			created, _ := time.Parse(time.RFC3339, awssdk.ToString(ami.CreationDate))
			images = append(images, Image{
				ID:                 awssdk.ToString(ami.ImageId),
				Owner:              awssdk.ToString(ami.OwnerId),
				Name:               awssdk.ToString(ami.Name),
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

	// Newest first, which is what makes --latest-id mean anything. Images whose
	// CreationDate did not parse sort to the end rather than to the front, so a
	// malformed date cannot be mistaken for the newest image.
	sort.SliceStable(images, func(i, j int) bool {
		if images[i].Created.IsZero() != images[j].Created.IsZero() {
			return !images[i].Created.IsZero()
		}
		return images[i].Created.After(images[j].Created)
	})

	if len(images) == 0 {
		return images, fmt.Errorf("%w for account %s with name pattern %q, architecture %s, %s, %s: "+
			"either nothing is published matching that, or the vendor changed how it names images",
			ErrNoMatch, account, s.namePattern(), s.arch(), s.virt(), s.store())
	}

	r.mu.Lock()
	r.cache[key] = images
	r.mu.Unlock()

	return images, nil
}
