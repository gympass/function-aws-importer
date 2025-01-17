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