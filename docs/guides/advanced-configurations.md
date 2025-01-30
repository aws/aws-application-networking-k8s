# Advanced configurations

The section below covers advanced configuration techniques for installing and using the AWS Gateway API Controller. This includes things such as running the controller on a self-hosted cluster on AWS or using an IPv6 EKS cluster.

### Using a self-managed Kubernetes cluster

You can install AWS Gateway API Controller to a self-managed Kubernetes cluster in AWS.

However, the controller utilizes [IMDS](https://docs.aws.amazon.com/AWSEC2/latest/UserGuide/ec2-instance-metadata.html) to get necessary information from instance metadata, such as AWS account ID and VPC ID. So:

- **If your cluster is using IMDSv2.** ensure the hop limit is 2 or higher to allow the access from the controller:

    ```bash
    aws ec2 modify-instance-metadata-options --http-put-response-hop-limit 2 --region <region> --instance-id <instance-id>
    ```

- **If your cluster cannot access to IMDS.** ensure to specify the[configuration variables](environment.md) when installing the controller.

### IPv6 support

IPv6 address type is automatically used for your services and pods if
[your cluster is configured to use IPv6 addresses](https://docs.aws.amazon.com/eks/latest/userguide/cni-ipv6.html).

If your cluster is configured to be dual-stack, you can set the IP address type
of your service using the `ipFamilies` field. For example:

```yaml title="parking_service.yaml"
apiVersion: v1
kind: Service
metadata:
  name: ipv4-target-in-dual-stack-cluster
spec:
  ipFamilies:
    - "IPv4"
  selector:
    app: parking
  ports:
    - protocol: TCP
      port: 80
      targetPort: 8090
```
