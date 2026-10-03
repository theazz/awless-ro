package awsfetch

import (
	"context"
	"fmt"
	"sync"

	"github.com/aws/aws-sdk-go-v2/service/iam"
	iamtypes "github.com/aws/aws-sdk-go-v2/service/iam/types"

	"github.com/theazz/awless-ro/fetch"
)

// AccountAuthorizationDetails is the one account-wide IAM call that backs the
// user, group, role and policy fetchers.
type AccountAuthorizationDetails struct {
	Groups   []iamtypes.GroupDetail
	Policies []iamtypes.ManagedPolicyDetail
	Roles    []iamtypes.RoleDetail
	Users    []iamtypes.UserDetail
}

func getAccountAuthorizationDetails(ctx context.Context, cache fetch.Cache, api IamAPI) (*AccountAuthorizationDetails, error) {
	var entities []iamtypes.EntityType
	var cacheKey string
	resourceType, ok := fetch.IsFetchingByType(ctx)
	if ok {
		switch resourceType {
		case "user":
			cacheKey = "usersDetails"
			entities = append(entities, iamtypes.EntityTypeUser)
		case "group":
			cacheKey = "groupsDetails"
			entities = append(entities, iamtypes.EntityTypeGroup)
		case "role":
			cacheKey = "rolesDetails"
			entities = append(entities, iamtypes.EntityTypeRole)
		case "policy":
			cacheKey = "policiesDetails"
			entities = append(entities, iamtypes.EntityTypeLocalManagedPolicy, iamtypes.EntityTypeAWSManagedPolicy)
		}
	} else {
		cacheKey = "accountDetails"
		entities = append(entities, iamtypes.EntityTypeUser, iamtypes.EntityTypeGroup, iamtypes.EntityTypeRole)
		entities = append(entities, iamtypes.EntityTypeLocalManagedPolicy, iamtypes.EntityTypeAWSManagedPolicy)
	}

	if val, err := cache.Get(cacheKey, func() (interface{}, error) {
		return fetchAccountAuthorizationDetails(ctx, entities, api)
	}); err != nil {
		return nil, err
	} else if v, ok := val.(*AccountAuthorizationDetails); ok {
		return v, nil
	} else {
		return nil, fmt.Errorf("cannot get account details (val of type %T)", val)
	}
}

// fetchAccountAuthorizationDetails runs one GetAccountAuthorizationDetails
// pagination per entity type, side by side, and merges them.
//
// It used to be a single pagination over every type, page after page. IAM caps a
// page by size rather than by count — policy documents make them heavy — so an
// account with ~500 roles and ~560 managed policies took 25 sequential pages, about 28
// seconds, nearly all of a first sync. A larger MaxItems does not help, because the
// cap is on size. The paginations of different types are independent, though, and
// run together they took 14 seconds on the same account: the slowest type alone.
//
// Users and groups share a pagination, since there are rarely enough of them to be
// worth a request of their own.
func fetchAccountAuthorizationDetails(ctx context.Context, entities []iamtypes.EntityType, api IamAPI) (*AccountAuthorizationDetails, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	var (
		details  = new(AccountAuthorizationDetails)
		mu       sync.Mutex
		wg       sync.WaitGroup
		once     sync.Once
		firstErr error
	)
	for _, filter := range paginationGroups(entities) {
		wg.Add(1)
		go func(filter []iamtypes.EntityType) {
			defer wg.Done()
			paginator := iam.NewGetAccountAuthorizationDetailsPaginator(api, &iam.GetAccountAuthorizationDetailsInput{
				Filter: filter,
			})
			for paginator.HasMorePages() {
				out, err := paginator.NextPage(ctx)
				if err != nil {
					once.Do(func() {
						firstErr = err
						cancel()
					})
					return
				}
				mu.Lock()
				details.Users = append(details.Users, out.UserDetailList...)
				details.Groups = append(details.Groups, out.GroupDetailList...)
				details.Roles = append(details.Roles, out.RoleDetailList...)
				details.Policies = append(details.Policies, out.Policies...)
				mu.Unlock()
			}
		}(filter)
	}
	wg.Wait()

	if firstErr != nil {
		return details, firstErr
	}
	return details, nil
}

// paginationGroups splits the entity types into the paginations to run side by side:
// users and groups together, every other type on its own. Each type lands in exactly
// one group, so the order within a type is the order IAM returned it in.
func paginationGroups(entities []iamtypes.EntityType) [][]iamtypes.EntityType {
	var small []iamtypes.EntityType
	var groups [][]iamtypes.EntityType
	for _, e := range entities {
		switch e {
		case iamtypes.EntityTypeUser, iamtypes.EntityTypeGroup:
			small = append(small, e)
		default:
			groups = append(groups, []iamtypes.EntityType{e})
		}
	}
	if len(small) > 0 {
		groups = append([][]iamtypes.EntityType{small}, groups...)
	}
	return groups
}
