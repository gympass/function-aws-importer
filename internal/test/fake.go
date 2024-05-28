package test

import (
	"context"
	"slices"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/resourcegroupstaggingapi"
	"github.com/aws/aws-sdk-go-v2/service/resourcegroupstaggingapi/types"
)

var _ resourcegroupstaggingapi.GetResourcesAPIClient = &FakeGetResourcesAPIClient{}

type FakeGetResourcesAPIClient struct {
	Resources []types.ResourceTagMapping
}

func (f *FakeGetResourcesAPIClient) GetResources(ctx context.Context, input *resourcegroupstaggingapi.GetResourcesInput, opts ...func(*resourcegroupstaggingapi.Options)) (*resourcegroupstaggingapi.GetResourcesOutput, error) {
	out := &resourcegroupstaggingapi.GetResourcesOutput{}

	for _, existingMapping := range f.Resources {
		for _, tag := range existingMapping.Tags {
			for _, filter := range input.TagFilters {
				if aws.ToString(tag.Key) == aws.ToString(filter.Key) {
					if slices.Contains(filter.Values, aws.ToString(tag.Value)) {
						// we're kinda cheating by not respecting resources per page, etc
						// but should be ok for what we need to test
						out.ResourceTagMappingList = append(out.ResourceTagMappingList, existingMapping)
					}
				}
			}
		}
	}

	return out, nil
}
