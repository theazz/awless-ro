package awsfetch

import (
	"context"
	"strings"
	"sync"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"

	"github.com/theazz/awless-ro/aws/conv"
	"github.com/theazz/awless-ro/cloud/rdf"
	"github.com/theazz/awless-ro/fetch"
	"github.com/theazz/awless-ro/graph"
)

func forEachBucketParallel(ctx context.Context, cache fetch.Cache, api S3API, f func(b s3types.Bucket) error) error {
	var buckets []s3types.Bucket

	if val, e := cache.Get("getBucketsPerRegion", func() (interface{}, error) {
		return getBucketsPerRegion(ctx, api)
	}); e != nil {
		return e
	} else if v, ok := val.([]s3types.Bucket); ok {
		buckets = v
	}

	errc := make(chan error)
	var wg sync.WaitGroup

	for _, output := range buckets {
		wg.Add(1)
		go func(b s3types.Bucket) {
			defer wg.Done()
			if err := f(b); err != nil {
				errc <- err
			}
		}(output)
	}
	go func() {
		wg.Wait()
		close(errc)
	}()

	for err := range errc {
		if err != nil {
			return err
		}
	}

	return nil
}

func fetchObjectsForBucket(ctx context.Context, api S3API, bucket s3types.Bucket, resourcesC chan<- *graph.Resource) error {
	paginator := s3.NewListObjectsV2Paginator(api, &s3.ListObjectsV2Input{Bucket: bucket.Name})
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return err
		}
		for _, output := range page.Contents {
			res, err := awsconv.NewResource(output)
			if err != nil {
				return err
			}
			res.SetProperty("Bucket", aws.ToString(bucket.Name))
			resourcesC <- res

			parent, err := awsconv.InitResource(bucket)
			if err != nil {
				return err
			}
			res.AddRelation(rdf.ChildrenOfRel, parent)
			resourcesC <- parent
		}
	}
	return nil
}

// getBucketsPerRegion keeps only the buckets that live in the region being
// synced. ListBuckets is global, so each bucket's location needs its own call.
func getBucketsPerRegion(ctx context.Context, api S3API) ([]s3types.Bucket, error) {
	var buckets []s3types.Bucket

	out, err := api.ListBuckets(ctx, &s3.ListBucketsInput{})
	if err != nil {
		return buckets, err
	}

	var userBucketName string
	var hasBucketFilter bool
	if id, hasID := getUserFiltersFromContext(ctx)["id"]; hasID {
		userBucketName = id
		hasBucketFilter = true
	} else if buck, hasBucket := getUserFiltersFromContext(ctx)["bucket"]; hasBucket {
		userBucketName = buck
		hasBucketFilter = true
	}

	if hasBucketFilter {
		for _, b := range out.Buckets {
			if strings.Contains(strings.ToLower(aws.ToString(b.Name)), strings.ToLower(userBucketName)) {
				buckets = append(buckets, b)
			}
		}
	} else {
		buckets = out.Buckets
	}

	bucketc := make(chan s3types.Bucket)
	errc := make(chan error)

	var wg sync.WaitGroup

	for _, bucket := range buckets {
		wg.Add(1)
		go func(b s3types.Bucket) {
			defer wg.Done()
			loc, err := api.GetBucketLocation(ctx, &s3.GetBucketLocationInput{Bucket: b.Name})
			if err != nil {
				errc <- err
				return
			}

			region, _ := ctx.Value("region").(string)
			// An empty location constraint means us-east-1.
			switch string(loc.LocationConstraint) {
			case "":
				if region == "us-east-1" {
					bucketc <- b
				}
			case region:
				bucketc <- b
			}
		}(bucket)
	}
	go func() {
		wg.Wait()
		close(bucketc)
	}()

	var bucketsInRegion []s3types.Bucket
	for {
		select {
		case err := <-errc:
			if err != nil {
				return bucketsInRegion, err
			}
		case b, ok := <-bucketc:
			if !ok {
				return bucketsInRegion, nil
			}
			bucketsInRegion = append(bucketsInRegion, b)
		}
	}
}

func fetchAndExtractGrantsFn(ctx context.Context, api S3API, bucketName string) ([]*graph.Grant, error) {
	acls, err := api.GetBucketAcl(ctx, &s3.GetBucketAclInput{Bucket: aws.String(bucketName)})
	if err != nil {
		return nil, err
	}
	var grants []*graph.Grant
	for _, acl := range acls.Grants {
		var displayName, granteeType, granteeId string
		if acl.Grantee != nil {
			displayName = aws.ToString(acl.Grantee.DisplayName)
			granteeType = string(acl.Grantee.Type)
			granteeId = aws.ToString(acl.Grantee.ID)

			if aws.ToString(acl.Grantee.EmailAddress) != "" {
				displayName += "<" + aws.ToString(acl.Grantee.EmailAddress) + ">"
			}
			if granteeType == "Group" {
				granteeId += aws.ToString(acl.Grantee.URI)
			}
		}
		grants = append(grants, &graph.Grant{
			Permission: string(acl.Permission),
			Grantee: graph.Grantee{
				GranteeID:          granteeId,
				GranteeType:        granteeType,
				GranteeDisplayName: displayName,
			},
		})
	}
	return grants, nil
}
