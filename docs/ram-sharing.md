# Share Kubernetes Gateway (VPC lattice service network) between different AWS accounts

AWS Resource Access Manager(RAM) helps you share your resources across AWS accounts, within your organization or
organizational units (OUs). Now VPC lattice support 2 types of resource sharing: share VPC lattice service or sharing
VPC lattice services, in the AWS Gateway API Controller now it only support sharing VPC lattice service network.

Here in a example that account B (sharer account) could share it's service network with account A (sharee account), and
account A could access all k8s services (vpc lattice target groups) and k8s httproutes(vpc lattice services) within this
sharer account's service network.

**Steps**

1. Create a full connectivity setup example (include a gateway, a service and a httproute ) in the account B (sharer
   account): `kubectl apply -f examples/second-account-gw1-full-setup.yaml`


2. Go to accountB's aws "Resource Access Manager" console, create a `VPC Lattice Service Networks` type resource
   sharing, share the service network that created from previous step's gateway(second-account-gw1)  (You could check
   the VPC Lattice console to get the resource arn, service network name should also be second-account-gw1)

3. Open the account A (sharee account)'s "aws Resource Access Manager" console, in the "Shared with me" section accept
   the accountB's Service Network sharing invitation.

4. Load the account A's aws credential in you command line, do `kubectl config use-context <accountA cluster>` to switch
   to accountA's context

5. Apply the same "second-account-gw1" account A (sharee account)'s cluster
   by `kubectl apply -f examples/second-account-gw1-in-primary-account.yaml`

6. All done, you could verify service network(gateway) sharing by: Attach to any pod in account A's cluster,
   do `curl <vpc lattice service dns for 'second-account-gw1-httproute'>`, it should be able to get correct response "
   second-account-gw1-svc handler pod" 
