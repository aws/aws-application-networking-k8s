# AWS Gateway API Controller for VPC Lattice

<p align="center">
    <img src="docs/images/kubernetes_icon.svg" alt="Kubernetes logo" width="100" /> 
    <img src="docs/images/controller.png" alt="AWS Load Balancer logo" width="100" />
</p>

AWS Application Networking is an implementation of the Kubernetes [Gateway API](https://gateway-api.sigs.k8s.io/). This project is designed to run in a Kubernetes cluster and orchestrates AWS VPC Lattice resources using Kubernetes Custom Resource Definitions like Gateway and HTTPRoute.

## Documentation

### Website

The API specification and detailed documentation is available on the project
website: [https://www.gateway-api-controller.eks.aws.dev/][ghp].

### Concepts

To get started, please read through [API concepts][concepts]. These documents give the necessary background to understand the API and the use-cases it targets.

### Getting started

Once you have a good understanding of the API at a higher-level, check out
[getting started][getting-started] to install your first Gateway controller and try out
one of the guides.

### References

A complete API reference, please refer to:

- [API reference][spec]
- [Go docs for the package][godoc]

## Contributing

Developer guide can be found on the [developer guide page][dev].
Our Kubernetes Slack channel is [#aws-gateway-api-controller][slack].

### Code of conduct

Participation in the Kubernetes community is governed by the
[Kubernetes Code of Conduct](code-of-conduct.md).

## Security

See [CONTRIBUTING](CONTRIBUTING.md#security-issue-notifications) for more information.

## License

This project is licensed under the Apache-2.0 License.

[ghp]: https://www.gateway-api-controller.eks.aws.dev/
[dev]: https://www.gateway-api-controller.eks.aws.dev/contributing/developer/
[slack]: https://kubernetes.slack.com/messages/aws-gateway-api-controller
[getting-started]: https://www.gateway-api-controller.eks.aws.dev/guides/getstarted/
[spec]: https://www.gateway-api-controller.eks.aws.dev/api-reference/
[concepts]: https://www.gateway-api-controller.eks.aws.dev/concepts/
[gh_release]: https://github.com/aws/aws-application-networking-k8s/releases/tag/v1.0.6
[godoc]: https://www.gateway-api-controller.eks.aws.dev/
