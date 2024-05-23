package main

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/resourcegroupstaggingapi"
	"github.com/aws/aws-sdk-go-v2/service/resourcegroupstaggingapi/types"
	"github.com/crossplane/crossplane-runtime/pkg/logging"
	fnv1beta1 "github.com/crossplane/function-sdk-go/proto/v1beta1"
	"github.com/crossplane/function-sdk-go/resource"
	"github.com/crossplane/function-sdk-go/response"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
	"google.golang.org/protobuf/types/known/durationpb"

	"github.com/gympass/function-aws-importer/input/v1beta1"
)

var _ resourcegroupstaggingapi.GetResourcesAPIClient = &mockGetResourcesAPIClient{}

// TODO(lcaparelli): consider writing a fake implementation instead,
// we can't properly test the filters we build with a mock
type mockGetResourcesAPIClient struct {
	mock.Mock
}

func (m *mockGetResourcesAPIClient) GetResources(ctx context.Context, input *resourcegroupstaggingapi.GetResourcesInput, f ...func(*resourcegroupstaggingapi.Options)) (*resourcegroupstaggingapi.GetResourcesOutput, error) {
	called := m.Called(ctx, input, f)
	return called.Get(0).(*resourcegroupstaggingapi.GetResourcesOutput), called.Error(1)
}

func TestRunFunctionSuite(t *testing.T) {
	suite.Run(t, &functionSuite{})
}

type functionSuite struct {
	suite.Suite
	in *v1beta1.Input
}

// We're using an SG for the tests, it could be any AWS managed resource with tags.
// We're using a specific MR instead of inserting a bunch of 'foos' and 'bars' to make
// the tests bear some resemblance of a real use-case.
func (s *functionSuite) req() *fnv1beta1.RunFunctionRequest {
	return &fnv1beta1.RunFunctionRequest{
		Input: resource.MustStructObject(s.in),
		Desired: &fnv1beta1.State{
			Resources: map[string]*fnv1beta1.Resource{
				"securityGroup": {Resource: resource.MustStructJSON(`
					{
						"apiVersion": "ec2.aws.upbound.io/v1beta1",
						"kind": "SecurityGroup",
						"metadata": {
							"name": "test"
						},
						"spec": {
							"deletionPolicy": "Orphan",
							"forProvider": {
								"description": "foo-bar",
								"name": "test",
								"region": "us-east-1",
								"revokeRulesOnDelete": true,
								"tags": {
									"Name": "test"
								},
								"vpcId": "some-vpc-id"
							}
						}
					}
					`)},
			},
		},
	}
}

func (s *functionSuite) SetupTest() {
	s.in = &v1beta1.Input{ResourceName: "securityGroup"}
}

func (s *functionSuite) TestRunFunction_FilterMatchesResource_ShouldSetExternalNameAnnotation() {
	client := &mockGetResourcesAPIClient{}
	client.On("GetResources", mock.Anything, mock.Anything, mock.Anything).
		Return(&resourcegroupstaggingapi.GetResourcesOutput{
			ResourceTagMappingList: []types.ResourceTagMapping{
				{
					ResourceARN: aws.String("resource1"),
					Tags: []types.Tag{{
						Key:   aws.String(externalNameTag),
						Value: aws.String("sg-0ea154g1e2fd170bc"),
					}},
				},
			},
		}, nil)

	fn := &Function{log: logging.NewNopLogger(), client: client}
	rsp, err := fn.RunFunction(context.Background(), s.req())

	s.NoError(err)

	s.Len(rsp.Results, 1)
	s.Equal(fnv1beta1.Severity_SEVERITY_NORMAL, rsp.Results[0].Severity)
	s.Equal(durationpb.New(response.DefaultTTL), rsp.Meta.Ttl)

	// TODO(lcaparelli): find a better way to do this, preferably one that also checks
	// we didn't change anything else from desired.
	var got string
	s.NotPanics(func() {
		got = rsp.GetDesired().GetResources()[string(s.in.ResourceName)].GetResource().
			GetFields()["metadata"].GetStructValue().
			GetFields()["annotations"].GetStructValue().
			GetFields()["crossplane.io/external-name"].GetStringValue()
	})

	s.Equal("sg-0ea154g1e2fd170bc", got)
}

func (s *functionSuite) TestRunFunction_NilInput_ShouldFail() {
	req := &fnv1beta1.RunFunctionRequest{}

	fn := &Function{log: logging.NewNopLogger()}
	rsp, err := fn.RunFunction(context.Background(), req)

	s.NoError(err)

	s.Len(rsp.Results, 1)
	s.Equal(fnv1beta1.Severity_SEVERITY_FATAL, rsp.Results[0].Severity)
	s.Equal(durationpb.New(response.DefaultTTL), rsp.Meta.Ttl)

	s.Nil(rsp.Desired)
}

func (s *functionSuite) TestRunFunction_InvalidInput_ShouldFail() {
	testCases := []struct {
		name string
		in   *v1beta1.Input
	}{
		{
			name: "Input has no resource name",
			in: &v1beta1.Input{
				ResourceName: "",
				TagFilters: []v1beta1.TagFilter{{
					Key:      "foo",
					Strategy: "value",
					Value:    "bar",
				}},
			},
		},
		{
			name: "Input has a tag filter with no key",
			in: &v1beta1.Input{
				ResourceName: "securitGroup",
				TagFilters: []v1beta1.TagFilter{{
					Key:      "",
					Strategy: "value",
					Value:    "bar",
				}},
			},
		},
		{
			name: "Input has a tag filter using 'value' strategy, but didn't inform the value",
			in: &v1beta1.Input{
				ResourceName: "securitGroup",
				TagFilters: []v1beta1.TagFilter{{
					Key:      "foo",
					Strategy: "value",
					Value:    "",
				}},
			},
		},
		{
			name: "Input has a tag filter using 'valuePath' strategy, but didn't inform the value path",
			in: &v1beta1.Input{
				ResourceName: "securitGroup",
				TagFilters: []v1beta1.TagFilter{{
					Key:       "foo",
					Strategy:  "valuePath",
					ValuePath: "",
				}},
			},
		},
		{
			name: "Input has a tag filter with no strategy",
			in: &v1beta1.Input{
				ResourceName: "securitGroup",
				TagFilters: []v1beta1.TagFilter{{
					Key:       "foo",
					Strategy:  "",
					ValuePath: "bar",
				}},
			},
		},
		{
			name: "Input uses an invalid strategy in a tag filter",
			in: &v1beta1.Input{
				ResourceName: "securitGroup",
				TagFilters: []v1beta1.TagFilter{{
					Key:       "foo",
					Strategy:  "invalid",
					ValuePath: "bar",
				}},
			},
		},
	}

	for _, tc := range testCases {
		s.Run(tc.name, func() {
			req := &fnv1beta1.RunFunctionRequest{Input: resource.MustStructObject(tc.in)}

			fn := &Function{log: logging.NewNopLogger()}
			rsp, err := fn.RunFunction(context.Background(), req)

			s.NoError(err)

			s.Len(rsp.Results, 1)
			s.Equal(fnv1beta1.Severity_SEVERITY_FATAL, rsp.Results[0].Severity)
			s.Equal(durationpb.New(response.DefaultTTL), rsp.Meta.Ttl)

			s.Nil(rsp.Desired)
		})
	}
}

func (s *functionSuite) TestRunFunction_ExternalNameIsSetAlready_ShouldDoNothing() {
	req := s.req()
	req.Observed = &fnv1beta1.State{
		Resources: map[string]*fnv1beta1.Resource{
			"securityGroup": {Resource: resource.MustStructJSON(`
				{
					"apiVersion": "ec2.aws.upbound.io/v1beta1",
					"kind": "SecurityGroup",
					"metadata": {
						"annotations": {
							"crossplane.io/composition-resource-name": "securityGroup",
							"crossplane.io/external-name": "sg-0ea154g1e2fd170bc"
						}
					}
				}`)},
		},
	}

	fn := &Function{log: logging.NewNopLogger()}
	rsp, err := fn.RunFunction(context.Background(), req)

	s.NoError(err)

	s.Len(rsp.Results, 1)
	s.Equal(fnv1beta1.Severity_SEVERITY_NORMAL, rsp.Results[0].Severity)
	s.Equal(durationpb.New(response.DefaultTTL), rsp.Meta.Ttl)

	s.Equal(req.Desired, rsp.Desired)
}

func (s *functionSuite) TestRunFunction_UnresolvableTagFilter_ShouldFail() {
	s.in.TagFilters = []v1beta1.TagFilter{{
		Key:       "key",
		Strategy:  "valuePath",
		ValuePath: "some.field.that.doesnt.exist",
	}}

	fn := &Function{log: logging.NewNopLogger()}
	rsp, err := fn.RunFunction(context.Background(), s.req())

	s.NoError(err)

	s.Len(rsp.Results, 1)
	s.Equal(fnv1beta1.Severity_SEVERITY_FATAL, rsp.Results[0].Severity)
	s.Equal(durationpb.New(response.DefaultTTL), rsp.Meta.Ttl)

	s.Equal(s.req().Desired, rsp.Desired)
}

func (s *functionSuite) TestRunFunction_MultipleTagFilterMatches_ShouldFail() {
	client := &mockGetResourcesAPIClient{}
	client.On("GetResources", mock.Anything, mock.Anything, mock.Anything).
		Return(&resourcegroupstaggingapi.GetResourcesOutput{
			ResourceTagMappingList: []types.ResourceTagMapping{
				{ResourceARN: aws.String("resource1")},
				{ResourceARN: aws.String("resource2")},
			},
		}, nil)

	fn := &Function{log: logging.NewNopLogger(), client: client}
	rsp, err := fn.RunFunction(context.Background(), s.req())

	s.NoError(err)

	s.Len(rsp.Results, 1)
	s.Equal(fnv1beta1.Severity_SEVERITY_FATAL, rsp.Results[0].Severity)
	s.Equal(durationpb.New(response.DefaultTTL), rsp.Meta.Ttl)

	s.Equal(s.req().Desired, rsp.Desired)
}

func (s *functionSuite) TestRunFunction_NoTagFilterMatches_ShouldDoNothing() {
	client := &mockGetResourcesAPIClient{}
	client.On("GetResources", mock.Anything, mock.Anything, mock.Anything).
		Return(&resourcegroupstaggingapi.GetResourcesOutput{}, nil)

	fn := &Function{log: logging.NewNopLogger(), client: client}
	rsp, err := fn.RunFunction(context.Background(), s.req())

	s.NoError(err)

	s.Len(rsp.Results, 1)
	s.Equal(fnv1beta1.Severity_SEVERITY_NORMAL, rsp.Results[0].Severity)
	s.Equal(durationpb.New(response.DefaultTTL), rsp.Meta.Ttl)

	s.Equal(s.req().Desired, rsp.Desired)
}

func (s *functionSuite) TestRunFunction_FilterMatchesButResourceHasNoExternalNameTag_ShouldFail() {
	client := &mockGetResourcesAPIClient{}
	client.On("GetResources", mock.Anything, mock.Anything, mock.Anything).
		Return(&resourcegroupstaggingapi.GetResourcesOutput{
			ResourceTagMappingList: []types.ResourceTagMapping{
				{
					ResourceARN: aws.String("resource1"),
					Tags: []types.Tag{{
						Key:   aws.String("key"),
						Value: aws.String("value"),
					}},
				},
			},
		}, nil)

	fn := &Function{log: logging.NewNopLogger(), client: client}
	rsp, err := fn.RunFunction(context.Background(), s.req())

	s.NoError(err)

	s.Len(rsp.Results, 1)
	s.Equal(fnv1beta1.Severity_SEVERITY_FATAL, rsp.Results[0].Severity)
	s.Equal(durationpb.New(response.DefaultTTL), rsp.Meta.Ttl)

	s.Equal(s.req().Desired, rsp.Desired)
}
