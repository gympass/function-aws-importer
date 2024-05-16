# function-aws-resource-observer

A Crossplane Composition Function that automatically imports existing resources to Crossplane even if their external-name
is non-deterministic, like in EC2 Security Groups, Route53 Hosted Zones, etc.

It filters resources from AWS using the 
[Resource Groups Tagging API ](https://docs.aws.amazon.com/resourcegroupstagging/latest/APIReference/overview.html) based
on tags Crossplane inserts automatically (`crossplane-name` and `crossplane-kind`), then gets the external-name value from
one the resource's tags, specifically `crossplane.io/external-name`.

Its main goal is to avoid errors or duplication of resources on AWS when a Managed Resource is deleted by mistake, or in 
catastrophic events that lead to the recreation of Kubernetes clusters while the external resources still exist on AWS.

## Usage

Your composition must guarantee that external-name tag exists and has a valid value. This can be achieved with a patch
similar to this:

```yaml
- type: ToCompositeFieldPath
  fromFieldPath: metadata.annotations[crossplane.io/external-name]
  toFieldPath: status.externalName
- type: FromCompositeFieldPath
  fromFieldPath: status.externalName
  toFieldPath: spec.forProvider.tags[crossplane.io/external-name]
```

When the resource is first created, Crossplane will set the annotation automatically, and the next time the composition
is rendered, these patches will run and ensure the tag is present, with the value that's set in the annotation.

You can use the function by inserting a pipeline step that runs after you define the Managed Resource you want to ensure
importing happens:

```yaml
- step: import-sg-if-exists
  functionRef:
   name: function-aws-resource-observer
  input:
   resourceName: securityGroup
```

`input.resourceName` must match the name that was assigned to the Managed Resource in the composition (not to be confused
with the actual resource `metadata.name`).

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
        - name: securityGroup # this name must match the one in the function's input
        base:
           apiVersion: ec2.aws.upbound.io/v1beta1
           kind: SecurityGroup
           spec:
              deletionPolicy: Orphan
              forProvider:
                # etc
                # etc
        patches:
        - type: ToCompositeFieldPath
          fromFieldPath: metadata.annotations[crossplane.io/external-name]
          toFieldPath: status.externalName
        - type: FromCompositeFieldPath
          fromFieldPath: status.externalName
          toFieldPath: spec.forProvider.tags[crossplane.io/external-name]
   - step: import-sg-if-exists
     functionRef:
        name: function-aws-resource-observer
     input:
        resourceName: securityGroup # must match the name of the resource, see comment above
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
- the patch to copy the external name from the MR to the XR, then back from the XR into the MR's tags may cause full 
reconciliation to take more round-trips until everything is as it should.
- the need to ensure the tag is set by the composition makes this function not self-contained. We aim to improve this in the future.
