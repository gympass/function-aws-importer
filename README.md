# function-aws-importer

A Crossplane Composition Function that automatically imports existing resources to Crossplane even if their external-name
is non-deterministic, like in EC2 Security Groups, Route53 Hosted Zones, etc.

It filters resources from AWS using the 
[Resource Groups Tagging API ](https://docs.aws.amazon.com/resourcegroupstagging/latest/APIReference/overview.html) based
on tags Crossplane inserts automatically (`crossplane-name` and `crossplane-kind`), then gets the external-name value from
one the resource's tags, specifically `crossplane-external-name`. This tag is populated automatically by the function
based on the composed resource's "crossplane.io/external-name" annotation, if it exists (which should be true once the 
resource is first created).

The function never sets any value to the "crossplane.io/external-name" annotation if it's already present. The annotation
continues to be the single source of truth for the external-name, and information always flows from it to tags if it's already
present.

The function's main goal is to avoid errors or duplication of resources on AWS when a Managed Resource is deleted by mistake,
or in catastrophic events that lead to the recreation of Kubernetes clusters while the external resources still exist on AWS.

For more details, see the [design docs](./design/README.md).

## Usage

You can use the function by inserting a pipeline step that runs after you define the Managed Resources you want to ensure
importing happens for:

```yaml
- step: import-sg-if-exists
  functionRef:
   name: function-aws-importer
```

By inserting this patch as shown above, the function will try to import all resources that were defined in previous steps.

This incomplete composition illustrates how they must complement each other:

```yaml
apiVersion: apiextensions.crossplane.io/v1
kind: Composition
spec:
   mode: Pipeline
   pipeline:
   - step: ensure-security-group
     functionRef:
        name: function-patch-and-transform
     input:
        apiVersion: pt.fn.crossplane.io/v1beta1
        kind: Resources
        resources:
        - name: securityGroup
        base:
           apiVersion: ec2.aws.upbound.io/v1beta1
           kind: SecurityGroup
           spec:
              deletionPolicy: Orphan
              forProvider:
                # etc
                # etc
        patches:
          # etc
          # etc
   - step: import-sg-if-exists
     functionRef:
        name: function-aws-importer
```

The composed resources must support tagging via `.spec.forProvider.tags`. The function patches this field in composed
resources when rendering the composition with the value from the "crossplane.io/external-name" annotation.

### Tag filters (optional input)

For each composed managed resource, the function calls the Resource Groups Tagging API with tag filters that always
include `crossplane-name` (the resource’s Kubernetes metadata name) and `crossplane-kind` (lower-case `Kind.group`). You
can add **extra** filters via the optional `Input.tagFilters` list so AWS matches only resources in a narrower scope (for
example an account- or environment-specific tag).

Each entry has:

- **`key`**: tag key sent to AWS.
- **`strategy`**: how the tag value is chosen:
  - `value` — use the literal **`value`** field.
  - `valuePath` — read the composite (XR) field at **`valuePath`** and use that string as the tag value (same fieldpath
    style as elsewhere in Crossplane).

Those filters are combined with `crossplane-name` and `crossplane-kind` for every lookup; all must match for a resource
to be considered.

Example:

```yaml
input:
  apiVersion: aws-importer.fn.wellhub.cloud/v1beta1
  kind: Input
  tagFilters:
    - key: Environment
      strategy: value
      value: production
    - key: TenantId
      strategy: valuePath
      valuePath: spec.tenantId
```

The full `Input` schema (including `tagFilters` and `equivalentEmptyExternalNames`) is in the
[Input CRD](./package/input/aws-importer.fn.wellhub.cloud_inputs.yaml).

### Placeholder external names (optional input)

If the provider sets `crossplane.io/external-name` to a **placeholder** (for example `sgr-stub` on a security group rule)
before this function runs, the function would otherwise treat any non-empty annotation as final and skip the AWS import
path. You can list those placeholder values per managed resource **GroupKind** so they are ignored for that decision only
(the real ID is still resolved from AWS using `crossplane-name` / `crossplane-kind` tags and `crossplane-external-name` on
the resource).

Configure the optional function `Input` field `equivalentEmptyExternalNames`: keys are **GroupKind** strings in the same
form this function uses for the `crossplane-kind` tag filter (lower-case `Kind.group`, e.g.
`securitygroupingressrule.ec2.aws.upbound.io` for `SecurityGroupIngressRule`). Keys are matched case-insensitively.
Values are lists of annotation strings to treat as unset; most of the time you only need one value per kind.

Example pipeline step:

```yaml
- step: import-sg-if-exists
  functionRef:
    name: function-aws-importer
  input:
    apiVersion: aws-importer.fn.wellhub.cloud/v1beta1
    kind: Input
    equivalentEmptyExternalNames:
      securitygroupingressrule.ec2.aws.upbound.io:
        - sgr-stub
```

## Development

Run the function locally:

```shell
make run
```

Run tests:

```shell
make test
```

Render the example (the function must already be running locally):

```shell
make render
```

Build and push the function with a `dev` tag:

```shell
FUNCTION_REGISTRY=my.cool.oci.registry make build-and-push-dev
```

## Known Issues
- Because the function is the one to ensure the crossplane-external-name tag on resources when the composition is rendered,
it will not automatically import resources that only exist on AWS and were not created when the function was already in use.
In this scenario, the function fails, as it finds the resource on AWS, but it has no tag to import it.
This would not be a problem if Upjet itself ensured this tag on all resources: https://github.com/crossplane/upjet/issues/408