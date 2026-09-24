package awsconv

import (
	"strings"
	"testing"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	lambdatypes "github.com/aws/aws-sdk-go-v2/service/lambda/types"
	snstypes "github.com/aws/aws-sdk-go-v2/service/sns/types"

	"github.com/theazz/awless-ro/cloud"
	"github.com/theazz/awless-ro/graph"
)

// A subscription is identified by its own arn, never by its endpoint.
//
// This was found against a live account. An endpoint is not an identity of the
// subscription; it is a reference to something else, and for the lambda and sqs
// protocols it is that thing's arn. Keying the subscription by it therefore put two
// different resources on one graph node, which carried two rdf:type triples, and
// every lookup that has only an id to go on then failed outright: `show <topic>`
// printed the topic's properties and died on "cannot resolve unique type for
// resource".
func TestLambdaSubscriptionDoesNotCollideWithItsFunction(t *testing.T) {
	const functionArn = "arn:aws:lambda:eu-west-1:123456789012:function:notify"

	sub, err := NewResource(snstypes.Subscription{
		Endpoint:        awssdk.String(functionArn),
		Protocol:        awssdk.String("lambda"),
		SubscriptionArn: awssdk.String("arn:aws:sns:eu-west-1:123456789012:alerts:3f2a1c90"),
		TopicArn:        awssdk.String("arn:aws:sns:eu-west-1:123456789012:alerts"),
	})
	if err != nil {
		t.Fatal(err)
	}

	fn, err := NewResource(lambdatypes.FunctionConfiguration{
		FunctionArn:  awssdk.String(functionArn),
		FunctionName: awssdk.String("notify"),
	})
	if err != nil {
		t.Fatal(err)
	}

	if sub.Id() == fn.Id() {
		t.Fatalf("the subscription and the function share the id %q, so they are one node "+
			"in the graph with two types", sub.Id())
	}
	if sub.Id() != "arn:aws:sns:eu-west-1:123456789012:alerts:3f2a1c90" {
		t.Errorf("subscription id is %q, want its own arn", sub.Id())
	}

	// The endpoint is still there to be read; it just is not the identity.
	if got := sub.Properties()["Endpoint"]; got != functionArn {
		t.Errorf("Endpoint = %v, want %s", got, functionArn)
	}
}

// The same collision with the other protocol whose endpoint is an arn.
func TestSqsSubscriptionDoesNotCollideWithItsQueue(t *testing.T) {
	const queueArn = "arn:aws:sqs:eu-west-1:123456789012:jobs"

	sub, err := NewResource(snstypes.Subscription{
		Endpoint:        awssdk.String(queueArn),
		Protocol:        awssdk.String("sqs"),
		SubscriptionArn: awssdk.String("arn:aws:sns:eu-west-1:123456789012:alerts:7d4e5f18"),
	})
	if err != nil {
		t.Fatal(err)
	}

	if sub.Id() == queueArn {
		t.Errorf("the subscription took the queue's arn %q as its id", queueArn)
	}
}

// The same endpoint subscribed to two topics is two subscriptions, and used to be one
// node: the second sync overwrote the first.
func TestTwoTopicsOneEndpointAreTwoSubscriptions(t *testing.T) {
	const endpoint = "ops@example.com"

	first, err := NewResource(snstypes.Subscription{
		Endpoint:        awssdk.String(endpoint),
		Protocol:        awssdk.String("email"),
		SubscriptionArn: awssdk.String("arn:aws:sns:eu-west-1:1:alerts:sub-1"),
		TopicArn:        awssdk.String("arn:aws:sns:eu-west-1:1:alerts"),
	})
	if err != nil {
		t.Fatal(err)
	}
	second, err := NewResource(snstypes.Subscription{
		Endpoint:        awssdk.String(endpoint),
		Protocol:        awssdk.String("email"),
		SubscriptionArn: awssdk.String("arn:aws:sns:eu-west-1:1:billing:sub-2"),
		TopicArn:        awssdk.String("arn:aws:sns:eu-west-1:1:billing"),
	})
	if err != nil {
		t.Fatal(err)
	}

	if first.Id() == second.Id() {
		t.Errorf("both subscriptions have the id %q, so one replaces the other", first.Id())
	}
}

// Until the endpoint owner confirms, SNS returns the literal "PendingConfirmation" in
// place of an arn — the same string for every such subscription, so it cannot be an
// id. The fallback is the subscription's natural key, which is stable across syncs.
func TestPendingConfirmationGetsAStableIdOfItsOwn(t *testing.T) {
	pending := func(topic, endpoint string) *graph.Resource {
		res, err := NewResource(snstypes.Subscription{
			Endpoint:        awssdk.String(endpoint),
			Protocol:        awssdk.String("email"),
			SubscriptionArn: awssdk.String("PendingConfirmation"),
			TopicArn:        awssdk.String(topic),
		})
		if err != nil {
			t.Fatal(err)
		}
		return res
	}

	a := pending("arn:aws:sns:eu-west-1:1:alerts", "ops@example.com")
	b := pending("arn:aws:sns:eu-west-1:1:billing", "ops@example.com")
	c := pending("arn:aws:sns:eu-west-1:1:alerts", "other@example.com")

	if a.Id() == "PendingConfirmation" {
		t.Fatal("the placeholder arn was used as the id, so every pending subscription is one node")
	}
	if !strings.HasPrefix(a.Id(), "awls-") {
		t.Errorf("id is %q, want the generated form used elsewhere for resources AWS gives no id", a.Id())
	}
	if a.Type() != cloud.Subscription {
		t.Errorf("type is %q, want %q", a.Type(), cloud.Subscription)
	}

	// Different topic, or different endpoint, is a different subscription.
	if a.Id() == b.Id() {
		t.Error("two pending subscriptions on different topics share an id")
	}
	if a.Id() == c.Id() {
		t.Error("two pending subscriptions with different endpoints share an id")
	}

	// And the same one twice is the same id, or every sync would invent a new node.
	if again := pending("arn:aws:sns:eu-west-1:1:alerts", "ops@example.com"); again.Id() != a.Id() {
		t.Errorf("the same pending subscription got %q then %q", a.Id(), again.Id())
	}
}
