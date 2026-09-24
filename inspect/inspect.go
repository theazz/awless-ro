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

package inspect

import (
	"io"

	"github.com/theazz/awless-ro/cloud"
	"github.com/theazz/awless-ro/inspect/inspectors"
)

var InspectorsRegister map[string]Inspector

// Every inspector here reads the local graph and nothing else. An inspector must
// not call out to a third-party host: the upstream `pricer` did exactly that,
// POSTing the account's instance types and region to ec2-price.com over plaintext
// HTTP, and by 2026 that domain was no longer registered — anyone could have
// claimed it, harvested the inventory and dictated the numbers we printed as
// fact. It is removed rather than repaired. Pricing belongs to the AWS Pricing
// API, as its own feature, if it is wanted at all.
func init() {
	all := []Inspector{
		&inspectors.BucketSizer{},
		&inspectors.PortScanner{}, &inspectors.OpenBuckets{},
	}

	InspectorsRegister = make(map[string]Inspector)

	for _, i := range all {
		InspectorsRegister[i.Name()] = i
	}
}

type Inspector interface {
	Name() string
	Inspect(cloud.GraphAPI) error
	Print(io.Writer)
}
