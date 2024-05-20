// Package v1beta1 contains the input type for this Function
// +kubebuilder:object:generate=true
// +groupName=template.fn.crossplane.io
// +versionName=v1beta1
package v1beta1

import (
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/resourcegroupstaggingapi/types"
	"github.com/crossplane/crossplane-runtime/pkg/errors"
	"github.com/crossplane/function-sdk-go/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// This isn't a custom resource, in the sense that we never install its CRD.
// It is a KRM-like object, so we generate a CRD to describe its schema.

// Input can be used to provide input to this Function.
// +kubebuilder:object:root=true
// +kubebuilder:storageversion
// +kubebuilder:resource:categories=crossplane
type Input struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	ResourceName resource.Name `json:"resourceName"`
	// +optional
	TagFilters []TagFilter `json:"tagFilters"`
}

func (in *Input) ResolveTagFilters(xr *resource.Composite) ([]types.TagFilter, error) {
	var filters []types.TagFilter
	for _, tf := range in.TagFilters {
		// TODO(lcaparelli): consider polymorphism if this grows larger
		switch tf.Strategy {
		case StrategyValue:
			filters = append(filters, types.TagFilter{
				Key:    aws.String(tf.Key),
				Values: []string{tf.Value},
			})
		case StrategyValuePath:
			valPath := tf.ValuePath
			resolved, err := xr.Resource.GetString(valPath)
			if err != nil {
				return nil, fmt.Errorf("getting valuePath (%q) from XR: %v", valPath, err)
			}
			filters = append(filters, types.TagFilter{
				Key:    aws.String(tf.Key),
				Values: []string{resolved},
			})
		default:
			return nil, fmt.Errorf("invalid tag filter strategy: %q", tf.Strategy)
		}
	}
	return filters, nil
}

func (in *Input) Validate() error {
	if len(in.ResourceName) == 0 {
		return errors.New("resourceName must not be empty")
	}

	for _, tf := range in.TagFilters {
		if err := tf.validate(); err != nil {
			return fmt.Errorf("invalid tag filter: %v", err)
		}
	}

	return nil
}

type Strategy string

const (
	// StrategyValue represents a static value that should be used when filtering
	StrategyValue Strategy = "value"
	// StrategyValuePath represents a dynamic value that will be resolved from the given path on the XR
	StrategyValuePath Strategy = "valuePath"
)

var validStrategies = []Strategy{StrategyValue, StrategyValuePath}

type TagFilter struct {
	Key string `json:"key"`
	// +kubebuilder:validation:Enum=value;valuePath
	Strategy Strategy `json:"strategy"`
	// +optional
	Value string `json:"value,omitempty"`
	// +optional
	ValuePath string `json:"valuePath,omitempty"`
}

func (in *TagFilter) validate() error {
	if len(in.Key) == 0 {
		return errors.New(`"key" must not be empty`)
	}
	if len(in.Strategy) == 0 {
		return errors.New(`"strategy" must not be empty`)
	}

	switch in.Strategy {
	case StrategyValue:
		if len(in.Value) == 0 {
			return fmt.Errorf(`using %q strategy, but "value" is empty`, StrategyValue)
		}
	case StrategyValuePath:
		if len(in.ValuePath) == 0 {
			return fmt.Errorf(`using %q strategy, but "valuePath" is empty`, StrategyValuePath)
		}
	default:
		return fmt.Errorf("invalid strategy %q, valid options are: %v", in.Strategy, validStrategies)
	}

	return nil
}
