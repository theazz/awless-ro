package awsfetch

import (
	"context"
	"fmt"
	"strconv"
	"testing"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/apigatewayv2"
	apigatewayv2types "github.com/aws/aws-sdk-go-v2/service/apigatewayv2/types"
)

// pagedApigatewayv2 answers GetApis across several pages, echoing the continuation
// token the way API Gateway does.
type pagedApigatewayv2 struct {
	Apigatewayv2API
	pages [][]apigatewayv2types.Api
	calls int
}

func (p *pagedApigatewayv2) GetApis(_ context.Context, in *apigatewayv2.GetApisInput, _ ...func(*apigatewayv2.Options)) (*apigatewayv2.GetApisOutput, error) {
	p.calls++
	idx := tokenIndex(awssdk.ToString(in.NextToken))
	out := &apigatewayv2.GetApisOutput{Items: p.pages[idx]}
	if idx+1 < len(p.pages) {
		out.NextToken = awssdk.String(strconv.Itoa(idx + 1))
	}
	return out, nil
}

// The SDK has no GetApis paginator, so the fetcher walks NextToken by hand and
// has to keep going past the first page.
func TestApisAreFetchedAcrossPages(t *testing.T) {
	api := &pagedApigatewayv2{pages: [][]apigatewayv2types.Api{
		{{ApiId: awssdk.String("a1")}, {ApiId: awssdk.String("a2")}},
		{{ApiId: awssdk.String("a3")}},
		{{ApiId: awssdk.String("a4")}},
	}}

	funcs := BuildApigatewayv2FetchFuncs(NewConfig(&AWSAPI{Apigatewayv2: api}))
	got := ids(runFetcher(t, funcs, "api"))

	want := []string{"a1", "a2", "a3", "a4"}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("got %v, want %v", got, want)
	}
	if api.calls != 3 {
		t.Errorf("GetApis called %d times, want 3", api.calls)
	}
}
