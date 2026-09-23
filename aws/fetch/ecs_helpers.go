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
// cluster at a time, which is why the cluster travels alongside the ARNs.
func getAllTasks(ctx context.Context, cache fetch.Cache, api EcsAPI) ([]ecstypes.Task, error) {
	var res []ecstypes.Task

	clusterArns, err := getClusterArns(ctx, cache, api)
	if err != nil {
		return res, err
	}

	type taskArns struct {
		err     error
		arns    []string
		cluster string
	}
	arnsc := make(chan taskArns)
	var listWG sync.WaitGroup

	listTasks := func(cluster string, status ecstypes.DesiredStatus) {
		defer listWG.Done()
		paginator := ecs.NewListTasksPaginator(api, &ecs.ListTasksInput{
			Cluster:       aws.String(cluster),
			DesiredStatus: status,
		})
		for paginator.HasMorePages() {
			out, e := paginator.NextPage(ctx)
			if e != nil {
				arnsc <- taskArns{err: e}
				return
			}
			arnsc <- taskArns{arns: out.TaskArns, cluster: cluster}
		}
	}

	for _, cluster := range clusterArns {
		listWG.Add(1)
		go listTasks(cluster, ecstypes.DesiredStatusRunning)
		listWG.Add(1)
		go listTasks(cluster, ecstypes.DesiredStatusStopped)
	}

	type describedTasks struct {
		err   error
		tasks []ecstypes.Task
	}
	tasksc := make(chan describedTasks)
	var describeWG sync.WaitGroup

	describeWG.Add(1)
	go func() {
		defer describeWG.Done()
		for r := range arnsc {
			if r.err != nil {
				tasksc <- describedTasks{err: r.err}
				return
			}
			if len(r.arns) == 0 {
				continue
			}

			describeWG.Add(1)
			go func(arns []string, cluster string) {
				defer describeWG.Done()
				out, e := api.DescribeTasks(ctx, &ecs.DescribeTasksInput{
					Cluster: aws.String(cluster),
					Tasks:   arns,
				})
				if e != nil {
					tasksc <- describedTasks{err: e}
					return
				}
				tasksc <- describedTasks{tasks: out.Tasks}
			}(r.arns, r.cluster)
		}
	}()

	go func() {
		listWG.Wait()
		close(arnsc)
		describeWG.Wait()
		close(tasksc)
	}()

	for r := range tasksc {
		if r.err != nil {
			return res, r.err
		}
		res = append(res, r.tasks...)
	}

	return res, nil
}
