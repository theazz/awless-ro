package awstailers

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/autoscaling"
	autoscalingtypes "github.com/aws/aws-sdk-go-v2/service/autoscaling/types"

	awsfetch "github.com/theazz/awless-ro/aws/fetch"
)

type fakeAutoscaling struct {
	awsfetch.AutoscalingAPI
	activities []autoscalingtypes.Activity
}

func (f *fakeAutoscaling) DescribeScalingActivities(context.Context, *autoscaling.DescribeScalingActivitiesInput, ...func(*autoscaling.Options)) (*autoscaling.DescribeScalingActivitiesOutput, error) {
	return &autoscaling.DescribeScalingActivitiesOutput{Activities: f.activities}, nil
}

func tailer(follow bool, frequency time.Duration, activities ...autoscalingtypes.Activity) *scalingActivitiesTailer {
	t := NewScalingActivitiesTailer(10, follow, frequency)
	t.api = &fakeAutoscaling{activities: activities}
	t.notice = &bytes.Buffer{}
	return t
}

// An empty history used to print nothing and exit 0, indistinguishable from a failure.
func TestNoScalingActivitySaysSo(t *testing.T) {
	var out, notice bytes.Buffer
	tl := tailer(false, 10*time.Second)
	tl.notice = &notice

	if err := tl.Tail(&out); err != nil {
		t.Fatal(err)
	}
	if out.Len() != 0 {
		t.Errorf("stdout should hold events only, got %q", out.String())
	}
	if !strings.Contains(notice.String(), "no scaling activities") {
		t.Errorf("an empty history should be reported, got %q", notice.String())
	}
}

// --follow returned at once when there was no activity yet, which is exactly when one
// waits for the first.
func TestFollowWaitsWhenThereIsNoActivityYet(t *testing.T) {
	tl := tailer(true, 5*time.Second)

	done := make(chan error, 1)
	go func() { done <- tl.Tail(&bytes.Buffer{}) }()

	select {
	case err := <-done:
		t.Fatalf("--follow returned (%v) instead of waiting for activity", err)
	case <-time.After(300 * time.Millisecond):
	}
}

func TestScalingActivitiesArePrintedOldestFirst(t *testing.T) {
	now := time.Now()
	tl := tailer(false, 10*time.Second,
		autoscalingtypes.Activity{ActivityId: awssdk.String("2"), AutoScalingGroupName: awssdk.String("web"), StartTime: awssdk.Time(now), StatusCode: "Successful", Description: awssdk.String("Launching a new EC2 instance")},
		autoscalingtypes.Activity{ActivityId: awssdk.String("1"), AutoScalingGroupName: awssdk.String("web"), StartTime: awssdk.Time(now.Add(-time.Hour)), StatusCode: "Successful", Description: awssdk.String("Terminating an EC2 instance")},
	)
	var out, notice bytes.Buffer
	tl.notice = &notice

	if err := tl.Tail(&out); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	if strings.Index(got, "Terminating") > strings.Index(got, "Launching") {
		t.Errorf("activities should be oldest first:\n%s", got)
	}
	if notice.Len() != 0 {
		t.Errorf("nothing to report when there are activities, got %q", notice.String())
	}
}

func TestFollowRejectsAFrequencyBelowFiveSecondsBeforeCallingAWS(t *testing.T) {
	if err := tailer(true, time.Second).Tail(&bytes.Buffer{}); err == nil {
		t.Error("a 1s polling frequency should be refused")
	}
}
