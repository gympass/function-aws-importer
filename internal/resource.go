package internal

import (
	"fmt"
	"strings"

	"github.com/crossplane/crossplane-runtime/pkg/fieldpath"
	"github.com/crossplane/crossplane-runtime/pkg/meta"
	fnv1 "github.com/crossplane/function-sdk-go/proto/v1"
	"github.com/crossplane/function-sdk-go/request"
	"github.com/crossplane/function-sdk-go/resource"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

const externalNameAnnotationPath = `metadata.annotations["` + meta.AnnotationKeyExternalName + `"]`

// Resources aggregates desired and observed managed resources from a request
type Resources struct {
	desiredComposed  map[string]Resource
	observedComposed map[string]Resource
}

// NewResources creates Resources based on req
func NewResources(req *fnv1.RunFunctionRequest) (Resources, error) {
	desiredComposed, err := request.GetDesiredComposedResources(req)
	if err != nil {
		return Resources{}, fmt.Errorf("extracting desired composed resources from request: %v", err)
	}
	observedComposed, err := request.GetObservedComposedResources(req)
	if err != nil {
		return Resources{}, fmt.Errorf("extracting observed composed resources from request: %v", err)
	}

	resources := Resources{
		desiredComposed:  make(map[string]Resource, len(desiredComposed)),
		observedComposed: make(map[string]Resource, len(observedComposed)),
	}

	for name, desired := range desiredComposed {
		resources.desiredComposed[string(name)] = newResourceFromDesired(name, desired)
	}
	for name, obs := range observedComposed {
		res, err := newResourceFromObserved(name, obs)
		if err != nil {
			return Resources{}, fmt.Errorf("interpreting %q observed resource: %v", name, err)
		}
		resources.observedComposed[string(name)] = res
	}

	return resources, nil
}

// AllHaveExternalNamesSet returns true if all observed composed resources have the external-name annotation set.
func (r Resources) AllHaveExternalNamesSet() bool {
	for _, res := range r.observedComposed {
		if len(res.externalName) == 0 {
			return false
		}
	}
	return true
}

// ObservedExternalNames returns a map of all observed external names, indexed by the resource name on the composition
func (r Resources) ObservedExternalNames() map[string]string {
	names := make(map[string]string, len(r.observedComposed))
	for name, res := range r.observedComposed {
		names[name] = res.externalName
	}
	return names
}

// LenDesired returns how many desired composed resources there are
func (r Resources) LenDesired() int {
	return len(r.desiredComposed)
}

// LenObserved returns how many observed composed resources there are
func (r Resources) LenObserved() int {
	return len(r.observedComposed)
}

// ForEachDesiredComposed executes the given fn passing each desired composed resource as input
func (r Resources) ForEachDesiredComposed(fn func(desiredComposed Resource) error) error {
	for name, dc := range r.desiredComposed {
		if err := fn(dc); err != nil {
			return fmt.Errorf("%s: %v", name, err)
		}
	}
	return nil
}

// DesiredResourcesCompositionNames returns a list of composed resource's name as defined in their composition.
// Not to be confused with their .metadata.name
func (r Resources) DesiredResourcesCompositionNames() []string {
	var names []string
	for n := range r.desiredComposed {
		names = append(names, n)
	}
	return names
}

// SetDesiredExternalName sets the external name annotation to the desired composed resource matching the given composedName argument
func (r Resources) SetDesiredExternalName(composedName string, name string) error {
	res, ok := r.desiredComposed[composedName]
	if !ok {
		return fmt.Errorf("composed name %q not found", composedName)
	}

	res.externalName = name
	err := res.setExternalNameAnnotationOnDesired(name)
	if err != nil {
		return fmt.Errorf("setting external name annotation on desired: %q: %v", composedName, err)
	}

	r.desiredComposed[composedName] = res

	return nil
}

// FoundExistingResources detects whether existing external resources have been found so far based on the existence of
// an external name in each of them. Returns true if at least one desired composed resource has its external name set
func (r Resources) FoundExistingResources() bool {
	for _, res := range r.desiredComposed {
		if len(res.externalName) > 0 {
			return true
		}
	}
	return false
}

// DesiredComposedResources returns the up-to-date desired composed resources as expected by the composition-function SDK
func (r Resources) DesiredComposedResources() map[resource.Name]*resource.DesiredComposed {
	result := make(map[resource.Name]*resource.DesiredComposed)
	for n, res := range r.desiredComposed {
		result[resource.Name(n)] = res.desiredComposed
	}
	return result
}

// DesiredExternalNames returns a map of external names indexed by the desired composed resource name on the composition
func (r Resources) DesiredExternalNames() map[string]string {
	names := make(map[string]string)
	for n, res := range r.desiredComposed {
		names[n] = res.externalName
	}
	return names
}

// Resource represents a single managed resource, be it observed or desired
type Resource struct {
	// TODO(lcaparelli): externalName is only used for observed. desiredComposed only for desired.
	// Improve cohesion, maybe split into separate 'basic' resource that's composed between both types.
	k8sName         string
	gvk             schema.GroupVersionKind
	compositionName string
	externalName    string
	desiredComposed *resource.DesiredComposed
}

func newResourceFromDesired(compositionName resource.Name, composed *resource.DesiredComposed) Resource {
	return Resource{
		k8sName:         composed.Resource.GetName(),
		gvk:             composed.Resource.GroupVersionKind(),
		compositionName: string(compositionName),
		desiredComposed: composed,
	}
}

func newResourceFromObserved(compositionName resource.Name, composed resource.ObservedComposed) (Resource, error) {
	res := Resource{
		k8sName:         composed.Resource.GetName(),
		gvk:             composed.Resource.GroupVersionKind(),
		compositionName: string(compositionName),
	}

	extName, err := composed.Resource.GetString(externalNameAnnotationPath)
	if ignoreNotFound(err) != nil {
		return Resource{}, fmt.Errorf("getting %q value: %v", externalNameAnnotationPath, err)
	}

	res.externalName = extName
	return res, nil
}

// K8sName returns the composed resource's name on Kubernetes, its .metadata.name
func (r Resource) K8sName() string {
	return r.k8sName
}

// GroupKind returns the composed resource's group-kind as lower-case string (eg, 'securitygroups.ec2.aws.upbound.io')
func (r Resource) GroupKind() string {
	return strings.ToLower(r.gvk.GroupKind().String())
}

// CompositionName returns the composed resource's name as defined in the composition
func (r Resource) CompositionName() string {
	return r.compositionName
}

func (r Resource) setExternalNameAnnotationOnDesired(name string) error {
	return r.desiredComposed.Resource.SetString(externalNameAnnotationPath, name)
}

func ignoreNotFound(err error) error {
	if fieldpath.IsNotFound(err) {
		return nil
	}
	return err
}
