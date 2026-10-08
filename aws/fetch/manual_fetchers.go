package awsfetch

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ecs"
	ecstypes "github.com/aws/aws-sdk-go-v2/service/ecs/types"
	"github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2"
	elbv2types "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2/types"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	iamtypes "github.com/aws/aws-sdk-go-v2/service/iam/types"
	"github.com/aws/aws-sdk-go-v2/service/route53"
	route53types "github.com/aws/aws-sdk-go-v2/service/route53/types"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	sqstypes "github.com/aws/aws-sdk-go-v2/service/sqs/types"
	"github.com/theazz/awless-ro/aws/conv"
	"github.com/theazz/awless-ro/cloud"
	"github.com/theazz/awless-ro/cloud/properties"
	"github.com/theazz/awless-ro/cloud/rdf"
	"github.com/theazz/awless-ro/fetch"
	"github.com/theazz/awless-ro/graph"
)

func addManualInfraFetchFuncs(conf *Config, funcs map[string]fetch.Func) {
	funcs["containerinstance"] = func(ctx context.Context, cache fetch.Cache) ([]*graph.Resource, interface{}, error) {
		var objects []ecstypes.ContainerInstance
		var resources []*graph.Resource

		if !conf.getBoolDefaultTrue("aws.infra.containerinstance.sync") && !getBoolFromContext(ctx, "force") {
			conf.Log.Verbose("sync: *disabled* for resource infra[containerinstance]")
			return resources, objects, nil
		}

		clusterArns, err := getClusterArns(ctx, cache, conf.APIs.Ecs)
		if err != nil {
			return resources, objects, err
		}

		for _, cluster := range clusterArns {
			paginator := ecs.NewListContainerInstancesPaginator(conf.APIs.Ecs, &ecs.ListContainerInstancesInput{
				Cluster: awssdk.String(cluster),
			})
			for paginator.HasMorePages() {
				out, err := paginator.NextPage(ctx)
				if err != nil {
					return resources, objects, err
				}
				if len(out.ContainerInstanceArns) == 0 {
					continue
				}

				described, err := conf.APIs.Ecs.DescribeContainerInstances(ctx, &ecs.DescribeContainerInstancesInput{
					Cluster:            awssdk.String(cluster),
					ContainerInstances: out.ContainerInstanceArns,
				})
				if err != nil {
					return resources, objects, err
				}

				for _, inst := range described.ContainerInstances {
					objects = append(objects, inst)
					res, err := awsconv.NewResource(inst)
					if err != nil {
						return resources, objects, err
					}
					res.Properties()[properties.Cluster] = cluster
					resources = append(resources, res)
					parent := graph.InitResource(cloud.ContainerCluster, cluster)
					res.AddRelation(rdf.ChildrenOfRel, parent)
				}
			}
		}
		return resources, objects, nil
	}

	funcs["container"] = func(ctx context.Context, cache fetch.Cache) ([]*graph.Resource, interface{}, error) {
		var objects []ecstypes.Container
		var resources []*graph.Resource

		if !conf.getBoolDefaultTrue("aws.infra.container.sync") && !getBoolFromContext(ctx, "force") {
			conf.Log.Verbose("sync: *disabled* for resource infra[container]")
			return resources, objects, nil
		}

		var tasks []ecstypes.Task

		if val, e := cache.Get("getAllTasks", func() (interface{}, error) {
			return getAllTasks(ctx, cache, conf.APIs.Ecs)
		}); e != nil {
			return resources, objects, e
		} else if v, ok := val.([]ecstypes.Task); ok {
			tasks = v
		}

		for _, task := range tasks {
			for _, container := range task.Containers {
				objects = append(objects, container)
				res, err := awsconv.NewResource(container)
				if err != nil {
					return nil, nil, err
				}
				if task.ClusterArn != nil {
					res.Properties()[properties.Cluster] = awssdk.ToString(task.ClusterArn)
				}
				if task.ContainerInstanceArn != nil {
					res.Properties()[properties.ContainerInstance] = awssdk.ToString(task.ContainerInstanceArn)
				}
				if task.CreatedAt != nil {
					res.Properties()[properties.Created] = awssdk.ToTime(task.CreatedAt)
				}
				if task.StartedAt != nil {
					res.Properties()[properties.Launched] = awssdk.ToTime(task.StartedAt)
				}
				if task.StoppedAt != nil {
					res.Properties()[properties.Stopped] = awssdk.ToTime(task.StoppedAt)
				}
				if task.TaskDefinitionArn != nil {
					res.Properties()[properties.ContainerTask] = awssdk.ToString(task.TaskDefinitionArn)
				}
				if task.Group != nil {
					res.Properties()[properties.DeploymentName] = awssdk.ToString(task.Group)
				}

				res.AddRelation(rdf.ChildrenOfRel, graph.InitResource(cloud.ContainerCluster, awssdk.ToString(task.ClusterArn)))
				res.AddRelation(rdf.DependingOnRel, graph.InitResource(cloud.ContainerTask, awssdk.ToString(task.TaskDefinitionArn)))
				res.AddRelation(rdf.DependingOnRel, graph.InitResource(cloud.ContainerInstance, awssdk.ToString(task.ContainerInstanceArn)))

				resources = append(resources, res)
			}
		}

		return resources, objects, nil
	}

	funcs["containertask"] = func(ctx context.Context, cache fetch.Cache) ([]*graph.Resource, interface{}, error) {
		var objects []ecstypes.TaskDefinition
		var resources []*graph.Resource

		if !conf.getBoolDefaultTrue("aws.infra.containertask.sync") && !getBoolFromContext(ctx, "force") {
			conf.Log.Verbose("sync: *disabled* for resource infra[containertask]")
			return resources, objects, nil
		}

		type resStruct struct {
			res ecstypes.TaskDefinition
			err error
		}

		fetchDefinitionsInput := &ecs.ListTaskDefinitionsInput{}
		if givenFamilyPrefix, hasFilter := getUserFiltersFromContext(ctx)["name"]; hasFilter {
			fetchDefinitionsInput.FamilyPrefix = &givenFamilyPrefix
		}

		// Every active revision of every family is listed, which in an account that
		// deploys often is thousands, and each needs its own DescribeTaskDefinition.
		var arns []string
		definitionsPaginator := ecs.NewListTaskDefinitionsPaginator(conf.APIs.Ecs, fetchDefinitionsInput)
		for definitionsPaginator.HasMorePages() {
			out, err := definitionsPaginator.NextPage(ctx)
			if err != nil {
				return resources, objects, err
			}
			arns = append(arns, out.TaskDefinitionArns...)
		}

		// A definition that fails to describe is reported with the others rather
		// than ending the listing, as before; so the calls never return an error.
		var (
			mu        sync.Mutex
			described []resStruct
		)
		forEachParallel(ctx, arns, func(ctx context.Context, taskDefArn string) error {
			tasksOut, err := conf.APIs.Ecs.DescribeTaskDefinition(ctx, &ecs.DescribeTaskDefinitionInput{
				TaskDefinition: awssdk.String(taskDefArn),
			})
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				described = append(described, resStruct{err: err})
				return nil
			}
			if tasksOut.TaskDefinition != nil {
				described = append(described, resStruct{res: *tasksOut.TaskDefinition})
			}
			return nil
		})

		var tasks []ecstypes.Task
		if val, e := cache.Get("getAllTasks", func() (interface{}, error) {
			return getAllTasks(ctx, cache, conf.APIs.Ecs)
		}); e != nil {
			return resources, objects, e
		} else if v, ok := val.([]ecstypes.Task); ok {
			tasks = v
		}

		var errors []string

		for _, res := range described {
			if res.err != nil {
				errors = appendIfNotInSlice(errors, res.err.Error())
				continue
			}
			objects = append(objects, res.res)
			var graphres *graph.Resource
			graphres, cerr := awsconv.NewResource(res.res)
			if cerr != nil {
				errors = appendIfNotInSlice(errors, cerr.Error())
				continue
			}
			var deployments []*graph.KeyValue
			var runningServicesCount, stoppedServicesCount, runningTasksCount, stoppedTasksCount uint
			for _, t := range tasks {
				if awssdk.ToString(t.TaskDefinitionArn) == awssdk.ToString(res.res.TaskDefinitionArn) {
					group := awssdk.ToString(t.Group)
					state := strings.ToLower(awssdk.ToString(t.LastStatus))
					clusterArn := awssdk.ToString(t.ClusterArn)
					if strings.HasPrefix(group, "service:") {
						switch state {
						case "stopped":
							stoppedServicesCount++
							deployments = append(deployments, &graph.KeyValue{KeyName: arnToName(clusterArn), Value: group[len("service:"):] + " (stopped service)"})
						case "running":
							runningServicesCount++
							deployments = append(deployments, &graph.KeyValue{KeyName: arnToName(clusterArn), Value: group[len("service:"):] + " (running service)"})
						}
					}
					if strings.HasPrefix(group, "family:") {
						switch state {
						case "stopped":
							deployments = append(deployments, &graph.KeyValue{KeyName: arnToName(clusterArn), Value: group[len("family:"):] + " (stopped task)"})
							stoppedTasksCount++
						case "running":
							deployments = append(deployments, &graph.KeyValue{KeyName: arnToName(clusterArn), Value: group[len("family:"):] + " (running task)"})
							runningTasksCount++
						}
					}
				}
			}
			if len(deployments) > 0 {
				graphres.Properties()[properties.Deployments] = deployments
			}
			switch {
			case runningServicesCount+stoppedServicesCount+runningTasksCount+stoppedTasksCount == 0:
				if state := strings.ToLower(string(res.res.Status)); state == "active" {
					graphres.Properties()[properties.State] = "ready"
				} else {
					graphres.Properties()[properties.State] = state
				}
			default:
				var stateSl []string
				if runningServicesCount > 0 {
					stateSl = append(stateSl, fmt.Sprintf("%d %s running", runningServicesCount, pluralizeIfNeeded("service", runningServicesCount)))
				}
				if stoppedServicesCount > 0 {
					stateSl = append(stateSl, fmt.Sprintf("%d %s stopped", stoppedServicesCount, pluralizeIfNeeded("service", runningServicesCount)))
				}
				if runningTasksCount > 0 {
					stateSl = append(stateSl, fmt.Sprintf("%d %s running", runningTasksCount, pluralizeIfNeeded("task", runningServicesCount)))
				}
				if stoppedTasksCount > 0 {
					stateSl = append(stateSl, fmt.Sprintf("%d %s stopped", stoppedTasksCount, pluralizeIfNeeded("task", runningServicesCount)))
				}
				if len(stateSl) > 0 {
					graphres.Properties()[properties.State] = strings.Join(stateSl, " ")
				}
			}

			resources = append(resources, graphres)
		}

		if len(errors) > 0 {
			return resources, objects, fmt.Errorf("%s", strings.Join(errors, "; "))
		}

		return resources, objects, nil
	}

	funcs["containercluster"] = func(ctx context.Context, cache fetch.Cache) ([]*graph.Resource, interface{}, error) {
		var resources []*graph.Resource
		var objects []ecstypes.Cluster

		if !conf.getBoolDefaultTrue("aws.infra.containercluster.sync") && !getBoolFromContext(ctx, "force") {
			conf.Log.Verbose("sync: *disabled* for resource infra[containercluster]")
			return resources, objects, nil
		}

		clusterNames, err := getClusterArns(ctx, cache, conf.APIs.Ecs)
		if err != nil {
			return resources, objects, nil
		}

		for _, clusterArns := range sliceOfSlice(clusterNames, 100) {
			clustersOut, err := conf.APIs.Ecs.DescribeClusters(ctx, &ecs.DescribeClustersInput{Clusters: clusterArns})
			if err != nil {
				return resources, objects, err
			}

			for _, cluster := range clustersOut.Clusters {
				objects = append(objects, cluster)
				res, err := awsconv.NewResource(cluster)
				if err != nil {
					return resources, objects, err
				}
				resources = append(resources, res)
			}
		}
		return resources, objects, nil
	}

	funcs["listener"] = func(ctx context.Context, cache fetch.Cache) ([]*graph.Resource, interface{}, error) {
		var objects []elbv2types.Listener
		var resources []*graph.Resource

		if !conf.getBoolDefaultTrue("aws.infra.listener.sync") && !getBoolFromContext(ctx, "force") {
			conf.Log.Verbose("sync: *disabled* for resource infra[listener]")
			return resources, objects, nil
		}

		var balancerArns []*string
		balancers := elasticloadbalancingv2.NewDescribeLoadBalancersPaginator(conf.APIs.Elbv2,
			&elasticloadbalancingv2.DescribeLoadBalancersInput{})
		for balancers.HasMorePages() {
			out, err := balancers.NextPage(ctx)
			if err != nil {
				return resources, objects, err
			}
			for _, lb := range out.LoadBalancers {
				balancerArns = append(balancerArns, lb.LoadBalancerArn)
			}
		}

		var mu sync.Mutex
		err := forEachParallel(ctx, balancerArns, func(ctx context.Context, arn *string) error {
			listeners := elasticloadbalancingv2.NewDescribeListenersPaginator(conf.APIs.Elbv2,
				&elasticloadbalancingv2.DescribeListenersInput{LoadBalancerArn: arn})
			for listeners.HasMorePages() {
				page, err := listeners.NextPage(ctx)
				if err != nil {
					return err
				}
				mu.Lock()
				objects = append(objects, page.Listeners...)
				mu.Unlock()
			}
			return nil
		})
		if err != nil {
			return resources, objects, err
		}

		for _, listener := range objects {
			res, err := awsconv.NewResource(listener)
			if err != nil {
				return resources, objects, err
			}
			resources = append(resources, res)
		}
		return resources, objects, nil
	}
}

func addManualAccessFetchFuncs(conf *Config, funcs map[string]fetch.Func) {
	funcs["user"] = func(ctx context.Context, cache fetch.Cache) ([]*graph.Resource, interface{}, error) {
		var resources []*graph.Resource
		var objects []iamtypes.UserDetail

		if !conf.getBoolDefaultTrue("aws.access.user.sync") && !getBoolFromContext(ctx, "force") {
			conf.Log.Verbose("sync: *disabled* for resource access[user]")
			return resources, objects, nil
		}

		var wg sync.WaitGroup
		resourcesC := make(chan *graph.Resource)
		objectsC := make(chan iamtypes.UserDetail)
		errC := make(chan error)

		wg.Add(1)
		go func() {
			defer wg.Done()
			accountDetails, err := getAccountAuthorizationDetails(ctx, cache, conf.APIs.Iam)
			if err != nil {
				errC <- err
				return
			}
			for _, output := range accountDetails.Users {
				objectsC <- output
				if res, e := awsconv.NewResource(output); e != nil {
					errC <- e
					return
				} else {
					resourcesC <- res
				}
			}
		}()

		wg.Add(1)
		go func() {
			defer wg.Done()
			paginator := iam.NewListUsersPaginator(conf.APIs.Iam, &iam.ListUsersInput{})
			for paginator.HasMorePages() {
				page, err := paginator.NextPage(ctx)
				if err != nil {
					errC <- err
					return
				}
				for _, user := range page.Users {
					res, e := awsconv.NewResource(user)
					if e != nil {
						errC <- e
						return
					}
					resourcesC <- res
				}
			}
		}()

		go func() {
			wg.Wait()
			close(errC)
			close(objectsC)
			close(resourcesC)
		}()

		for {
			select {
			case e := <-errC:
				if e != nil {
					return resources, objects, e
				}
			case r, ok := <-resourcesC:
				if !ok {
					return resources, objects, nil
				}
				if r != nil {
					resources = append(resources, r)
				}
			case o, ok := <-objectsC:
				if !ok {
					return resources, objects, nil
				}
				objects = append(objects, o)
			}
		}
	}

	funcs["group"] = func(ctx context.Context, cache fetch.Cache) ([]*graph.Resource, interface{}, error) {
		var resources []*graph.Resource
		var objects []iamtypes.GroupDetail

		if !conf.getBoolDefaultTrue("aws.access.group.sync") && !getBoolFromContext(ctx, "force") {
			conf.Log.Verbose("sync: *disabled* for resource access[group]")
			return resources, objects, nil
		}

		accountDetails, err := getAccountAuthorizationDetails(ctx, cache, conf.APIs.Iam)
		if err != nil {
			return resources, objects, err
		}

		for _, output := range accountDetails.Groups {
			objects = append(objects, output)
			if res, err := awsconv.NewResource(output); err != nil {
				return resources, objects, err
			} else {
				resources = append(resources, res)
			}
		}

		return resources, objects, nil
	}

	funcs["role"] = func(ctx context.Context, cache fetch.Cache) ([]*graph.Resource, interface{}, error) {
		var resources []*graph.Resource
		var objects []iamtypes.RoleDetail

		if !conf.getBoolDefaultTrue("aws.access.role.sync") && !getBoolFromContext(ctx, "force") {
			conf.Log.Verbose("sync: *disabled* for resource access[role]")
			return resources, objects, nil
		}

		accountDetails, err := getAccountAuthorizationDetails(ctx, cache, conf.APIs.Iam)
		if err != nil {
			return resources, objects, err
		}

		for _, output := range accountDetails.Roles {
			objects = append(objects, output)
			if res, err := awsconv.NewResource(output); err != nil {
				return resources, objects, err
			} else {
				resources = append(resources, res)
			}
		}

		return resources, objects, nil
	}

	funcs["policy"] = func(ctx context.Context, cache fetch.Cache) ([]*graph.Resource, interface{}, error) {
		var resources []*graph.Resource
		var objects []iamtypes.Policy

		if !conf.getBoolDefaultTrue("aws.access.policy.sync") && !getBoolFromContext(ctx, "force") {
			conf.Log.Verbose("sync: *disabled* for resource access[policy]")
			return resources, objects, nil
		}

		errC := make(chan error)
		objectsC := make(chan iamtypes.Policy)
		resourcesC := make(chan *graph.Resource)

		var wg sync.WaitGroup

		wg.Add(1)
		go func() {
			defer wg.Done()

			accountDetails, err := getAccountAuthorizationDetails(ctx, cache, conf.APIs.Iam)
			if err != nil {
				errC <- err
				return
			}
			for _, p := range accountDetails.Policies {
				res, e := awsconv.NewResource(p)
				if e != nil {
					errC <- e
					return
				}
				if strings.HasPrefix(awssdk.ToString(p.Arn), "arn:aws:iam::aws:policy") {
					res.Properties()[properties.Type] = "AWS Managed"
				} else {
					res.Properties()[properties.Type] = "Customer Managed"
				}
				res.Properties()[properties.Attached] = int64(awssdk.ToInt32(p.AttachmentCount)) > 0
				resourcesC <- res
			}
		}()

		go func() {
			wg.Wait()
			close(errC)
			close(objectsC)
			close(resourcesC)
		}()

		for {
			select {
			case err := <-errC:
				if err != nil {
					return resources, objects, err
				}
			case o, ok := <-objectsC:
				if !ok {
					return resources, objects, nil
				}
				objects = append(objects, o)
			case r, ok := <-resourcesC:
				if !ok {
					return resources, objects, nil
				}
				resources = append(resources, r)

			}
		}
	}
	funcs["accesskey"] = func(ctx context.Context, cache fetch.Cache) ([]*graph.Resource, interface{}, error) {
		var resources []*graph.Resource
		var objects []iamtypes.AccessKeyMetadata

		if !conf.getBoolDefaultTrue("aws.access.accesskey.sync") && !getBoolFromContext(ctx, "force") {
			conf.Log.Verbose("sync: *disabled* for resource access[accesskey]")
			return resources, objects, nil
		}

		var users []iamtypes.User
		usersPaginator := iam.NewListUsersPaginator(conf.APIs.Iam, &iam.ListUsersInput{})
		for usersPaginator.HasMorePages() {
			outUsers, err := usersPaginator.NextPage(ctx)
			if err != nil {
				return resources, objects, err
			}
			users = append(users, outUsers.Users...)
		}

		// One ListAccessKeys per user, against IAM's low rate limits. This used to
		// stop on a hasError flag that the per-user goroutines wrote and the paging
		// loop read without synchronisation.
		var mu sync.Mutex
		err := forEachParallel(ctx, users, func(ctx context.Context, u iamtypes.User) error {
			userRes, err := awsconv.InitResource(u)
			if err != nil {
				return err
			}
			keys := iam.NewListAccessKeysPaginator(conf.APIs.Iam, &iam.ListAccessKeysInput{UserName: u.UserName})
			for keys.HasMorePages() {
				out, err := keys.NextPage(ctx)
				if err != nil {
					return err
				}
				for _, output := range out.AccessKeyMetadata {
					res, err := awsconv.NewResource(output)
					if err != nil {
						return err
					}
					res.AddRelation(rdf.ChildrenOfRel, userRes)
					mu.Lock()
					objects = append(objects, output)
					resources = append(resources, res)
					mu.Unlock()
				}
			}
			return nil
		})
		return resources, objects, err
	}
}

func addManualStorageFetchFuncs(conf *Config, funcs map[string]fetch.Func) {
	funcs["bucket"] = func(ctx context.Context, cache fetch.Cache) ([]*graph.Resource, interface{}, error) {
		var resources []*graph.Resource
		var objects []s3types.Bucket

		if !conf.getBoolDefaultTrue("aws.storage.bucket.sync") && !getBoolFromContext(ctx, "force") {
			conf.Log.Verbose("sync: *disabled* for resource storage[bucket]")
			return resources, objects, nil
		}

		bucketM := &sync.Mutex{}

		err := forEachBucketParallel(ctx, cache, conf.APIs.S3, func(b s3types.Bucket) error {
			bucketM.Lock()
			objects = append(objects, b)
			bucketM.Unlock()
			res, err := awsconv.NewResource(b)
			if err != nil {
				return fmt.Errorf("build resource for bucket `%s`: %s", awssdk.ToString(b.Name), err)
			}
			grants, err := fetchAndExtractGrantsFn(ctx, conf.APIs.S3, awssdk.ToString(b.Name))
			if err != nil {
				return fmt.Errorf("fetching grants for bucket %s: %s", awssdk.ToString(b.Name), err)
			}
			res.Properties()[properties.Grants] = grants
			bucketM.Lock()
			resources = append(resources, res)
			bucketM.Unlock()
			return nil
		})
		return resources, objects, err
	}

	funcs["s3object"] = func(ctx context.Context, cache fetch.Cache) ([]*graph.Resource, interface{}, error) {
		var objects []s3types.Object
		var resources []*graph.Resource

		resourcesC := make(chan *graph.Resource)

		if !conf.getBoolDefaultTrue("aws.storage.s3object.sync") && !getBoolFromContext(ctx, "force") {
			conf.Log.Verbose("sync: *disabled* for resource storage[s3object]")
			return resources, objects, nil
		}

		var wg sync.WaitGroup
		wg.Add(1)
		go func() {
			defer wg.Done()
			for r := range resourcesC {
				resources = append(resources, r)
			}
		}()

		err := forEachBucketParallel(ctx, cache, conf.APIs.S3, func(b s3types.Bucket) error {
			return fetchObjectsForBucket(ctx, conf.APIs.S3, b, resourcesC)
		})

		close(resourcesC)

		wg.Wait()

		return resources, objects, err
	}
}
func addManualMessagingFetchFuncs(conf *Config, funcs map[string]fetch.Func) {
	funcs["queue"] = func(ctx context.Context, cache fetch.Cache) ([]*graph.Resource, interface{}, error) {
		var objects []string
		var resources []*graph.Resource

		if !conf.getBoolDefaultTrue("aws.messaging.queue.sync") && !getBoolFromContext(ctx, "force") {
			conf.Log.Verbose("sync: *disabled* for resource messaging[queue]")
			return resources, objects, nil
		}

		var queueUrls []string
		queuesPaginator := sqs.NewListQueuesPaginator(conf.APIs.Sqs, &sqs.ListQueuesInput{})
		for queuesPaginator.HasMorePages() {
			out, err := queuesPaginator.NextPage(ctx)
			if err != nil {
				return nil, objects, err
			}
			queueUrls = append(queueUrls, out.QueueUrls...)
		}

		var mu sync.Mutex
		err := forEachParallel(ctx, queueUrls, func(ctx context.Context, url string) error {
			res := graph.InitResource(cloud.Queue, url)
			res.Properties()[properties.ID] = url
			attrs, err := conf.APIs.Sqs.GetQueueAttributes(ctx, &sqs.GetQueueAttributesInput{
				AttributeNames: []sqstypes.QueueAttributeName{sqstypes.QueueAttributeNameAll},
				QueueUrl:       awssdk.String(url),
			})
			// A queue can disappear between listing and describing it; that
			// is not a sync failure.
			var notExist *sqstypes.QueueDoesNotExist
			var deleted *sqstypes.QueueDeletedRecently
			if errors.As(err, &notExist) || errors.As(err, &deleted) {
				mu.Lock()
				objects = append(objects, url)
				mu.Unlock()
				return nil
			}
			if err != nil {
				return err
			}
			for k, v := range attrs.Attributes {
				switch k {
				case "ApproximateNumberOfMessages":
					count, err := strconv.Atoi(v)
					if err != nil {
						return err
					}
					res.Properties()[properties.ApproximateMessageCount] = count
				case "CreatedTimestamp":
					if v != "" {
						timestamp, err := strconv.ParseInt(v, 10, 64)
						if err != nil {
							return err
						}
						res.Properties()[properties.Created] = time.Unix(timestamp, 0)
					}
				case "LastModifiedTimestamp":
					if v != "" {
						timestamp, err := strconv.ParseInt(v, 10, 64)
						if err != nil {
							return err
						}
						res.Properties()[properties.Modified] = time.Unix(timestamp, 0)
					}
				case "QueueArn":
					res.Properties()[properties.Arn] = v
				case "DelaySeconds":
					delay, err := strconv.Atoi(v)
					if err != nil {
						return err
					}
					res.Properties()[properties.Delay] = delay
				}
			}
			mu.Lock()
			objects = append(objects, url)
			resources = append(resources, res)
			mu.Unlock()
			return nil
		})
		return resources, objects, err
	}
}

func addManualDnsFetchFuncs(conf *Config, funcs map[string]fetch.Func) {
	funcs["record"] = func(ctx context.Context, cache fetch.Cache) ([]*graph.Resource, interface{}, error) {
		var objects []route53types.ResourceRecordSet
		var resources []*graph.Resource

		if !conf.getBoolDefaultTrue("aws.dns.record.sync") && !getBoolFromContext(ctx, "force") {
			conf.Log.Verbose("sync: *disabled* for resource dns[record]")
			return resources, objects, nil
		}

		zoneName, hasZoneFilter := getUserFiltersFromContext(ctx)["zone"]

		var zones []route53types.HostedZone
		paginator := route53.NewListHostedZonesPaginator(conf.APIs.Route53, &route53.ListHostedZonesInput{})
		for paginator.HasMorePages() {
			out, err := paginator.NextPage(ctx)
			if err != nil {
				return resources, objects, err
			}
			for _, output := range out.HostedZones {
				if hasZoneFilter && !strings.Contains(strings.ToLower(awssdk.ToString(output.Name)), strings.ToLower(zoneName)) {
					continue
				}
				zones = append(zones, output)
			}
		}

		// Route 53 allows five requests a second per account, so a zone-per-goroutine
		// burst over many zones was throttled into failure; bounded, the SDK's own
		// retries absorb it.
		var mu sync.Mutex
		err := forEachParallel(ctx, zones, func(ctx context.Context, z route53types.HostedZone) error {
			parent, err := awsconv.InitResource(z)
			if err != nil {
				return err
			}
			// ListResourceRecordSets has no paginator in SDK v2: it pages on a
			// record name and type pair rather than a single token, so the loop is
			// written out here.
			input := &route53.ListResourceRecordSetsInput{HostedZoneId: z.Id}
			for {
				out, err := conf.APIs.Route53.ListResourceRecordSets(ctx, input)
				if err != nil {
					return err
				}
				for _, output := range out.ResourceRecordSets {
					// The zone has to be in hand when the id is computed: a
					// record's identity is zone + name + type + set identifier.
					// objects keeps the raw shape, which is the cache's element type.
					res, err := awsconv.NewResource(awsconv.RecordSetInZone{
						ResourceRecordSet: output,
						ZoneId:            awssdk.ToString(z.Id),
						ZoneName:          awssdk.ToString(z.Name),
					})
					if err != nil {
						return err
					}
					res.AddRelation(rdf.ChildrenOfRel, parent)
					mu.Lock()
					objects = append(objects, output)
					resources = append(resources, res)
					mu.Unlock()
				}
				if !out.IsTruncated {
					return nil
				}
				input.StartRecordName = out.NextRecordName
				input.StartRecordType = out.NextRecordType
				input.StartRecordIdentifier = out.NextRecordIdentifier
			}
		})
		return resources, objects, err
	}
}

func addManualLambdaFetchFuncs(conf *Config, funcs map[string]fetch.Func) {
}
func addManualMonitoringFetchFuncs(conf *Config, funcs map[string]fetch.Func) {
}
func addManualCdnFetchFuncs(conf *Config, funcs map[string]fetch.Func) {
}
func addManualCloudformationFetchFuncs(conf *Config, funcs map[string]fetch.Func) {
}
