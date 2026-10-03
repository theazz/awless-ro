package awsfetch

import (
	"context"
	"sync"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ecs"
	ecstypes "github.com/aws/aws-sdk-go-v2/service/ecs/types"

	"github.com/theazz/awless-ro/fetch"
)

func getClusterArns(ctx context.Context, cache fetch.Cache, api EcsAPI) ([]string, error) {
	var arns []string
	if clusterName, hasFilter := getUserFiltersFromContext(ctx)["cluster"]; hasFilter {
		out, err := api.DescribeClusters(ctx, &ecs.DescribeClustersInput{Clusters: []string{clusterName}})
		if err != nil {
			return arns, err
		}
		for _, c := range out.Clusters {
			arns = append(arns, aws.ToString(c.ClusterArn))
		}
		return arns, nil
	}

	if val, cerr := cache.Get("getClustersNames", func() (interface{}, error) {
		var all []string
		paginator := ecs.NewListClustersPaginator(api, &ecs.ListClustersInput{})
		for paginator.HasMorePages() {
			out, err := paginator.NextPage(ctx)
			if err != nil {
				return all, err
			}
			all = append(all, out.ClusterArns...)
		}
		return all, nil
	}); cerr != nil {
		return arns, cerr
	} else if v, ok := val.([]string); ok {
		arns = v
	}

	return arns, nil
}

// getAllTasks lists the running and the stopped tasks of every cluster, then
// describes them in batches. DescribeTasks only accepts task ARNs of a single
// cluster at a time, which is why the cluster travels alongside the ARNs. A page of
// ListTasks holds at most 100 ARNs, which is also as many as DescribeTasks takes.
func getAllTasks(ctx context.Context, cache fetch.Cache, api EcsAPI) ([]ecstypes.Task, error) {
	clusterArns, err := getClusterArns(ctx, cache, api)
	if err != nil {
		return nil, err
	}

	type listing struct {
		cluster string
		status  ecstypes.DesiredStatus
	}
	var listings []listing
	for _, cluster := range clusterArns {
		listings = append(listings,
			listing{cluster, ecstypes.DesiredStatusRunning},
			listing{cluster, ecstypes.DesiredStatusStopped})
	}

	type batch struct {
		cluster string
		arns    []string
	}
	var (
		mu      sync.Mutex
		batches []batch
	)
	err = forEachParallel(ctx, listings, func(ctx context.Context, l listing) error {
		paginator := ecs.NewListTasksPaginator(api, &ecs.ListTasksInput{
			Cluster:       aws.String(l.cluster),
			DesiredStatus: l.status,
		})
		for paginator.HasMorePages() {
			out, e := paginator.NextPage(ctx)
			if e != nil {
				return e
			}
			if len(out.TaskArns) == 0 {
				continue
			}
			mu.Lock()
			batches = append(batches, batch{cluster: l.cluster, arns: out.TaskArns})
			mu.Unlock()
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	var res []ecstypes.Task
	err = forEachParallel(ctx, batches, func(ctx context.Context, b batch) error {
		out, e := api.DescribeTasks(ctx, &ecs.DescribeTasksInput{
			Cluster: aws.String(b.cluster),
			Tasks:   b.arns,
		})
		if e != nil {
			return e
		}
		mu.Lock()
		res = append(res, out.Tasks...)
		mu.Unlock()
		return nil
	})
	if err != nil {
		return nil, err
	}
	return res, nil
}
