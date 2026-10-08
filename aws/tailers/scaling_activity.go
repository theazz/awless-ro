package awstailers

import (
	"context"
	"fmt"
	"io"
	"os"
	"sort"
	"time"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/autoscaling"
	autoscalingtypes "github.com/aws/aws-sdk-go-v2/service/autoscaling/types"
	awsfetch "github.com/theazz/awless-ro/aws/fetch"
	"github.com/theazz/awless-ro/aws/services"
)

type scalingActivitiesTailer struct {
	follow           bool
	pollingFrequency time.Duration
	lastEventTime    time.Time
	nbEvents         int
	// notice receives what is said about the output rather than the output itself,
	// so that stdout stays the events alone.
	notice io.Writer
	// api is the autoscaling client; nil means the one set up for the session.
	api awsfetch.AutoscalingAPI
}

func NewScalingActivitiesTailer(nbEvents int, follow bool, frequency time.Duration) *scalingActivitiesTailer {
	return &scalingActivitiesTailer{nbEvents: nbEvents, follow: follow, pollingFrequency: frequency, notice: os.Stderr}
}

func (t *scalingActivitiesTailer) Name() string {
	return "scaling-activities"
}

func (t *scalingActivitiesTailer) Tail(w io.Writer) error {
	api := t.api
	if api == nil {
		infra, ok := awsservices.InfraService.(*awsservices.Infra)
		if !ok {
			return fmt.Errorf("invalid cloud service, expected awsservices.Infra, got %T", awsservices.InfraService)
		}
		api = infra.AutoscalingAPI
	}
	if t.follow && t.pollingFrequency < 5*time.Second {
		return fmt.Errorf("invalid polling frequency: %s, must be at least 5s", t.pollingFrequency)
	}

	if err := t.displayLastEvents(api, w); err != nil {
		return err
	}

	// No activity printed nothing and exited 0, which reads the same as a command
	// that silently failed. Autoscaling keeps six weeks of history, so an empty answer
	// is common in a quiet account and worth saying.
	if t.lastEventTime.IsZero() {
		fmt.Fprintln(t.notice, "no scaling activities in the last six weeks (the history autoscaling keeps)")
		if !t.follow {
			return nil
		}
		// --follow used to return here, so waiting for the first scaling event —
		// the case it is most wanted for — ended immediately. Start from now
		// instead: anything newer is new.
		t.lastEventTime = time.Now()
	}

	if !t.follow {
		return nil
	}

	ticker := time.NewTicker(t.pollingFrequency)
	defer ticker.Stop()
	for range ticker.C {
		if err := t.displayNewEvents(api, w); err != nil {
			return err
		}
	}
	return nil

}

func (t *scalingActivitiesTailer) displayLastEvents(api awsfetch.AutoscalingAPI, w io.Writer) error {
	out, err := api.DescribeScalingActivities(context.Background(), &autoscaling.DescribeScalingActivitiesInput{MaxRecords: awssdk.Int32(int32(t.nbEvents))})
	if err != nil {
		return err
	}
	var events []*event
	for i, activity := range out.Activities {
		evt := newEventFromScalingActivity(activity)
		if i == 0 {
			t.lastEventTime = evt.stamp
		}
		events = append(events, evt)
	}
	sort.Slice(events, func(i int, j int) bool { return events[i].stamp.Before(events[j].stamp) })
	for _, evt := range events {
		if err := evt.print(w); err != nil {
			return err
		}
	}
	return nil
}

func (t *scalingActivitiesTailer) displayNewEvents(api awsfetch.AutoscalingAPI, w io.Writer) error {
	var eventFound bool
	var newEvents []*event
	lastEventTime := t.lastEventTime
	ctx := context.Background()
	paginator := autoscaling.NewDescribeScalingActivitiesPaginator(api,
		&autoscaling.DescribeScalingActivitiesInput{})
	for paginator.HasMorePages() && !eventFound {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return err
		}
		for _, act := range page.Activities {
			evt := newEventFromScalingActivity(act)
			if t.lastEventTime.Before(evt.stamp) {
				t.lastEventTime = evt.stamp
			}
			if evt.stamp == lastEventTime || evt.stamp.Before(lastEventTime) {
				eventFound = true
				break
			}
			newEvents = append(newEvents, evt)
		}
	}
	sort.Slice(newEvents, func(i int, j int) bool { return newEvents[i].stamp.Before(newEvents[j].stamp) })
	for _, e := range newEvents {
		if err := e.print(w); err != nil {
			return err
		}
	}
	return nil
}

type event struct {
	id      string
	element string
	stamp   time.Time
	message string
}

func newEventFromScalingActivity(s autoscalingtypes.Activity) *event {
	return &event{
		id:      awssdk.ToString(s.ActivityId),
		stamp:   awssdk.ToTime(s.StartTime),
		message: fmt.Sprintf("%s: %s", string(s.StatusCode), awssdk.ToString(s.Description)),
		element: awssdk.ToString(s.AutoScalingGroupName),
	}
}

func (e *event) print(w io.Writer) error {
	_, err := fmt.Fprintf(w, "%s: %s\n\t%s\n", e.stamp, e.element, e.message)
	return err
}
