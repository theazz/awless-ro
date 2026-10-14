package awstailers

import (
	"bytes"
	"testing"
	"time"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	autoscalingtypes "github.com/aws/aws-sdk-go-v2/service/autoscaling/types"
	cfntypes "github.com/aws/aws-sdk-go-v2/service/cloudformation/types"

	"github.com/theazz/awless-ro/internal/golden"
)

func TestScalingActivitiesJSONGolden(t *testing.T) {
	at := time.Date(2026, 10, 14, 9, 0, 0, 0, time.UTC)
	tl := tailer(false, 10*time.Second,
		autoscalingtypes.Activity{ActivityId: awssdk.String("2"), AutoScalingGroupName: awssdk.String("web"), StartTime: awssdk.Time(at), StatusCode: "Successful", Description: awssdk.String("Launching a new EC2 instance: i-2")},
		autoscalingtypes.Activity{ActivityId: awssdk.String("1"), AutoScalingGroupName: awssdk.String("web"), StartTime: awssdk.Time(at.Add(-time.Hour)), StatusCode: "Failed", Description: awssdk.String("Terminating EC2 instance: i-1")},
	).WithJSON(true)

	var out bytes.Buffer
	if err := tl.Tail(&out); err != nil {
		t.Fatal(err)
	}
	golden.Assert(t, "scaling-activities", out.Bytes())
}

// An empty history prints nothing at all on stdout in JSON either: the notice is on
// stderr, and a stream of no events is no lines.
func TestNoScalingActivityJSONIsNoLines(t *testing.T) {
	var out bytes.Buffer
	if err := tailer(false, 10*time.Second).WithJSON(true).Tail(&out); err != nil {
		t.Fatal(err)
	}
	if out.Len() != 0 {
		t.Errorf("got %q, want nothing", out.String())
	}
}

func TestStackEventsJSONGolden(t *testing.T) {
	at := time.Date(2026, 10, 14, 9, 0, 0, 0, time.UTC)
	// Newest first, as DescribeStackEvents returns them.
	events := stackEvents{
		{cfntypes.StackEvent{EventId: awssdk.String("e3"), StackName: awssdk.String("app"), LogicalResourceId: awssdk.String("app"),
			ResourceType: awssdk.String("AWS::CloudFormation::Stack"), ResourceStatus: cfntypes.ResourceStatusUpdateComplete, Timestamp: awssdk.Time(at.Add(2 * time.Minute))}},
		{cfntypes.StackEvent{EventId: awssdk.String("e2"), StackName: awssdk.String("app"), LogicalResourceId: awssdk.String("Bucket"), PhysicalResourceId: awssdk.String("app-bucket-1"),
			ResourceType: awssdk.String("AWS::S3::Bucket"), ResourceStatus: cfntypes.ResourceStatusUpdateFailed, ResourceStatusReason: awssdk.String("Access Denied"), Timestamp: awssdk.Time(at.Add(time.Minute))}},
		{cfntypes.StackEvent{EventId: awssdk.String("e1"), StackName: awssdk.String("app"), LogicalResourceId: awssdk.String("app"),
			ResourceType: awssdk.String("AWS::CloudFormation::Stack"), ResourceStatus: cfntypes.ResourceStatusUpdateInProgress, Timestamp: awssdk.Time(at)}},
	}
	tl := NewCloudformationEventsTailer("app", 10, false, 10*time.Second, []string{StackEventStatus}, time.Hour).WithJSON(true)

	var out bytes.Buffer
	if err := tl.print(&out, events); err != nil {
		t.Fatal(err)
	}
	golden.Assert(t, "stack-events", out.Bytes())
}
