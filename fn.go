package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/resourcegroupstaggingapi"
	"github.com/aws/aws-sdk-go-v2/service/resourcegroupstaggingapi/types"
	"github.com/crossplane/crossplane-runtime/pkg/errors"
	"github.com/crossplane/crossplane-runtime/pkg/fieldpath"
	runtimeresource "github.com/crossplane/crossplane-runtime/pkg/resource"
	"github.com/crossplane/function-sdk-go/logging"
	fnv1beta1 "github.com/crossplane/function-sdk-go/proto/v1beta1"
	"github.com/crossplane/function-sdk-go/request"
	"github.com/crossplane/function-sdk-go/resource"
	"github.com/crossplane/function-sdk-go/response"

	"github.com/gympass/function-aws-resource-observer/input/v1beta1"
)

const (
	externalNameAnnotationPath = `metadata.annotations["crossplane.io/external-name"]`
	externalNameTag            = "crossplane.io/external-name"
)

// Function returns whatever response you ask it to.
type Function struct {
	fnv1beta1.UnimplementedFunctionRunnerServiceServer

	log    logging.Logger
	client resourcegroupstaggingapi.GetResourcesAPIClient
}

// TODO(lcaparelli): extract into functions for readability

// RunFunction runs the Function.
func (f *Function) RunFunction(ctx context.Context, req *fnv1beta1.RunFunctionRequest) (*fnv1beta1.RunFunctionResponse, error) {
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

	observedMRs, err := request.GetObservedComposedResources(req)
	if err != nil {
		f.log.Info("Failed to get observed composed resources.",
			"error", err,
		)
		response.Fatal(rsp, errors.Wrapf(err, "cannot get observed composed resources in %T", rsp))
		return rsp, nil
	}

	observedMR, ok := observedMRs[in.ResourceName]
	if ok {
		externalName, err := observedMR.Resource.GetString(externalNameAnnotationPath)
		if err == nil {
			f.log.Debug("External name already set",
				"externalName", externalName,
				"resourceName", in.ResourceName,
			)
			response.Normalf(rsp, "external name annotation for %q is already set to %q", in.ResourceName, externalName)
			return rsp, nil
		}
		if ignoreNotFound(err) != nil {
			f.log.Info("Failed to get external name annotation from composed resource.",
				"error", err,
				"resourceName", in.ResourceName,
			)
			response.Fatal(rsp, errors.Wrapf(err, "cannot get external name annotation from composed resource %q", in.ResourceName))
			return rsp, nil
		}
	}
	xr, err := request.GetObservedCompositeResource(req)
	if err != nil {
		f.log.Info("Failed to get observed XR from req.",
			"error", err,
		)
		response.Fatal(rsp, errors.Wrapf(err, "cannot get observed XR from req"))
		return rsp, nil
	}

	desiredMRs, err := request.GetDesiredComposedResources(req)
	if err != nil {
		f.log.Info("Failed to get observed Composted Resources from req.",
			"error", err,
		)
		response.Fatal(rsp, errors.Wrap(err, "cannot get observed MRs from req"))
		return rsp, nil
	}

	desiredComposed, ok := desiredMRs[in.ResourceName]
	if !ok {
		f.log.Info("Failed to get desired Composed Resource from req.",
			"error", err,
			"resourceName", in.ResourceName,
		)
		response.Fatal(rsp, fmt.Errorf("cannot get desired MR %q from req", in.ResourceName))
		return rsp, nil

	}

	tagFilters, err := resolveTagFilters(in, xr, desiredComposed)
	if err != nil {
		f.log.Info("Failed to resolve tag filters.",
			"error", err,
			"tagFilters", in.TagFilters,
			"xr", xr,
			"managedResource", desiredComposed,
		)
		response.Fatal(rsp, errors.Wrapf(err, "cannot resolve tag filters"))
		return rsp, nil
	}

	paginator := resourcegroupstaggingapi.NewGetResourcesPaginator(f.client, &resourcegroupstaggingapi.GetResourcesInput{
		TagFilters: tagFilters,
	})

	var tagMappings []types.ResourceTagMapping
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(context.Background())
		if err != nil {
			f.log.Info("Failed to paginate resource tag mappings.",
				"error", err,
			)
			response.Fatal(rsp, errors.Wrapf(err, "cannot get resources tag mappings"))
			return rsp, nil
		}

		for _, t := range page.ResourceTagMappingList {
			tagMappings = append(tagMappings, t)
		}
	}

	if len(tagMappings) > 1 {
		f.log.Info("Ambiguous tag filters.", // TODO(lcaparelli): better word than ambiguous, maybe better message overall
			"error", errors.New("found more than one resource matching tag filters"),
			"tagFilters", tagFilters,
			"matchingResources", extractARNs(tagMappings),
		)
		response.Fatal(rsp, fmt.Errorf("found more than one resource matching tag filters: %v", extractARNs(tagMappings)))
		return rsp, nil
	}

	if len(tagMappings) == 0 {
		f.log.Debug("External resource not found",
			"tagFilters", tagFilters,
		)
		response.Normalf(rsp, "external resource (%q) not found", in.ResourceName)
		return rsp, nil
	}

	tags := tagMappings[0].Tags
	f.log.Debug("Found resource with matching tags",
		"tags", tags,
		"tagFilters", tagFilters,
	)

	var externalName string
	for _, t := range tags {
		// TODO(lcaparelli): make this a parameter for the function, allow users to fetch external-name value from any tag
		if aws.ToString(t.Key) == externalNameTag {
			externalName = aws.ToString(t.Value)
			break
		}
	}

	if len(externalName) == 0 {
		f.log.Info("Cannnot fetch external name from tags.",
			"error", errors.New("tag does not exist or is empty"),
			"existingTags", tags,
			"externalNameTagKey", externalNameTag,
		)
		response.Fatal(rsp, fmt.Errorf("found resource matching tag filters, but %q tag is not present or is empty", externalNameTag))
		return rsp, nil
	}

	err = desiredMRs[in.ResourceName].Resource.SetString(externalNameAnnotationPath, externalName)
	if err != nil {
		f.log.Info("Failed to set external-name on desired composed resource.",
			"error", err,
		)
		response.Fatal(rsp, errors.Wrapf(err, "cannot set external-name on desired composed resource"))
		return rsp, nil
	}

	if err := response.SetDesiredComposedResources(rsp, desiredMRs); err != nil {
		f.log.Info("Failed to set desired composed resources.",
			"error", err,
			"desired", desiredMRs,
		)
		response.Fatal(rsp, errors.Wrapf(err, "cannot set desired composed resources in %T", rsp))
		return rsp, nil
	}

	response.Normalf(rsp, "added external name annotation to %q with value %q", in.ResourceName, externalName)
	f.log.Info("Added external name annotation.",
		"externalName", externalName,
	)

	return rsp, nil
}

func resolveTagFilters(in *v1beta1.Input, xr *resource.Composite, mr *resource.DesiredComposed) ([]types.TagFilter, error) {
	additionalFilters, err := in.ResolveTagFilters(xr)
	if err != nil {
		return nil, fmt.Errorf("resolving input tag filters: %v", err)
	}

	return append(additionalFilters, nameAndKindFilters(mr)...), nil
}

func nameAndKindFilters(mr *resource.DesiredComposed) []types.TagFilter {
	return []types.TagFilter{
		{
			Key:    aws.String(runtimeresource.ExternalResourceTagKeyName),
			Values: []string{mr.Resource.GetName()},
		},
		{
			Key:    aws.String(runtimeresource.ExternalResourceTagKeyKind),
			Values: []string{strings.ToLower(mr.Resource.GroupVersionKind().GroupKind().String())},
		},
	}
}

func ignoreNotFound(err error) error {
	if fieldpath.IsNotFound(err) {
		return nil
	}
	return err
}

func extractARNs(tagMappings []types.ResourceTagMapping) []string {
	var arns []string
	for _, t := range tagMappings {
		arns = append(arns, aws.ToString(t.ResourceARN))
	}
	return arns
}
