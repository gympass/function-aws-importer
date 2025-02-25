package main

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/resourcegroupstaggingapi"
	"github.com/aws/aws-sdk-go-v2/service/resourcegroupstaggingapi/types"
	"github.com/crossplane/crossplane-runtime/pkg/errors"
	runtimeresource "github.com/crossplane/crossplane-runtime/pkg/resource"
	"github.com/crossplane/function-sdk-go/logging"
	fnv1 "github.com/crossplane/function-sdk-go/proto/v1"
	"github.com/crossplane/function-sdk-go/request"
	"github.com/crossplane/function-sdk-go/resource"
	"github.com/crossplane/function-sdk-go/response"

	"github.com/gympass/function-aws-importer/input/v1beta1"
	"github.com/gympass/function-aws-importer/internal"
)

const (
	externalNameTag = "crossplane-external-name"
)

// Function returns whatever response you ask it to.
type Function struct {
	fnv1.UnimplementedFunctionRunnerServiceServer

	log    logging.Logger
	client resourcegroupstaggingapi.GetResourcesAPIClient
}

// RunFunction runs the Function.
func (f *Function) RunFunction(ctx context.Context, req *fnv1.RunFunctionRequest) (*fnv1.RunFunctionResponse, error) {
	f.log.Info("Running function",
		"tag", req.GetMeta().GetTag(),
	)

	rsp := response.To(req, response.DefaultTTL)

	in := &v1beta1.Input{}
	if err := request.GetInput(req, in); err != nil {
		response.Fatal(rsp, errors.Wrapf(err, "cannot get Function input from %T", req))
		return rsp, nil
	}

	f.log.Debug("Fetched input.",
		"input", in,
	)

	if err := in.Validate(); err != nil {
		response.Fatal(rsp, errors.Wrap(err, "invalid Function input"))
		return rsp, nil
	}

	resources, err := internal.NewResources(req)
	if err != nil {
		f.log.Info("Failed to extract observed and desired composed resources.",
			"error", err,
		)
		response.Fatal(rsp, fmt.Errorf("cannot extract observed and desired composed resources: %v", err))
		return rsp, nil
	}

	if resources.LenDesired() == 0 {
		f.log.Info("Empty desired composed resources")
		response.Warning(rsp, errors.New("found no desired composed resources. Are you running the function before other steps that define the resources? It should always run after them."))
		return rsp, nil
	}

	err = resources.EnsureExternalNameTags(externalNameTag)
	if err != nil {
		f.log.Info("Failed to ensure external name tags.",
			"error", err,
		)
		response.Fatal(rsp, fmt.Errorf("cannot ensure external name tags: %v", err))
		return rsp, nil
	}

	if resources.LenObserved() > 0 && resources.AllHaveExternalNamesSet() {
		err := response.SetDesiredComposedResources(rsp, resources.DesiredComposedResources())
		if err != nil {
			f.log.Info("Failed to set desired composed resources.",
				"error", err,
				"desired", resources.DesiredComposedResources(),
			)
			response.Fatal(rsp, errors.Wrapf(err, "cannot set desired composed resources in %T", rsp))
			return rsp, nil
		}

		externalNames := resources.ObservedExternalNames()
		f.log.Debug("External name already set for all resources",
			"externalNames", externalNames,
		)
		response.Normalf(rsp, "external name annotation already set for all resources: %v", externalNames)
		return rsp, nil
	}

	err = resources.ForEachDesiredComposed(func(desiredComposed internal.Resource) error {
		externalName, err := f.fetchExternalNameFromAWS(ctx, req, in, desiredComposed)
		if err != nil {
			return fmt.Errorf("fetching external name from AWS: %v", err)
		}

		return resources.SetDesiredExternalName(desiredComposed.CompositionName(), externalName)
	})
	if err != nil {
		f.log.Info("Failed to reconcile desired managed resource.",
			"error", err,
		)
		response.Fatal(rsp, fmt.Errorf("cannot reconcile desired managed resource: %v", err))
		return rsp, nil
	}

	if !resources.FoundExistingResources() {
		desiredResourcesCompName := resources.DesiredResourcesCompositionNames()
		f.log.Info("External resources not found", "resources", desiredResourcesCompName)
		response.Normalf(rsp, "external resources not found: %v", desiredResourcesCompName)
	}

	desiredMRs := resources.DesiredComposedResources()
	if err := response.SetDesiredComposedResources(rsp, desiredMRs); err != nil {
		f.log.Info("Failed to set desired composed resources.",
			"error", err,
			"desired", desiredMRs,
		)
		response.Fatal(rsp, errors.Wrapf(err, "cannot set desired composed resources in %T", rsp))
		return rsp, nil
	}

	desiredExternalNames := resources.DesiredExternalNames()
	response.Normalf(rsp, "added external name annotations: %v", desiredExternalNames)
	f.log.Info("Added external name annotation.",
		"externalNames", desiredExternalNames,
	)

	return rsp, nil
}

func (f *Function) fetchExternalNameFromAWS(ctx context.Context, req *fnv1.RunFunctionRequest, in *v1beta1.Input, desiredComposed internal.Resource) (string, error) {
	tagFilters, err := f.extractTagFilters(desiredComposed, req, in)
	if err != nil {
		return "", fmt.Errorf("extracting tag filters: %v", err)
	}

	tagMappings, err := f.getResourceTagMappings(ctx, tagFilters)
	if err != nil {
		return "", fmt.Errorf("getting resource tag mappings: %v", err)
	}

	if len(tagMappings) > 1 {
		f.log.Info("Cannot decide which resource to import.",
			"error", errors.New("found more than one resource matching tag filters"),
			"tagFilters", tagFilters,
			"matchingResources", extractARNs(tagMappings),
		)
		return "", fmt.Errorf("found more than one resource matching tag filters: %v", extractARNs(tagMappings))
	}

	if len(tagMappings) == 0 {
		f.log.Debug("External resource not found",
			"tagFilters", tagFilters,
		)
		return "", nil
	}

	tags := tagMappings[0].Tags
	f.log.Debug("Found resource with matching tags",
		"tags", tags,
		"tagFilters", tagFilters,
	)

	return f.extractExternalName(tags)
}

func (f *Function) extractExternalName(tags []types.Tag) (string, error) {
	var externalName string
	for _, t := range tags {
		// TODO(lcaparelli): make this a parameter for the function, allow users to fetch external-name value from any tag
		if aws.ToString(t.Key) == externalNameTag {
			externalName = aws.ToString(t.Value)
			break
		}
	}

	if len(externalName) == 0 {
		f.log.Info("Cannot fetch external name from tags.",
			"error", errors.New("tag does not exist or is empty"),
			"existingTags", tags,
			"externalNameTagKey", externalNameTag,
		)
		return "", fmt.Errorf("found resource matching tag filters, but %q tag is not present or is empty", externalNameTag)
	}
	return externalName, nil
}

func (f *Function) getResourceTagMappings(ctx context.Context, tagFilters []types.TagFilter) ([]types.ResourceTagMapping, error) {
	paginator := resourcegroupstaggingapi.NewGetResourcesPaginator(f.client, &resourcegroupstaggingapi.GetResourcesInput{
		TagFilters: tagFilters,
	})

	var tagMappings []types.ResourceTagMapping
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("getting resources tag mappings: %v", err)
		}

		for _, t := range page.ResourceTagMappingList {
			tagMappings = append(tagMappings, t)
		}
	}
	return tagMappings, nil
}

func (f *Function) extractTagFilters(desiredComposed internal.Resource, req *fnv1.RunFunctionRequest, in *v1beta1.Input) ([]types.TagFilter, error) {
	xr, err := request.GetObservedCompositeResource(req)
	if err != nil {
		return nil, fmt.Errorf("extracting observed XR from req: %v", err)
	}

	tagFilters, err := resolveTagFilters(in, xr, desiredComposed)
	if err != nil {
		f.log.Info("Failed to resolve tag filters.",
			"error", err,
			"tagFilters", in.TagFilters,
			"xr", xr,
			"managedResource", desiredComposed,
		)
		return nil, err
	}
	return tagFilters, nil
}

func resolveTagFilters(in *v1beta1.Input, xr *resource.Composite, res internal.Resource) ([]types.TagFilter, error) {
	additionalFilters, err := in.ResolveTagFilters(xr)
	if err != nil {
		return nil, fmt.Errorf("resolving input tag filters: %v", err)
	}

	return append(additionalFilters, nameAndKindFilters(res)...), nil
}

func nameAndKindFilters(res internal.Resource) []types.TagFilter {
	return []types.TagFilter{
		{
			Key:    aws.String(runtimeresource.ExternalResourceTagKeyName),
			Values: []string{res.K8sName()},
		},
		{
			Key:    aws.String(runtimeresource.ExternalResourceTagKeyKind),
			Values: []string{res.GroupKind()},
		},
	}
}

func extractARNs(tagMappings []types.ResourceTagMapping) []string {
	var arns []string
	for _, t := range tagMappings {
		arns = append(arns, aws.ToString(t.ResourceARN))
	}
	return arns
}
