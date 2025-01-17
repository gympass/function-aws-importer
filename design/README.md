# Design docs

This directory holds design documents for this project. It does not aim to be a comprehensive source of information, but
rather a place to document the reasoning behind some decisions and the design of some components.

## Why does this function exist?

Many cloud provider external resources are associated with a non-deterministic ID/external-name. To deal with that, Crossplane
providers rely on the `crossplane.io/external-name` annotation for discovering existing external resources that a managed
resource controls.

It's not unusual for Crossplane users to set `deletionPolicy` to `Orphan` on production environments to prevent accidental
deletions of external-resources in the event of human error or catastrophic failure in Kubernetes clusters requiring re-creation
of all resources.

During such events, the MR gets re-created without the annotation, and one of these scenarios follow:

1. The MR breaks and never syncs, because it identifies the resource you're attempting to manage already exists, and it 
does not get imported automatically. Human intervention is required.
2. The MR results in the creation of a different resource, effectively duplicating everything, and the provider is now 
managing a resource which might not even be at use. In this scenario, the failure to import the existing resource is not
reported, and it's marked as healthy (even though the original resource is now orphaned and unmanaged). Not only requires
manual intervention, but might go unnoticed until other things start to break.

These problems are aggravated when compositions are used, as it makes it all more complex. For example, imagine a composition that manages DNS hosted zones. As well as creating the zone itself, it might also create NS records on the parent zone, to delegate authority to it.

Within the dropdown you'll find a concrete example of a real scenario.

<details>
  <summary>Detailed Example</summary>

To make this example more concrete, let's use AWS' Route53. Assume the zone already exists in AWS, previously created by upjet-aws-provider.

  ```mermaid
  flowchart TD
   subgraph subgraph_aws["AWS"]
          hosted_zone(["Hosted Zone zone1 (foo.bar.baz)"])
          ns_record(["NS Record (on zone bar.baz, points to zone1)"])
    end
      mr_zone["Zone Managed Resource (external-name zone1)"]
      mr_record["Record Managed Resource"]
      xr["XR"] --> mr_zone & mr_record
      mr_zone --> hosted_zone
      mr_record --> ns_record
  
  ```

However, for one reason or another, the claim/XR/MRs that manage it had to be re-created. The resources were left in AWS. So far no disruption to DNS resolution is happening.

  ```mermaid
  flowchart TD
   subgraph subgraph_aws["AWS"]
          hosted_zone(["Hosted Zone zone1 (foo.bar.baz)"])
          ns_record(["NS Record (on zone bar.baz, points to zone1)"])
    end
  ```

Once the claim, XR and MRs are recreated, a duplicate DNS zone will be created, and it will be empty.

  ```mermaid
  flowchart TD
   subgraph subgraph_aws["AWS"]
          hosted_zone(["Hosted Zone zone1 (foo.bar.baz)"])
          duplicated_zone(["Hosted Zone zone2 (empty) (foo.bar.baz)"])
          ns_record(["NS Record (on zone bar.baz, points to zone1)"])
    end
      mr_zone["Zone Managed Resource (external-name zone2)"]
      mr_record["Record Managed Resource"]
      xr["XR"] --> mr_zone & mr_record
      mr_zone --> duplicated_zone
      mr_record --> ns_record
  
  ```


Because it throws no errors to halt the composition (as far as Crossplane is concerned, everything is as it should be), the NS resource also gets updated on the parent DNS zone, delegating authority to the new, empty DNS zone.

  ```mermaid
  flowchart TD
   subgraph subgraph_aws["AWS"]
          hosted_zone(["Hosted Zone zone1 (foo.bar.baz)"])
          duplicated_zone(["Hosted Zone zone2 (empty) (foo.bar.baz)"])
          ns_record(["NS Record (on zone bar.baz, points to zone2)"])
    end
      mr_zone["Zone Managed Resource (external-name zone2)"]
      mr_record["Record Managed Resource"]
      xr["XR"] --> mr_zone & mr_record
      mr_zone --> duplicated_zone
      mr_record --> ns_record
  
  ```

From this point forward, all DNS resolution for the original zone start failing, until an engineer updates the external-name annotation on the hosted zone MR to point to the old zone.

</details>
