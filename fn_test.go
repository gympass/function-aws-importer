package main

import (
	"context"
	"testing"

	fnv1 "github.com/crossplane/function-sdk-go/proto/v1"
	"github.com/google/go-cmp/cmp"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/testing/protocmp"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/resourcegroupstaggingapi/types"
	"github.com/crossplane/crossplane-runtime/pkg/logging"
	runtimeresource "github.com/crossplane/crossplane-runtime/pkg/resource"
	"github.com/crossplane/function-sdk-go/resource"
	"github.com/crossplane/function-sdk-go/response"
	"github.com/stretchr/testify/suite"
	"google.golang.org/protobuf/types/known/durationpb"

	"github.com/gympass/function-aws-importer/input/v1beta1"
	"github.com/gympass/function-aws-importer/internal/test"
)

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
func (s *functionSuite) req() *fnv1.RunFunctionRequest {
	return &fnv1.RunFunctionRequest{
		Input: resource.MustStructObject(s.in),
		Desired: &fnv1.State{
			Resources: map[string]*fnv1.Resource{
				"someXR": {Resource: resource.MustStructJSON(`
					{
						"apiVersion": "acme.io/v1beta1",
						"kind": "XSomeResource",
						"metadata": {
							"name": "test"
						},
						"spec": {
							"deletionPolicy": "Orphan"
                        }
                    }`)},
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
					}`)},
				"test-0-ipv4": {Resource: resource.MustStructJSON(`
					{
						"apiVersion": "ec2.aws.upbound.io/v1beta1",
						"kind": "SecurityGroupIngressRule",
						"metadata": {
							"name": "test-0-ipv4"
						},
						"spec": {
							"deletionPolicy": "Orphan",
							"forProvider": {
								"cidrIpv4": "192.1.0.0/16",
								"description": "grant ingress on port 5432/TCP",
								"fromPort": 5432,
								"ipProtocol": "tcp",
								"region": "us-east-1",
								"securityGroupIdRef": {
									"name": "test",
									"policy": {
										"resolution": "Required",
										"resolve": "Always"
									}
								},
								"toPort": 5432
							}
						}
					}`)},
				"test-1-ipv4": {Resource: resource.MustStructJSON(`
					{
						"apiVersion": "ec2.aws.upbound.io/v1beta1",
						"kind": "SecurityGroupIngressRule",
						"metadata": {
							"name": "test-1-ipv4"
						},
						"spec": {
							"deletionPolicy": "Orphan",
							"forProvider": {
								"cidrIpv4": "192.2.0.0/16",
								"description": "grant ingress on port 5432/TCP",
								"fromPort": 5432,
								"ipProtocol": "tcp",
								"region": "us-east-1",
								"securityGroupIdRef": {
									"name": "test",
									"policy": {
										"resolution": "Required",
										"resolve": "Always"
									}
								},
								"toPort": 5432
							}
						}
					}`)},
			},
		},
	}
}

func (s *functionSuite) reqWithObservedExternalName(externalName string) *fnv1.RunFunctionRequest {
	return &fnv1.RunFunctionRequest{
		Input: resource.MustStructObject(s.in),
		Desired: &fnv1.State{
			Resources: map[string]*fnv1.Resource{
				"someXR": {Resource: resource.MustStructJSON(`
					{
						"apiVersion": "acme.io/v1beta1",
						"kind": "XSomeResource",
						"metadata": {
							"name": "test"
						},
						"spec": {
							"deletionPolicy": "Orphan"
                        }
                    }`)},
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
					}`)},
				"test-0-ipv4": {Resource: resource.MustStructJSON(`
					{
						"apiVersion": "ec2.aws.upbound.io/v1beta1",
						"kind": "SecurityGroupIngressRule",
						"metadata": {
							"name": "test-0-ipv4"
						},
						"spec": {
							"deletionPolicy": "Orphan",
							"forProvider": {
								"cidrIpv4": "192.1.0.0/16",
								"description": "grant ingress on port 5432/TCP",
								"fromPort": 5432,
								"ipProtocol": "tcp",
								"region": "us-east-1",
								"securityGroupIdRef": {
									"name": "test",
									"policy": {
										"resolution": "Required",
										"resolve": "Always"
									}
								},
								"toPort": 5432
							}
						}
					}`)},
				"test-1-ipv4": {Resource: resource.MustStructJSON(`
					{
						"apiVersion": "ec2.aws.upbound.io/v1beta1",
						"kind": "SecurityGroupIngressRule",
						"metadata": {
							"name": "test-1-ipv4"
						},
						"spec": {
							"deletionPolicy": "Orphan",
							"forProvider": {
								"cidrIpv4": "192.2.0.0/16",
								"description": "grant ingress on port 5432/TCP",
								"fromPort": 5432,
								"ipProtocol": "tcp",
								"region": "us-east-1",
								"securityGroupIdRef": {
									"name": "test",
									"policy": {
										"resolution": "Required",
										"resolve": "Always"
									}
								},
								"toPort": 5432
							}
						}
					}`)},
			},
		},
		Observed: &fnv1.State{
			Resources: map[string]*fnv1.Resource{
				"someXR": {Resource: resource.MustStructJSON(`
					{
						"apiVersion": "acme.io/v1beta1",
						"kind": "XSomeResource",
						"metadata": {
							"name": "test"
						},
						"spec": {
							"deletionPolicy": "Orphan"
                        }
                    }`)},
				"securityGroup": {Resource: resource.MustStructJSON(`
					{
						"apiVersion": "ec2.aws.upbound.io/v1beta1",
						"kind": "SecurityGroup",
						"metadata": {
							"name": "test",
							"annotations": {
								"crossplane.io/external-name": "` + externalName + `"
							}
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
					}`)},
				"test-0-ipv4": {Resource: resource.MustStructJSON(`
					{
						"apiVersion": "ec2.aws.upbound.io/v1beta1",
						"kind": "SecurityGroupIngressRule",
						"metadata": {
							"name": "test-0-ipv4",
							"annotations": {
								"crossplane.io/external-name": "` + externalName + `"
							}
						},
						"spec": {
							"deletionPolicy": "Orphan",
							"forProvider": {
								"cidrIpv4": "192.1.0.0/16",
								"description": "grant ingress on port 5432/TCP",
								"fromPort": 5432,
								"ipProtocol": "tcp",
								"region": "us-east-1",
								"securityGroupIdRef": {
									"name": "test",
									"policy": {
										"resolution": "Required",
										"resolve": "Always"
									}
								},
								"toPort": 5432
							}
						}
					}`)},
				"test-1-ipv4": {Resource: resource.MustStructJSON(`
					{
						"apiVersion": "ec2.aws.upbound.io/v1beta1",
						"kind": "SecurityGroupIngressRule",
						"metadata": {
							"name": "test-1-ipv4",
							"annotations": {
								"crossplane.io/external-name": "` + externalName + `"
							}
						},
						"spec": {
							"deletionPolicy": "Orphan",
							"forProvider": {
								"cidrIpv4": "192.2.0.0/16",
								"description": "grant ingress on port 5432/TCP",
								"fromPort": 5432,
								"ipProtocol": "tcp",
								"region": "us-east-1",
								"securityGroupIdRef": {
									"name": "test",
									"policy": {
										"resolution": "Required",
										"resolve": "Always"
									}
								},
								"toPort": 5432
							}
						}
					}`)},
			},
		},
	}
}

func (s *functionSuite) SetupTest() {
	s.in = &v1beta1.Input{}
}

func (s *functionSuite) TestRunFunction_AllResourcesExist_ShouldSetExternalNameAnnotationOnAllResources() {
	client := &test.FakeGetResourcesAPIClient{
		Resources: []types.ResourceTagMapping{
			{
				Tags: []types.Tag{
					{
						Key:   aws.String(externalNameTag),
						Value: aws.String("some-external-name"),
					},
					{
						Key:   aws.String(runtimeresource.ExternalResourceTagKeyName),
						Value: aws.String("test"),
					},
					{
						Key:   aws.String(runtimeresource.ExternalResourceTagKeyKind),
						Value: aws.String("securitygroups.ec2.aws.upbound.io"),
					},
				},
			},
			{
				Tags: []types.Tag{
					{
						Key:   aws.String(externalNameTag),
						Value: aws.String("some-external-name"),
					},
					{
						Key:   aws.String(runtimeresource.ExternalResourceTagKeyName),
						Value: aws.String("test-0-ipv4"),
					},
					{
						Key:   aws.String(runtimeresource.ExternalResourceTagKeyKind),
						Value: aws.String("securitygroupingressrules.ec2.aws.upbound.io"),
					},
				},
			},
			{
				Tags: []types.Tag{
					{
						Key:   aws.String(externalNameTag),
						Value: aws.String("some-external-name"),
					},
					{
						Key:   aws.String(runtimeresource.ExternalResourceTagKeyName),
						Value: aws.String("test-1-ipv4"),
					},
					{
						Key:   aws.String(runtimeresource.ExternalResourceTagKeyKind),
						Value: aws.String("securitygroupingressrules.ec2.aws.upbound.io"),
					},
				},
			},
		},
	}

	fn := &Function{log: logging.NewNopLogger(), client: client}
	rsp, err := fn.RunFunction(context.Background(), s.req())

	s.NoError(err)

	s.Len(rsp.Results, 1)
	s.Equalf(fnv1.Severity_SEVERITY_NORMAL, rsp.Results[0].Severity, "msg: %s", rsp.Results[0].GetMessage())
	s.Equal(durationpb.New(response.DefaultTTL), rsp.Meta.Ttl)

	// TODO(lcaparelli): find a better way to do this, preferably one that also checks
	// we didn't change anything else from desired.
	var got string
	got = rsp.GetDesired().GetResources()["securityGroup"].GetResource().
		GetFields()["metadata"].GetStructValue().
		GetFields()["annotations"].GetStructValue().
		GetFields()["crossplane.io/external-name"].GetStringValue()
	s.Equal("some-external-name", got)

	got = rsp.GetDesired().GetResources()["test-0-ipv4"].GetResource().
		GetFields()["metadata"].GetStructValue().
		GetFields()["annotations"].GetStructValue().
		GetFields()["crossplane.io/external-name"].GetStringValue()
	s.Equal("some-external-name", got)

	got = rsp.GetDesired().GetResources()["test-1-ipv4"].GetResource().
		GetFields()["metadata"].GetStructValue().
		GetFields()["annotations"].GetStructValue().
		GetFields()["crossplane.io/external-name"].GetStringValue()
	s.Equal("some-external-name", got)
}

func (s *functionSuite) TestRunFunction_SomeResourcesExist_ShouldSetExternalNameAnnotationOnSomeResources() {
	client := &test.FakeGetResourcesAPIClient{
		Resources: []types.ResourceTagMapping{
			{
				Tags: []types.Tag{
					{
						Key:   aws.String(externalNameTag),
						Value: aws.String("some-external-name"),
					},
					{
						Key:   aws.String(runtimeresource.ExternalResourceTagKeyName),
						Value: aws.String("test"),
					},
					{
						Key:   aws.String(runtimeresource.ExternalResourceTagKeyKind),
						Value: aws.String("securitygroups.ec2.aws.upbound.io"),
					},
				},
			},
			{
				Tags: []types.Tag{
					{
						Key:   aws.String(externalNameTag),
						Value: aws.String("some-external-name"),
					},
					{
						Key:   aws.String(runtimeresource.ExternalResourceTagKeyName),
						Value: aws.String("test-0-ipv4"),
					},
					{
						Key:   aws.String(runtimeresource.ExternalResourceTagKeyKind),
						Value: aws.String("securitygroupingressrules.ec2.aws.upbound.io"),
					},
				},
			},
		},
	}

	fn := &Function{log: logging.NewNopLogger(), client: client}
	rsp, err := fn.RunFunction(context.Background(), s.req())

	s.NoError(err)

	s.Len(rsp.Results, 1)
	s.Equalf(fnv1.Severity_SEVERITY_NORMAL, rsp.Results[0].Severity, "msg: %s", rsp.Results[0].GetMessage())
	s.Equal(durationpb.New(response.DefaultTTL), rsp.Meta.Ttl)

	// TODO(lcaparelli): find a better way to do this, preferably one that also checks
	// we didn't change anything else from desired.
	var got string
	got = rsp.GetDesired().GetResources()["securityGroup"].GetResource().
		GetFields()["metadata"].GetStructValue().
		GetFields()["annotations"].GetStructValue().
		GetFields()["crossplane.io/external-name"].GetStringValue()
	s.Equal("some-external-name", got)

	got = rsp.GetDesired().GetResources()["test-0-ipv4"].GetResource().
		GetFields()["metadata"].GetStructValue().
		GetFields()["annotations"].GetStructValue().
		GetFields()["crossplane.io/external-name"].GetStringValue()
	s.Equal("some-external-name", got)

	got = rsp.GetDesired().GetResources()["test-1-ipv4"].GetResource().
		GetFields()["metadata"].GetStructValue().
		GetFields()["annotations"].GetStructValue().
		GetFields()["crossplane.io/external-name"].GetStringValue()
	s.Empty(got)
}

func (s *functionSuite) TestRunFunction_InvalidInput_ShouldFail() {
	testCases := []struct {
		name string
		in   *v1beta1.Input
	}{
		{
			name: "Input has a tag filter with no key",
			in: &v1beta1.Input{
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
			req := &fnv1.RunFunctionRequest{Input: resource.MustStructObject(tc.in)}

			fn := &Function{log: logging.NewNopLogger()}
			rsp, err := fn.RunFunction(context.Background(), req)

			s.NoError(err)

			s.Len(rsp.Results, 1)
			s.Equalf(fnv1.Severity_SEVERITY_FATAL, rsp.Results[0].Severity, "msg: %s", rsp.Results[0].GetMessage())
			s.Equal(durationpb.New(response.DefaultTTL), rsp.Meta.Ttl)

			s.Nil(rsp.Desired)
		})
	}
}

func (s *functionSuite) TestRunFunction_NoDesiredComposedResources_ShouldDoNothing() {
	req := s.req()
	req.Desired.Resources = nil

	fn := &Function{log: logging.NewNopLogger()}
	rsp, err := fn.RunFunction(context.Background(), req)

	s.NoError(err)

	s.Len(rsp.Results, 1)
	s.Equalf(fnv1.Severity_SEVERITY_WARNING, rsp.Results[0].Severity, "msg: %s", rsp.Results[0].GetMessage())
	s.Equal(durationpb.New(response.DefaultTTL), rsp.Meta.Ttl)

	s.Equal(req.Desired, rsp.Desired)
}

func (s *functionSuite) TestRunFunction_AllExternalNamesAreSetAlready_ShouldEnsureAllExternalNameTagsAreSet() {
	req := s.req()
	req.Observed = &fnv1.State{
		Resources: map[string]*fnv1.Resource{
			"securityGroup": {Resource: resource.MustStructJSON(`
				{
					"apiVersion": "ec2.aws.upbound.io/v1beta1",
					"kind": "SecurityGroup",
					"metadata": {
						"annotations": {
							"crossplane.io/composition-resource-name": "securityGroup",
							"crossplane.io/external-name": "some-external-name"
						},
						"name": "test"
					}
				}`)},
			"test-0-ipv4": {Resource: resource.MustStructJSON(`
				{
					"apiVersion": "ec2.aws.upbound.io/v1beta1",
					"kind": "SecurityGroupIngressRule",
					"metadata": {
						"annotations": {
							"crossplane.io/composition-resource-name": "test-0-ipv4",
							"crossplane.io/external-name": "some-external-name"
						},
						"name": "test-0-ipv4"
					}
				}`)},
			"test-1-ipv4": {Resource: resource.MustStructJSON(`
				{
					"apiVersion": "ec2.aws.upbound.io/v1beta1",
					"kind": "SecurityGroupIngressRule",
					"metadata": {
						"annotations": {
							"crossplane.io/composition-resource-name": "test-1-ipv4",
							"crossplane.io/external-name": "some-external-name"
						},
						"name": "test-1-ipv4"
					}
				}`)},
		},
	}

	fn := &Function{log: logging.NewNopLogger()}
	rsp, err := fn.RunFunction(context.Background(), req)

	s.NoError(err)

	s.Len(rsp.Results, 1)
	s.Equalf(fnv1.Severity_SEVERITY_NORMAL, rsp.Results[0].Severity, "msg: %s", rsp.Results[0].GetMessage())
	s.Equal(durationpb.New(response.DefaultTTL), rsp.Meta.Ttl)

	for _, r := range rsp.Desired.GetResources() {
		got := r.Resource.
			GetFields()["spec"].GetStructValue().
			GetFields()["forProvider"].GetStructValue().
			GetFields()["tags"].GetStructValue().
			GetFields()[externalNameTag].GetStringValue()

		if isManagedResource := r.Resource.GetFields()["apiVersion"].GetStringValue() != "acme.io/v1beta1"; isManagedResource {
			s.Equal("some-external-name", got)
		} else {
			s.Empty(got)
		}
	}
}

func (s *functionSuite) TestRunFunction_AllExternalNamesAreSetAlready_ShouldEnsureAllExternalNameTagsAreSetButNotAllResourcesSupportTags() {
	req := s.req()
	req.Desired = &fnv1.State{
		Resources: map[string]*fnv1.Resource{
			"tagSupport": {Resource: resource.MustStructJSON(`
				{
					"apiVersion": "ec2.aws.upbound.io/v1beta1",
					"kind": "SecurityGroup",
					"metadata": {
						"annotations": {
							"crossplane.io/composition-resource-name": "tagSupport",
							"crossplane.io/external-name": "some-external-name"
						},
						"name": "test"
					}
				}`)},
			"noTagSupport": {Resource: resource.MustStructJSON(`
				{
					"apiVersion": "ec2.aws.upbound.io/v1beta1",
					"kind": "SomeResourceWithoutTags",
					"metadata": {
						"annotations": {
							"crossplane.io/composition-resource-name": "noTagSupport",
							"crossplane.io/external-name": "some-external-name"
						},
						"name": "test"
					}
				}`)},
		},
	}
	req.Observed = &fnv1.State{
		Resources: map[string]*fnv1.Resource{
			"tagSupport": {Resource: resource.MustStructJSON(`
				{
					"apiVersion": "ec2.aws.upbound.io/v1beta1",
					"kind": "SecurityGroup",
					"metadata": {
						"annotations": {
							"crossplane.io/composition-resource-name": "tagSupport",
							"crossplane.io/external-name": "some-external-name"
						},
						"name": "test"
					}
				}`)},
			"noTagSupport": {Resource: resource.MustStructJSON(`
				{
					"apiVersion": "ec2.aws.upbound.io/v1beta1",
					"kind": "SomeResourceWithoutTags",
					"metadata": {
						"annotations": {
							"crossplane.io/composition-resource-name": "noTagSupport",
							"crossplane.io/external-name": "some-external-name"
						},
						"name": "test"
					}
				}`)},
		},
	}

	fn := &Function{log: logging.NewNopLogger()}
	rsp, err := fn.RunFunction(context.Background(), req)

	s.NoError(err)

	s.Len(rsp.Results, 1)
	s.Equalf(fnv1.Severity_SEVERITY_NORMAL, rsp.Results[0].Severity, "msg: %s", rsp.Results[0].GetMessage())
	s.Equal(durationpb.New(response.DefaultTTL), rsp.Meta.Ttl)

	got := rsp.Desired.GetResources()["tagSupport"].Resource.
		GetFields()["spec"].GetStructValue().
		GetFields()["forProvider"].GetStructValue().
		GetFields()["tags"].GetStructValue().
		GetFields()[externalNameTag].GetStringValue()
	s.Equal("some-external-name", got)

	_, tagsFieldExists := rsp.Desired.GetResources()["noTagSupport"].Resource.
		GetFields()["spec"].GetStructValue().
		GetFields()["forProvider"].GetStructValue().
		GetFields()["tags"]
	s.False(tagsFieldExists)
}

func (s *functionSuite) TestRunFunction_SomeExternalNamesAreSetAlready_ShouldSetOthers() {
	client := &test.FakeGetResourcesAPIClient{
		Resources: []types.ResourceTagMapping{
			{
				Tags: []types.Tag{
					{
						Key:   aws.String(externalNameTag),
						Value: aws.String("some-external-name"),
					},
					{
						Key:   aws.String(runtimeresource.ExternalResourceTagKeyName),
						Value: aws.String("test"),
					},
					{
						Key:   aws.String(runtimeresource.ExternalResourceTagKeyKind),
						Value: aws.String("securitygroups.ec2.aws.upbound.io"),
					},
				},
			},
			{
				Tags: []types.Tag{
					{
						Key:   aws.String(externalNameTag),
						Value: aws.String("some-external-name"),
					},
					{
						Key:   aws.String(runtimeresource.ExternalResourceTagKeyName),
						Value: aws.String("test-0-ipv4"),
					},
					{
						Key:   aws.String(runtimeresource.ExternalResourceTagKeyKind),
						Value: aws.String("securitygroupingressrules.ec2.aws.upbound.io"),
					},
				},
			},
			{
				Tags: []types.Tag{
					{
						Key:   aws.String(externalNameTag),
						Value: aws.String("some-external-name"),
					},
					{
						Key:   aws.String(runtimeresource.ExternalResourceTagKeyName),
						Value: aws.String("test-1-ipv4"),
					},
					{
						Key:   aws.String(runtimeresource.ExternalResourceTagKeyKind),
						Value: aws.String("securitygroupingressrules.ec2.aws.upbound.io"),
					},
				},
			},
		},
	}

	req := s.req()
	req.Observed = &fnv1.State{
		Resources: map[string]*fnv1.Resource{
			"securityGroup": {Resource: resource.MustStructJSON(`
				{
					"apiVersion": "ec2.test.upbound.io/v1beta1",
					"kind": "SecurityGroup",
					"metadata": {
						"annotations": {
							"crossplane.io/composition-resource-name": "securityGroup",
							"crossplane.io/external-name": "sg-0ea154g1e2fd170bc"
						},
						"name": "test"
					}
				}`)},
			"test-0-ipv4": {Resource: resource.MustStructJSON(`
				{
					"apiVersion": "ec2.test.upbound.io/v1beta1",
					"kind": "SecurityGroupIngressRule",
					"metadata": {
						"annotations": {
							"crossplane.io/composition-resource-name": "test-0-ipv4"
						},
						"name": "test-0-ipv4"
					}
				}`)},
			"test-1-ipv4": {Resource: resource.MustStructJSON(`
				{
					"apiVersion": "ec2.test.upbound.io/v1beta1",
					"kind": "SecurityGroupIngressRule",
					"metadata": {
						"annotations": {
							"crossplane.io/composition-resource-name": "test-1-ipv4"
						},
						"name": "test-1-ipv4"
					}
				}`)},
		},
	}

	fn := &Function{log: logging.NewNopLogger(), client: client}
	rsp, err := fn.RunFunction(context.Background(), req)

	s.NoError(err)

	s.Len(rsp.Results, 1)
	s.Equalf(fnv1.Severity_SEVERITY_NORMAL, rsp.Results[0].Severity, "msg: %s", rsp.Results[0].GetMessage())
	s.Equal(durationpb.New(response.DefaultTTL), rsp.Meta.Ttl)

	s.Equal(req.Desired.Resources["securityGroup"], rsp.Desired.Resources["securityGroup"])

	// TODO(lcaparelli): find a better way to do this, preferably one that also checks
	// we didn't change anything else from desired.
	var got string
	got = rsp.GetDesired().GetResources()["test-0-ipv4"].GetResource().
		GetFields()["metadata"].GetStructValue().
		GetFields()["annotations"].GetStructValue().
		GetFields()["crossplane.io/external-name"].GetStringValue()
	s.Equal("some-external-name", got)

	got = rsp.GetDesired().GetResources()["test-1-ipv4"].GetResource().
		GetFields()["metadata"].GetStructValue().
		GetFields()["annotations"].GetStructValue().
		GetFields()["crossplane.io/external-name"].GetStringValue()
	s.Equal("some-external-name", got)
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
	s.Equalf(fnv1.Severity_SEVERITY_FATAL, rsp.Results[0].Severity, "msg: %s", rsp.Results[0].GetMessage())
	s.Equal(durationpb.New(response.DefaultTTL), rsp.Meta.Ttl)

	s.Equal(s.req().Desired, rsp.Desired)
}

func (s *functionSuite) TestRunFunction_MultipleTagFilterMatches_ShouldFail() {
	client := &test.FakeGetResourcesAPIClient{
		Resources: []types.ResourceTagMapping{
			{
				Tags: []types.Tag{
					{
						Key:   aws.String(externalNameTag),
						Value: aws.String("some-external-name"),
					},
					{
						Key:   aws.String(runtimeresource.ExternalResourceTagKeyName),
						Value: aws.String("test"),
					},
					{
						Key:   aws.String(runtimeresource.ExternalResourceTagKeyKind),
						Value: aws.String("securitygroups.ec2.aws.upbound.io"),
					},
				},
			},
			{
				Tags: []types.Tag{
					{
						Key:   aws.String(externalNameTag),
						Value: aws.String("another-external-name"),
					},
					{
						Key:   aws.String(runtimeresource.ExternalResourceTagKeyName),
						Value: aws.String("test"),
					},
					{
						Key:   aws.String(runtimeresource.ExternalResourceTagKeyKind),
						Value: aws.String("securitygroups.ec2.aws.upbound.io"),
					},
				},
			},
		},
	}

	fn := &Function{log: logging.NewNopLogger(), client: client}
	rsp, err := fn.RunFunction(context.Background(), s.req())

	s.NoError(err)

	s.Len(rsp.Results, 1)
	s.Equalf(fnv1.Severity_SEVERITY_FATAL, rsp.Results[0].Severity, "msg: %s", rsp.Results[0].GetMessage())
	s.Equal(durationpb.New(response.DefaultTTL), rsp.Meta.Ttl)

	s.Equal(s.req().Desired, rsp.Desired)
}

func (s *functionSuite) TestRunFunction_NoTagFilterMatches_ShouldDoNothing() {
	client := &test.FakeGetResourcesAPIClient{}

	fn := &Function{log: logging.NewNopLogger(), client: client}
	rsp, err := fn.RunFunction(context.Background(), s.req())

	s.NoError(err)

	s.Len(rsp.Results, 2)
	s.Equalf(fnv1.Severity_SEVERITY_NORMAL, rsp.Results[0].Severity, "msg: %s", rsp.Results[0].GetMessage())
	s.Equalf(fnv1.Severity_SEVERITY_NORMAL, rsp.Results[1].Severity, "msg: %s", rsp.Results[0].GetMessage())
	s.Truef(proto.Equal(durationpb.New(response.DefaultTTL), rsp.Meta.Ttl), "diff: %s", cmp.Diff(durationpb.New(response.DefaultTTL), rsp.Meta.Ttl, protocmp.Transform()))

	s.Truef(proto.Equal(s.req().Desired, rsp.Desired), "diff: %s", cmp.Diff(s.req().Desired, rsp.Desired, protocmp.Transform()))
}

func (s *functionSuite) TestRunFunction_FilterMatchesButResourceHasNoExternalNameTag_ShouldFail() {
	client := &test.FakeGetResourcesAPIClient{
		Resources: []types.ResourceTagMapping{
			{
				Tags: []types.Tag{
					{
						Key:   aws.String(runtimeresource.ExternalResourceTagKeyName),
						Value: aws.String("test"),
					},
					{
						Key:   aws.String(runtimeresource.ExternalResourceTagKeyKind),
						Value: aws.String("securitygroups.ec2.aws.upbound.io"),
					},
				},
			},
			{
				Tags: []types.Tag{
					{
						Key:   aws.String(runtimeresource.ExternalResourceTagKeyName),
						Value: aws.String("test-0-ipv4"),
					},
					{
						Key:   aws.String(runtimeresource.ExternalResourceTagKeyKind),
						Value: aws.String("securitygroupingressrules.ec2.aws.upbound.io"),
					},
				},
			},
			{
				Tags: []types.Tag{
					{
						Key:   aws.String(runtimeresource.ExternalResourceTagKeyName),
						Value: aws.String("test-1-ipv4"),
					},
					{
						Key:   aws.String(runtimeresource.ExternalResourceTagKeyKind),
						Value: aws.String("securitygroupingressrules.ec2.aws.upbound.io"),
					},
				},
			},
		},
	}

	fn := &Function{log: logging.NewNopLogger(), client: client}
	rsp, err := fn.RunFunction(context.Background(), s.req())

	s.NoError(err)

	s.Len(rsp.Results, 1)
	s.Equalf(fnv1.Severity_SEVERITY_FATAL, rsp.Results[0].Severity, "msg: %s", rsp.Results[0].GetMessage())
	s.Equal(durationpb.New(response.DefaultTTL), rsp.Meta.Ttl)

	s.Equal(s.req().Desired, rsp.Desired)
}
