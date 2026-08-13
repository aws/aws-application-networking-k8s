# Multi-Environment Access with Service Network Endpoints

This guide explains how to reach VPC Lattice services from multiple environments (for example Production
and Staging) when the EKS clusters share the same VPC.

## The constraint

A VPC can have only **one** service network VPC association. The first environment that associates works
normally. The second attempt fails:

```text
ConflictException: VPC has already been associated to a service network.
```

A **service network VPC endpoint** (SNE) solves this. It provisions ENIs in your VPC subnets and gives
each service in the associated service network its own endpoint DNS name, allowing you to reach every service
in the associated service network without consuming the VPC's single association slot.

!!! note
    Setting the Helm value `defaultServiceNetwork` causes the controller to create the service network
    (if it does not already exist) and associate the VPC to it at startup. If the VPC is already
    associated to another service network, the association attempt fails with a 409. The controller logs
    the error and continues — routes still reconcile and create Lattice resources, but workloads in the
    VPC cannot reach services on that service network because the VPC association is missing.
    **Do not** set `defaultServiceNetwork` on the second environment's controller install.

## Prerequisites

- Two EKS clusters in the same VPC (e.g. one per environment)
- Each cluster has the VPC Lattice Controller installed and exposes its services through its own VPC Lattice Service
  Network ([Controller Installation guide](deploy.md))
- The VPC has `enableDnsSupport` and `enableDnsHostnames` enabled
- The endpoint's security groups allow client traffic on the service ports
- Any VPC Lattice IAM auth policy on the service network or service permits the caller
- For the ACK manifests shown below:
  the [ACK EC2 controller](https://aws-controllers-k8s.github.io/community/docs/community/services/#amazon-elastic-compute-cloud-ec2)
  and/or [ACK Route 53 controller](https://aws-controllers-k8s.github.io/community/docs/community/services/#amazon-route-53)
  installed in your cluster (the AWS CLI alternatives do not require these)

## Controller setup for the second environment

The first environment uses the standard install with `defaultServiceNetwork` as described in the
[Getting Started guide](getstarted.md). For the second environment:

1. Before installing the controller, create the service network **out-of-band**:

    ```bash
    aws vpc-lattice create-service-network --name <second-service-network-name>
    ```

2. Install the controller **without** `defaultServiceNetwork`:

    ```bash
    helm install gateway-api-controller \
        oci://public.ecr.aws/aws-application-networking-k8s/aws-gateway-controller-chart \
        --version=v2.1.2 \
        --set=serviceAccount.create=false \
        --namespace aws-application-networking-system \
        --set=log.level=info
    ```

3. Create a Gateway whose name **matches the service network name**. The controller uses the Gateway
   name to look up the service network in Lattice:

    ```yaml
    apiVersion: gateway.networking.k8s.io/v1
    kind: Gateway
    metadata:
      name: <second-service-network-name>  # Must match
    spec:
      gatewayClassName: amazon-vpc-lattice
      listeners:
      - name: http
        protocol: HTTP
        port: 80
    ```

4. Deploy HTTPRoutes referencing this Gateway as a `parentRef`, just like you would in the first
   environment. The controller will create the Lattice services and associate them with the second
   service network. The service FQDN appears in the route annotation
   `application-networking.k8s.aws/lattice-assigned-domain-name` once reconciliation completes.

## Creating the Service Network Endpoint

Create the SNE using the AWS CLI or the ACK EC2 controller.

=== "AWS CLI"

    ```bash
    aws ec2 create-vpc-endpoint \
      --vpc-endpoint-type ServiceNetwork \
      --vpc-id vpc-xxxxxxxx \
      --service-network-arn arn:aws:vpc-lattice:<region>:<account>:servicenetwork/sn-xxxxxxxx \
      --subnet-ids subnet-aaaa subnet-bbbb \
      --security-group-ids sg-xxxxxxxx
    ```

=== "ACK EC2 Controller"

    ```yaml
    apiVersion: ec2.services.k8s.aws/v1alpha1
    kind: VPCEndpoint
    metadata:
      name: staging-sne
    spec:
      vpcID: vpc-xxxxxxxx
      vpcEndpointType: ServiceNetwork
      serviceNetworkARN: arn:aws:vpc-lattice:<region>:<account>:servicenetwork/sn-xxxxxxxx
      subnetIDs:
        - subnet-aaaa
        - subnet-bbbb
      securityGroupIDs:
        - sg-xxxxxxxx
    ```

!!! note
    The Gateway API Controller does **not** create or manage the SNE. You provision it separately.

## Addressing services through the endpoint

Once the SNE exists, there are three ways to address services behind it.

### Option 1: Call the endpoint DNS name directly

Retrieve the endpoint DNS name and point callers at it:

```bash
aws ec2 describe-vpc-endpoint-associations \
  --vpc-endpoint-ids <vpce-id> \
  --query 'VpcEndpointAssociations[].DnsEntry.DnsName'
```

The name has the form `vpce-<endpoint-id>-snsa-<assoc-id>.<hash>.vpc-lattice-svcs.<region>.on.aws`.
Inject it into your workloads via environment variables or ConfigMaps.

**Trade-off**: the name is endpoint-specific and changes if the endpoint is recreated.

### Option 2: Use the standard service DNS name (Private Hosted Zone)

Use this when applications already call the standard Lattice service DNS name and you want **zero
application change**.

Create a Private Hosted Zone (PHZ) to override the standard name's resolution so it points to the
SNE instead of the VPC-associated environment.

The service FQDN is the value of the `application-networking.k8s.aws/lattice-assigned-domain-name`
annotation on your HTTPRoute (for example,
`myservice-default-abc123.7d67968.vpc-lattice-svcs.us-west-2.on.aws`).

1. Get the endpoint DNS name and its hosted zone ID:

    ```bash
    aws ec2 describe-vpc-endpoint-associations \
      --vpc-endpoint-ids <vpce-id> \
      --query 'VpcEndpointAssociations[].{name:DnsEntry.DnsName,zone:DnsEntry.HostedZoneId}'
    ```

2. Create a PHZ named the **exact** standard service FQDN, associated to the VPC.

3. In the PHZ, create an alias A record (and AAAA if using IPv6) at the zone apex pointing to the
   endpoint DNS name. Use the hosted zone ID from step 1 as the alias target zone.

=== "AWS CLI"

    ```bash
    # Create PHZ (--vpc implies private zone)
    aws route53 create-hosted-zone \
      --name <service-name>-svc-<id>.<hash>.vpc-lattice-svcs.<region>.on.aws \
      --caller-reference <unique-string> \
      --hosted-zone-config Comment="lattice sne override" \
      --vpc VPCRegion=<region>,VPCId=<vpc-id>

    # Create alias A record
    aws route53 change-resource-record-sets \
      --hosted-zone-id <phz-id> \
      --change-batch '{
        "Changes": [{
          "Action": "UPSERT",
          "ResourceRecordSet": {
            "Name": "<service-fqdn>",
            "Type": "A",
            "AliasTarget": {
              "HostedZoneId": "<endpoint-dns-hosted-zone-id>",
              "DNSName": "<endpoint-dns-name>",
              "EvaluateTargetHealth": false
            }
          }
        }]
      }'
    ```

=== "ACK Route 53 Controller"

    ```yaml
    apiVersion: route53.services.k8s.aws/v1alpha1
    kind: HostedZone
    metadata:
      name: staging-svc-phz
    spec:
      name: <service-fqdn>
      hostedZoneConfig:
        privateZone: true
      vpc:
        vpcID: vpc-xxxxxxxx
        vpcRegion: <region>
    ---
    apiVersion: route53.services.k8s.aws/v1alpha1
    kind: RecordSet
    metadata:
      name: staging-svc-apex-alias
    spec:
      hostedZoneRef:
        from:
          name: staging-svc-phz
      name: <service-fqdn>
      recordType: A
      aliasTarget:
        dnsName: <endpoint-dns-name>
        hostedZoneID: <endpoint-dns-hosted-zone-id>
        evaluateTargetHealth: false
    ```

!!! warning
    The PHZ overrides DNS for that FQDN in the **entire VPC**, including the first environment's
    workloads. Only use this for services that exist exclusively in the second environment. You also
    need one PHZ per service — if you have many, consider Option 3 instead.

**Trade-off**: if the endpoint is recreated, you update the alias record target. An alias is required
here because the record sits at a zone apex where CNAMEs are not allowed.

### Option 3: Use a custom domain name

Use this when you want a friendly, stable name you own (for example `claims-api.internal`).

1. Set the custom domain on the HTTPRoute `spec.hostnames` field **at creation time**.
   See the [Custom Domain Name](custom-domain-name.md) guide.

    !!! warning
        Custom domain names cannot be changed after service creation. A service without a custom domain
        must be recreated to add one.

2. Create a CNAME in a zone you own pointing to the endpoint DNS name:

    ```text
    claims-api.internal  CNAME  vpce-<endpoint-id>-snsa-<assoc-id>.<hash>.vpc-lattice-svcs.<region>.on.aws
    ```

3. For HTTPS, attach an ACM certificate covering the custom domain to the service. The Lattice-managed
   `*.vpc-lattice-svcs.<region>.on.aws` certificate does **not** cover custom domains. If an issued ACM
   certificate matching the hostname exists in your account, the controller discovers and attaches it
   automatically — see [Automatic Certificate Discovery](https.md#automatic-certificate-discovery).

**Trade-off**: if the endpoint is recreated, you update the CNAME target.

## Verifying connectivity

Once you have deployed an HTTPRoute on the second environment and chosen an addressing option above,
verify from a pod in the VPC:

```bash
# Using Option 1 (endpoint DNS name) or Option 2/3 (your chosen name)
curl http://<target-name>/
```

If the request fails, check:

- The SNE security group allows inbound traffic on the service port from the caller's CIDR.
- The VPC has `enableDnsSupport` and `enableDnsHostnames` enabled.
- For Option 2, the PHZ is associated to the correct VPC and the alias record target matches the
  endpoint DNS name.

## Summary

| Approach                      | Application change       | DNS setup               | Survives endpoint recreation |
|-------------------------------|--------------------------|-------------------------|------------------------------|
| Option 1: Endpoint DNS name   | Inject new target        | None                    | No (name changes)            |
| Option 2: Private Hosted Zone | None                     | PHZ + alias per service | Update alias target          |
| Option 3: Custom domain       | Set hostname at creation | CNAME per service       | Update CNAME target          |
