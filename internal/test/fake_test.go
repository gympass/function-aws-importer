package test

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/resourcegroupstaggingapi"
	"github.com/aws/aws-sdk-go-v2/service/resourcegroupstaggingapi/types"
	"github.com/stretchr/testify/suite"
)

func TestRunFakeSuite(t *testing.T) {
	suite.Run(t, &fakeGetResourcesAPIClientSuite{})
}

type fakeGetResourcesAPIClientSuite struct {
	suite.Suite
}

func (s *fakeGetResourcesAPIClientSuite) TestGetResources_NoFiltersMatch_ShouldReturnNil() {
	testCases := []struct {
		name string
		fake *FakeGetResourcesAPIClient
	}{
		{
			name: "Fake is empty",
			fake: &FakeGetResourcesAPIClient{},
		},
		{
			name: "Fake is not empty",
			fake: &FakeGetResourcesAPIClient{
				Resources: []types.ResourceTagMapping{{
					ResourceARN: aws.String("some-arn"),
					Tags: []types.Tag{{
						Key:   aws.String("key"),
						Value: aws.String("value"),
					}},
				}},
			},
		},
	}

	for _, tc := range testCases {
		s.Run(tc.name, func() {
			gotRes, gotErr := tc.fake.GetResources(context.Background(), &resourcegroupstaggingapi.GetResourcesInput{
				TagFilters: []types.TagFilter{
					{
						Key:    aws.String("another-key"),
						Values: []string{"value"},
					},
					{
						Key:    aws.String("key"),
						Values: []string{"another-value"},
					},
				},
			})

			s.Empty(gotRes.ResourceTagMappingList)
			s.Nil(gotErr)
		})
	}
}

func (s *fakeGetResourcesAPIClientSuite) TestGetResources_OneFilterMatchesOneResource_ShouldReturnMatchingResource() {
	fake := &FakeGetResourcesAPIClient{
		Resources: []types.ResourceTagMapping{
			{
				ResourceARN: aws.String("some-arn"),
				Tags: []types.Tag{{
					Key:   aws.String("key"),
					Value: aws.String("value"),
				}},
			},
			{
				ResourceARN: aws.String("another-arn"),
				Tags: []types.Tag{{
					Key:   aws.String("another-key"),
					Value: aws.String("another-value"),
				}},
			},
		},
	}

	gotRes, gotErr := fake.GetResources(context.Background(), &resourcegroupstaggingapi.GetResourcesInput{
		TagFilters: []types.TagFilter{
			{
				Key:    aws.String("another-key"),
				Values: []string{"another-value"},
			},
			{
				Key:    aws.String("key"),
				Values: []string{"another-value"},
			},
		},
	})

	s.Nil(gotErr)

	s.Len(gotRes.ResourceTagMappingList, 1)
	s.Equal(types.ResourceTagMapping{
		ResourceARN: aws.String("another-arn"),
		Tags: []types.Tag{{
			Key:   aws.String("another-key"),
			Value: aws.String("another-value"),
		}},
	}, gotRes.ResourceTagMappingList[0])
}

func (s *fakeGetResourcesAPIClientSuite) TestGetResources_OneFilterMatchesAllResources_ShouldReturnMatchingResource() {
	fake := &FakeGetResourcesAPIClient{
		Resources: []types.ResourceTagMapping{
			{
				ResourceARN: aws.String("some-arn"),
				Tags: []types.Tag{{
					Key:   aws.String("key"),
					Value: aws.String("value"),
				}},
			},
			{
				ResourceARN: aws.String("another-arn"),
				Tags: []types.Tag{{
					Key:   aws.String("key"),
					Value: aws.String("value"),
				}},
			},
		},
	}

	gotRes, gotErr := fake.GetResources(context.Background(), &resourcegroupstaggingapi.GetResourcesInput{
		TagFilters: []types.TagFilter{
			{
				Key:    aws.String("another-key"),
				Values: []string{"another-value"},
			},
			{
				Key:    aws.String("key"),
				Values: []string{"value"},
			},
		},
	})

	s.Nil(gotErr)

	s.Len(gotRes.ResourceTagMappingList, 2)
	s.ElementsMatch([]types.ResourceTagMapping{
		{
			ResourceARN: aws.String("another-arn"),
			Tags: []types.Tag{{
				Key:   aws.String("key"),
				Value: aws.String("value"),
			}},
		},
		{
			ResourceARN: aws.String("some-arn"),
			Tags: []types.Tag{{
				Key:   aws.String("key"),
				Value: aws.String("value"),
			}},
		},
	}, gotRes.ResourceTagMappingList)
}
