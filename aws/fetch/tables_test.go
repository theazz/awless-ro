package awsfetch

import (
	"context"
	"fmt"
	"sort"
	"testing"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	dynamodbtypes "github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

// pagedDynamodb lists table names one page at a time and describes them by name.
// A name with no description answers the way DynamoDB does for a table deleted
// after it was listed.
type pagedDynamodb struct {
	DynamodbAPI
	pages  [][]string
	tables map[string]dynamodbtypes.TableDescription
}

func (d pagedDynamodb) ListTables(_ context.Context, in *dynamodb.ListTablesInput, _ ...func(*dynamodb.Options)) (*dynamodb.ListTablesOutput, error) {
	page := 0
	if in.ExclusiveStartTableName != nil {
		fmt.Sscan(awssdk.ToString(in.ExclusiveStartTableName), &page)
	}
	out := &dynamodb.ListTablesOutput{TableNames: d.pages[page]}
	if page+1 < len(d.pages) {
		out.LastEvaluatedTableName = awssdk.String(fmt.Sprint(page + 1))
	}
	return out, nil
}

func (d pagedDynamodb) DescribeTable(_ context.Context, in *dynamodb.DescribeTableInput, _ ...func(*dynamodb.Options)) (*dynamodb.DescribeTableOutput, error) {
	table, ok := d.tables[awssdk.ToString(in.TableName)]
	if !ok {
		return nil, &dynamodbtypes.ResourceNotFoundException{Message: in.TableName}
	}
	return &dynamodb.DescribeTableOutput{Table: &table}, nil
}

// Every listed table is described, across every page of names, and a table deleted
// between the listing and its description is left out instead of failing the sync.
func TestTablesAreDescribedAcrossPagesAndVanishedOnesSkipped(t *testing.T) {
	api := pagedDynamodb{
		pages: [][]string{{"orders", "gone"}, {"sessions"}},
		tables: map[string]dynamodbtypes.TableDescription{
			"orders":   {TableArn: awssdk.String("arn:aws:dynamodb:eu-west-1:1:table/orders")},
			"sessions": {TableArn: awssdk.String("arn:aws:dynamodb:eu-west-1:1:table/sessions")},
		},
	}

	funcs := BuildDynamodbFetchFuncs(NewConfig(&AWSAPI{Dynamodb: api}))
	got := ids(runFetcher(t, funcs, "table"))
	sort.Strings(got)

	want := []string{"arn:aws:dynamodb:eu-west-1:1:table/orders", "arn:aws:dynamodb:eu-west-1:1:table/sessions"}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("got %v, want %v", got, want)
	}
}
