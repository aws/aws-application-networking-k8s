# Update the AWS Gateway API Controller from v1.0.x to v1.1.y

Release `v1.1.0` of the AWS Gateway API Controller is built against `v1.2` of the Gateway API spec, but the controller is also compatible with the `v1.1` Gateway API. It is _not_ compatible the `v1.0` Gateway API.

Previous `v1.0.x` builds of the controller were built against `v1.0` of the Gateway API spec. This guide outlines the controller upgrade process from `v1.0.x` to `v1.1.y`. Please note that users are required to install Gateway API CRDs themselves as these are no longer bundled as of release `v1.1.0`. The latest Gateway API CRDs are available [here](https://gateway-api.sigs.k8s.io/). Please [follow this installation](https://gateway-api.sigs.k8s.io/guides/#installing-gateway-api) process.

# Basic Upgrade Process

1. Back up configuration, in particular GRPCRoute objects
2. Disable `v1.0.x` controller (e.g. scale to zero)
3. Update Gateway API CRDs to `v1.1.0`, available [here](https://gateway-api.sigs.k8s.io/)
4. Deploy and launch `v1.1.y` controller version

With the basic upgrade process, previously created GRPCRoutes on `v1alpha2` will automatically update to `v1` once they are reconciled by the controller. Alternatively, you can manually update your GRPCRoute versions (for example export to YAML, update version number, and apply updates). Creation of new GRPCRoutes objects using `v1alpha2` will be rejected.

# Upgrading to Gateway API `v1.2`

Moving to GatewayAPI `v1.2` can require an additional step as `v1alpha2` GRPCRoute objects have been removed. If GRPCRoute objects are not already on `v1`, you will need to follow steps outlined in the `v1.2.0` [release notes](https://github.com/kubernetes-sigs/gateway-api/releases/tag/v1.2.0).

1. Back up configuration, in particular GRPCRoute objects
2. Disable `v1.0.x` controller (e.g. scale to zero)
3. Update Gateway API CRDs to `v1.1.0`, available [here](https://gateway-api.sigs.k8s.io/)
4. Deploy and launch `v1.1.Y` controller version
5. Take upgrade steps outlined in `v1.2.0` [Gateway API release notes](https://github.com/kubernetes-sigs/gateway-api/releases/tag/v1.2.0)
6. Update Gateway API CRDs to `v1.2.0`