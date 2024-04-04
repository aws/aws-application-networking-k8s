# API Specification

This page contains the API field specification for Gateway API.

<p>Packages:</p>
<ul>
<li>
<a href="#application-networking.k8s.aws%2fv1alpha1">application-networking.k8s.aws/v1alpha1</a>
</li>
</ul>
<h2 id="application-networking.k8s.aws/v1alpha1">application-networking.k8s.aws/v1alpha1</h2>
<div>
</div>
Resource Types:
<ul><li>
<a href="#application-networking.k8s.aws/v1alpha1.AccessLogPolicy">AccessLogPolicy</a>
</li><li>
<a href="#application-networking.k8s.aws/v1alpha1.IAMAuthPolicy">IAMAuthPolicy</a>
</li><li>
<a href="#application-networking.k8s.aws/v1alpha1.ServiceExport">ServiceExport</a>
</li><li>
<a href="#application-networking.k8s.aws/v1alpha1.ServiceImport">ServiceImport</a>
</li><li>
<a href="#application-networking.k8s.aws/v1alpha1.TargetGroupPolicy">TargetGroupPolicy</a>
</li><li>
<a href="#application-networking.k8s.aws/v1alpha1.VpcAssociationPolicy">VpcAssociationPolicy</a>
</li></ul>
<h3 id="application-networking.k8s.aws/v1alpha1.AccessLogPolicy">AccessLogPolicy
</h3>
<div>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>apiVersion</code><br/>
string</td>
<td>
<code>
application-networking.k8s.aws/v1alpha1
</code>
</td>
</tr>
<tr>
<td>
<code>kind</code><br/>
string
</td>
<td><code>AccessLogPolicy</code></td>
</tr>
<tr>
<td>
<code>metadata</code><br/>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.24/#objectmeta-v1-meta">
Kubernetes meta/v1.ObjectMeta
</a>
</em>
</td>
<td>
Refer to the Kubernetes API documentation for the fields of the
<code>metadata</code> field.
</td>
</tr>
<tr>
<td>
<code>spec</code><br/>
<em>
<a href="#application-networking.k8s.aws/v1alpha1.AccessLogPolicySpec">
AccessLogPolicySpec
</a>
</em>
</td>
<td>
<br/>
<br/>
<table>
<tr>
<td>
<code>destinationArn</code><br/>
<em>
string
</em>
</td>
<td>
<p>The Amazon Resource Name (ARN) of the destination that will store access logs.
Supported values are S3 Bucket, CloudWatch Log Group, and Firehose Delivery Stream ARNs.</p>
<p>Changes to this value results in replacement of the VPC Lattice Access Log Subscription.</p>
</td>
</tr>
<tr>
<td>
<code>targetRef</code><br/>
<em>
<a href="https://gateway-api.sigs.k8s.io/geps/gep-713/?h=policytargetreference#policy-targetref-api">
sigs.k8s.io/gateway-api/apis/v1alpha2.PolicyTargetReference
</a>
</em>
</td>
<td>
<p>TargetRef points to the Kubernetes Gateway, HTTPRoute, or GRPCRoute resource that will have this policy attached.</p>
<p>This field is following the guidelines of Kubernetes Gateway API policy attachment.</p>
</td>
</tr>
</table>
</td>
</tr>
<tr>
<td>
<code>status</code><br/>
<em>
<a href="#application-networking.k8s.aws/v1alpha1.AccessLogPolicyStatus">
AccessLogPolicyStatus
</a>
</em>
</td>
<td>
<p>Status defines the current state of AccessLogPolicy.</p>
</td>
</tr>
</tbody>
</table>
<h3 id="application-networking.k8s.aws/v1alpha1.IAMAuthPolicy">IAMAuthPolicy
</h3>
<div>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>apiVersion</code><br/>
string</td>
<td>
<code>
application-networking.k8s.aws/v1alpha1
</code>
</td>
</tr>
<tr>
<td>
<code>kind</code><br/>
string
</td>
<td><code>IAMAuthPolicy</code></td>
</tr>
<tr>
<td>
<code>metadata</code><br/>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.24/#objectmeta-v1-meta">
Kubernetes meta/v1.ObjectMeta
</a>
</em>
</td>
<td>
Refer to the Kubernetes API documentation for the fields of the
<code>metadata</code> field.
</td>
</tr>
<tr>
<td>
<code>spec</code><br/>
<em>
<a href="#application-networking.k8s.aws/v1alpha1.IAMAuthPolicySpec">
IAMAuthPolicySpec
</a>
</em>
</td>
<td>
<br/>
<br/>
<table>
<tr>
<td>
<code>policy</code><br/>
<em>
string
</em>
</td>
<td>
<p>IAM auth policy content. It is a JSON string that uses the same syntax as AWS IAM policies. Please check the VPC Lattice documentation to get <a href="https://docs.aws.amazon.com/vpc-lattice/latest/ug/auth-policies.html#auth-policies-common-elements">the common elements in an auth policy</a></p>
</td>
</tr>
<tr>
<td>
<code>targetRef</code><br/>
<em>
<a href="https://gateway-api.sigs.k8s.io/geps/gep-713/?h=policytargetreference#policy-targetref-api">
sigs.k8s.io/gateway-api/apis/v1alpha2.PolicyTargetReference
</a>
</em>
</td>
<td>
<p>TargetRef points to the Kubernetes Gateway, HTTPRoute, or GRPCRoute resource that will have this policy attached.</p>
<p>This field is following the guidelines of Kubernetes Gateway API policy attachment.</p>
</td>
</tr>
</table>
</td>
</tr>
<tr>
<td>
<code>status</code><br/>
<em>
<a href="#application-networking.k8s.aws/v1alpha1.IAMAuthPolicyStatus">
IAMAuthPolicyStatus
</a>
</em>
</td>
<td>
<p>Status defines the current state of IAMAuthPolicy.</p>
</td>
</tr>
</tbody>
</table>
<h3 id="application-networking.k8s.aws/v1alpha1.ServiceExport">ServiceExport
</h3>
<div>
<p>ServiceExport declares that the Service with the same name and namespace
as this export should be consumable from other clusters.</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>apiVersion</code><br/>
string</td>
<td>
<code>
application-networking.k8s.aws/v1alpha1
</code>
</td>
</tr>
<tr>
<td>
<code>kind</code><br/>
string
</td>
<td><code>ServiceExport</code></td>
</tr>
<tr>
<td>
<code>metadata</code><br/>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.24/#objectmeta-v1-meta">
Kubernetes meta/v1.ObjectMeta
</a>
</em>
</td>
<td>
<em>(Optional)</em>
Refer to the Kubernetes API documentation for the fields of the
<code>metadata</code> field.
</td>
</tr>
<tr>
<td>
<code>status</code><br/>
<em>
<a href="#application-networking.k8s.aws/v1alpha1.ServiceExportStatus">
ServiceExportStatus
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>status describes the current state of an exported service.
Service configuration comes from the Service that had the same
name and namespace as this ServiceExport.
Populated by the multi-cluster service implementation&rsquo;s controller.</p>
</td>
</tr>
</tbody>
</table>
<h3 id="application-networking.k8s.aws/v1alpha1.ServiceImport">ServiceImport
</h3>
<div>
<p>ServiceImport describes a service imported from clusters in a ClusterSet.</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>apiVersion</code><br/>
string</td>
<td>
<code>
application-networking.k8s.aws/v1alpha1
</code>
</td>
</tr>
<tr>
<td>
<code>kind</code><br/>
string
</td>
<td><code>ServiceImport</code></td>
</tr>
<tr>
<td>
<code>metadata</code><br/>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.24/#objectmeta-v1-meta">
Kubernetes meta/v1.ObjectMeta
</a>
</em>
</td>
<td>
<em>(Optional)</em>
Refer to the Kubernetes API documentation for the fields of the
<code>metadata</code> field.
</td>
</tr>
<tr>
<td>
<code>spec</code><br/>
<em>
<a href="#application-networking.k8s.aws/v1alpha1.ServiceImportSpec">
ServiceImportSpec
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>spec defines the behavior of a ServiceImport.</p>
<br/>
<br/>
<table>
<tr>
<td>
<code>ports</code><br/>
<em>
<a href="#application-networking.k8s.aws/v1alpha1.ServicePort">
[]ServicePort
</a>
</em>
</td>
<td>
</td>
</tr>
<tr>
<td>
<code>ips</code><br/>
<em>
[]string
</em>
</td>
<td>
<em>(Optional)</em>
<p>ip will be used as the VIP for this service when type is ClusterSetIP.</p>
</td>
</tr>
<tr>
<td>
<code>type</code><br/>
<em>
<a href="#application-networking.k8s.aws/v1alpha1.ServiceImportType">
ServiceImportType
</a>
</em>
</td>
<td>
<p>type defines the type of this service.
Must be ClusterSetIP or Headless.</p>
</td>
</tr>
<tr>
<td>
<code>sessionAffinity</code><br/>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.24/#serviceaffinity-v1-core">
Kubernetes core/v1.ServiceAffinity
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>Supports &ldquo;ClientIP&rdquo; and &ldquo;None&rdquo;. Used to maintain session affinity.
Enable client IP based session affinity.
Must be ClientIP or None.
Defaults to None.
Ignored when type is Headless
More info: <a href="https://kubernetes.io/docs/concepts/services-networking/service/#virtual-ips-and-service-proxies">https://kubernetes.io/docs/concepts/services-networking/service/#virtual-ips-and-service-proxies</a></p>
</td>
</tr>
<tr>
<td>
<code>sessionAffinityConfig</code><br/>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.24/#sessionaffinityconfig-v1-core">
Kubernetes core/v1.SessionAffinityConfig
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>sessionAffinityConfig contains session affinity configuration.</p>
</td>
</tr>
</table>
</td>
</tr>
<tr>
<td>
<code>status</code><br/>
<em>
<a href="#application-networking.k8s.aws/v1alpha1.ServiceImportStatus">
ServiceImportStatus
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>status contains information about the exported services that form
the multi-cluster service referenced by this ServiceImport.</p>
</td>
</tr>
</tbody>
</table>
<h3 id="application-networking.k8s.aws/v1alpha1.TargetGroupPolicy">TargetGroupPolicy
</h3>
<div>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>apiVersion</code><br/>
string</td>
<td>
<code>
application-networking.k8s.aws/v1alpha1
</code>
</td>
</tr>
<tr>
<td>
<code>kind</code><br/>
string
</td>
<td><code>TargetGroupPolicy</code></td>
</tr>
<tr>
<td>
<code>metadata</code><br/>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.24/#objectmeta-v1-meta">
Kubernetes meta/v1.ObjectMeta
</a>
</em>
</td>
<td>
Refer to the Kubernetes API documentation for the fields of the
<code>metadata</code> field.
</td>
</tr>
<tr>
<td>
<code>spec</code><br/>
<em>
<a href="#application-networking.k8s.aws/v1alpha1.TargetGroupPolicySpec">
TargetGroupPolicySpec
</a>
</em>
</td>
<td>
<br/>
<br/>
<table>
<tr>
<td>
<code>protocol</code><br/>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>The protocol to use for routing traffic to the targets. Supported values are HTTP (default) and HTTPS.</p>
<p>Changes to this value results in a replacement of VPC Lattice target group.</p>
</td>
</tr>
<tr>
<td>
<code>protocolVersion</code><br/>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>The protocol version to use. Supported values are HTTP1 (default) and HTTP2. When a policy is behind GRPCRoute,
this field value will be ignored as GRPC is only supported through HTTP/2.</p>
<p>Changes to this value results in a replacement of VPC Lattice target group.</p>
</td>
</tr>
<tr>
<td>
<code>targetRef</code><br/>
<em>
<a href="https://gateway-api.sigs.k8s.io/geps/gep-713/?h=policytargetreference#policy-targetref-api">
sigs.k8s.io/gateway-api/apis/v1alpha2.PolicyTargetReference
</a>
</em>
</td>
<td>
<p>TargetRef points to the kubernetes Service resource that will have this policy attached.</p>
<p>This field is following the guidelines of Kubernetes Gateway API policy attachment.</p>
</td>
</tr>
<tr>
<td>
<code>healthCheck</code><br/>
<em>
<a href="#application-networking.k8s.aws/v1alpha1.HealthCheckConfig">
HealthCheckConfig
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>The health check configuration.</p>
<p>Changes to this value will update VPC Lattice resource in place.</p>
</td>
</tr>
</table>
</td>
</tr>
<tr>
<td>
<code>status</code><br/>
<em>
<a href="#application-networking.k8s.aws/v1alpha1.TargetGroupPolicyStatus">
TargetGroupPolicyStatus
</a>
</em>
</td>
<td>
</td>
</tr>
</tbody>
</table>
<h3 id="application-networking.k8s.aws/v1alpha1.VpcAssociationPolicy">VpcAssociationPolicy
</h3>
<div>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>apiVersion</code><br/>
string</td>
<td>
<code>
application-networking.k8s.aws/v1alpha1
</code>
</td>
</tr>
<tr>
<td>
<code>kind</code><br/>
string
</td>
<td><code>VpcAssociationPolicy</code></td>
</tr>
<tr>
<td>
<code>metadata</code><br/>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.24/#objectmeta-v1-meta">
Kubernetes meta/v1.ObjectMeta
</a>
</em>
</td>
<td>
Refer to the Kubernetes API documentation for the fields of the
<code>metadata</code> field.
</td>
</tr>
<tr>
<td>
<code>spec</code><br/>
<em>
<a href="#application-networking.k8s.aws/v1alpha1.VpcAssociationPolicySpec">
VpcAssociationPolicySpec
</a>
</em>
</td>
<td>
<br/>
<br/>
<table>
<tr>
<td>
<code>securityGroupIds</code><br/>
<em>
<a href="#application-networking.k8s.aws/v1alpha1.SecurityGroupId">
[]SecurityGroupId
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>SecurityGroupIds defines the security groups enforced on the VpcServiceNetworkAssociation.
Security groups does not take effect if AssociateWithVpc is set to false.</p>
<p>For more details, please check the VPC Lattice documentation <a href="https://docs.aws.amazon.com/vpc-lattice/latest/ug/security-groups.html">https://docs.aws.amazon.com/vpc-lattice/latest/ug/security-groups.html</a></p>
</td>
</tr>
<tr>
<td>
<code>associateWithVpc</code><br/>
<em>
bool
</em>
</td>
<td>
<em>(Optional)</em>
<p>AssociateWithVpc indicates whether the VpcServiceNetworkAssociation should be created for the current VPC of k8s cluster.</p>
<p>This value will be considered true by default.</p>
</td>
</tr>
<tr>
<td>
<code>targetRef</code><br/>
<em>
<a href="https://gateway-api.sigs.k8s.io/geps/gep-713/?h=policytargetreference#policy-targetref-api">
sigs.k8s.io/gateway-api/apis/v1alpha2.PolicyTargetReference
</a>
</em>
</td>
<td>
<p>TargetRef points to the kubernetes Gateway resource that will have this policy attached.</p>
<p>This field is following the guidelines of Kubernetes Gateway API policy attachment.</p>
</td>
</tr>
</table>
</td>
</tr>
<tr>
<td>
<code>status</code><br/>
<em>
<a href="#application-networking.k8s.aws/v1alpha1.VpcAssociationPolicyStatus">
VpcAssociationPolicyStatus
</a>
</em>
</td>
<td>
</td>
</tr>
</tbody>
</table>
<h3 id="application-networking.k8s.aws/v1alpha1.AccessLogPolicySpec">AccessLogPolicySpec
</h3>
<p>
(<em>Appears on:</em><a href="#application-networking.k8s.aws/v1alpha1.AccessLogPolicy">AccessLogPolicy</a>)
</p>
<div>
<p>AccessLogPolicySpec defines the desired state of AccessLogPolicy.</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>destinationArn</code><br/>
<em>
string
</em>
</td>
<td>
<p>The Amazon Resource Name (ARN) of the destination that will store access logs.
Supported values are S3 Bucket, CloudWatch Log Group, and Firehose Delivery Stream ARNs.</p>
<p>Changes to this value results in replacement of the VPC Lattice Access Log Subscription.</p>
</td>
</tr>
<tr>
<td>
<code>targetRef</code><br/>
<em>
<a href="https://gateway-api.sigs.k8s.io/geps/gep-713/?h=policytargetreference#policy-targetref-api">
sigs.k8s.io/gateway-api/apis/v1alpha2.PolicyTargetReference
</a>
</em>
</td>
<td>
<p>TargetRef points to the Kubernetes Gateway, HTTPRoute, or GRPCRoute resource that will have this policy attached.</p>
<p>This field is following the guidelines of Kubernetes Gateway API policy attachment.</p>
</td>
</tr>
</tbody>
</table>
<h3 id="application-networking.k8s.aws/v1alpha1.AccessLogPolicyStatus">AccessLogPolicyStatus
</h3>
<p>
(<em>Appears on:</em><a href="#application-networking.k8s.aws/v1alpha1.AccessLogPolicy">AccessLogPolicy</a>)
</p>
<div>
<p>AccessLogPolicyStatus defines the observed state of AccessLogPolicy.</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>conditions</code><br/>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.24/#condition-v1-meta">
[]Kubernetes meta/v1.Condition
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>Conditions describe the current conditions of the AccessLogPolicy.</p>
<p>Implementations should prefer to express Policy conditions
using the <code>PolicyConditionType</code> and <code>PolicyConditionReason</code>
constants so that operators and tools can converge on a common
vocabulary to describe AccessLogPolicy state.</p>
<p>Known condition types are:</p>
<ul>
<li>&ldquo;Accepted&rdquo;</li>
<li>&ldquo;Ready&rdquo;</li>
</ul>
</td>
</tr>
</tbody>
</table>
<h3 id="application-networking.k8s.aws/v1alpha1.ClusterStatus">ClusterStatus
</h3>
<p>
(<em>Appears on:</em><a href="#application-networking.k8s.aws/v1alpha1.ServiceImportStatus">ServiceImportStatus</a>)
</p>
<div>
<p>ClusterStatus contains service configuration mapped to a specific source cluster</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>cluster</code><br/>
<em>
string
</em>
</td>
<td>
<p>cluster is the name of the exporting cluster. Must be a valid RFC-1123 DNS
label.</p>
</td>
</tr>
</tbody>
</table>
<h3 id="application-networking.k8s.aws/v1alpha1.HealthCheckConfig">HealthCheckConfig
</h3>
<p>
(<em>Appears on:</em><a href="#application-networking.k8s.aws/v1alpha1.TargetGroupPolicySpec">TargetGroupPolicySpec</a>)
</p>
<div>
<p>HealthCheckConfig defines health check configuration for given VPC Lattice target group.
For the detailed explanation and supported values, please refer to VPC Lattice documentationon health checks.</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>enabled</code><br/>
<em>
bool
</em>
</td>
<td>
<em>(Optional)</em>
<p>Indicates whether health checking is enabled.</p>
</td>
</tr>
<tr>
<td>
<code>intervalSeconds</code><br/>
<em>
int64
</em>
</td>
<td>
<em>(Optional)</em>
<p>The approximate amount of time, in seconds, between health checks of an individual target.</p>
</td>
</tr>
<tr>
<td>
<code>timeoutSeconds</code><br/>
<em>
int64
</em>
</td>
<td>
<em>(Optional)</em>
<p>The amount of time, in seconds, to wait before reporting a target as unhealthy.</p>
</td>
</tr>
<tr>
<td>
<code>healthyThresholdCount</code><br/>
<em>
int64
</em>
</td>
<td>
<em>(Optional)</em>
<p>The number of consecutive successful health checks required before considering an unhealthy target healthy.</p>
</td>
</tr>
<tr>
<td>
<code>unhealthyThresholdCount</code><br/>
<em>
int64
</em>
</td>
<td>
<em>(Optional)</em>
<p>The number of consecutive failed health checks required before considering a target unhealthy.</p>
</td>
</tr>
<tr>
<td>
<code>statusMatch</code><br/>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>A regular expression to match HTTP status codes when checking for successful response from a target.</p>
</td>
</tr>
<tr>
<td>
<code>path</code><br/>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>The destination for health checks on the targets.</p>
</td>
</tr>
<tr>
<td>
<code>port</code><br/>
<em>
int64
</em>
</td>
<td>
<p>The port used when performing health checks on targets. If not specified, health check defaults to the
port that a target receives traffic on.</p>
</td>
</tr>
<tr>
<td>
<code>protocol</code><br/>
<em>
<a href="#application-networking.k8s.aws/v1alpha1.HealthCheckProtocol">
HealthCheckProtocol
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>The protocol used when performing health checks on targets.</p>
</td>
</tr>
<tr>
<td>
<code>protocolVersion</code><br/>
<em>
<a href="#application-networking.k8s.aws/v1alpha1.HealthCheckProtocolVersion">
HealthCheckProtocolVersion
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>The protocol version used when performing health checks on targets. Defaults to HTTP/1.</p>
</td>
</tr>
</tbody>
</table>
<h3 id="application-networking.k8s.aws/v1alpha1.HealthCheckProtocol">HealthCheckProtocol
(<code>string</code> alias)</h3>
<p>
(<em>Appears on:</em><a href="#application-networking.k8s.aws/v1alpha1.HealthCheckConfig">HealthCheckConfig</a>)
</p>
<div>
</div>
<table>
<thead>
<tr>
<th>Value</th>
<th>Description</th>
</tr>
</thead>
<tbody><tr><td><p>&#34;HTTP&#34;</p></td>
<td></td>
</tr><tr><td><p>&#34;HTTPS&#34;</p></td>
<td></td>
</tr></tbody>
</table>
<h3 id="application-networking.k8s.aws/v1alpha1.HealthCheckProtocolVersion">HealthCheckProtocolVersion
(<code>string</code> alias)</h3>
<p>
(<em>Appears on:</em><a href="#application-networking.k8s.aws/v1alpha1.HealthCheckConfig">HealthCheckConfig</a>)
</p>
<div>
</div>
<table>
<thead>
<tr>
<th>Value</th>
<th>Description</th>
</tr>
</thead>
<tbody><tr><td><p>&#34;HTTP1&#34;</p></td>
<td></td>
</tr><tr><td><p>&#34;HTTP2&#34;</p></td>
<td></td>
</tr></tbody>
</table>
<h3 id="application-networking.k8s.aws/v1alpha1.IAMAuthPolicySpec">IAMAuthPolicySpec
</h3>
<p>
(<em>Appears on:</em><a href="#application-networking.k8s.aws/v1alpha1.IAMAuthPolicy">IAMAuthPolicy</a>)
</p>
<div>
<p>IAMAuthPolicySpec defines the desired state of IAMAuthPolicy.
When the controller handles IAMAuthPolicy creation, if the targetRef k8s and VPC Lattice resource exists, the controller will change the auth_type of that VPC Lattice resource to AWS_IAM and attach this policy.
When the controller handles IAMAuthPolicy deletion, if the targetRef k8s and VPC Lattice resource exists, the controller will change the auth_type of that VPC Lattice resource to NONE and detach this policy.</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>policy</code><br/>
<em>
string
</em>
</td>
<td>
<p>IAM auth policy content. It is a JSON string that uses the same syntax as AWS IAM policies. Please check the VPC Lattice documentation to get <a href="https://docs.aws.amazon.com/vpc-lattice/latest/ug/auth-policies.html#auth-policies-common-elements">the common elements in an auth policy</a></p>
</td>
</tr>
<tr>
<td>
<code>targetRef</code><br/>
<em>
<a href="https://gateway-api.sigs.k8s.io/geps/gep-713/?h=policytargetreference#policy-targetref-api">
sigs.k8s.io/gateway-api/apis/v1alpha2.PolicyTargetReference
</a>
</em>
</td>
<td>
<p>TargetRef points to the Kubernetes Gateway, HTTPRoute, or GRPCRoute resource that will have this policy attached.</p>
<p>This field is following the guidelines of Kubernetes Gateway API policy attachment.</p>
</td>
</tr>
</tbody>
</table>
<h3 id="application-networking.k8s.aws/v1alpha1.IAMAuthPolicyStatus">IAMAuthPolicyStatus
</h3>
<p>
(<em>Appears on:</em><a href="#application-networking.k8s.aws/v1alpha1.IAMAuthPolicy">IAMAuthPolicy</a>)
</p>
<div>
<p>IAMAuthPolicyStatus defines the observed state of IAMAuthPolicy.</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>conditions</code><br/>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.24/#condition-v1-meta">
[]Kubernetes meta/v1.Condition
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>Conditions describe the current conditions of the IAMAuthPolicy.</p>
<p>Implementations should prefer to express Policy conditions
using the <code>PolicyConditionType</code> and <code>PolicyConditionReason</code>
constants so that operators and tools can converge on a common
vocabulary to describe IAMAuthPolicy state.</p>
<p>Known condition types are:</p>
<ul>
<li>&ldquo;Accepted&rdquo;</li>
<li>&ldquo;Ready&rdquo;</li>
</ul>
</td>
</tr>
</tbody>
</table>
<h3 id="application-networking.k8s.aws/v1alpha1.SecurityGroupId">SecurityGroupId
(<code>string</code> alias)</h3>
<p>
(<em>Appears on:</em><a href="#application-networking.k8s.aws/v1alpha1.VpcAssociationPolicySpec">VpcAssociationPolicySpec</a>)
</p>
<div>
</div>
<h3 id="application-networking.k8s.aws/v1alpha1.ServiceExportCondition">ServiceExportCondition
</h3>
<p>
(<em>Appears on:</em><a href="#application-networking.k8s.aws/v1alpha1.ServiceExportStatus">ServiceExportStatus</a>)
</p>
<div>
<p>ServiceExportCondition contains details for the current condition of this
service export.</p>
<p>Once <a href="https://github.com/kubernetes/enhancements/tree/master/keps/sig-api-machinery/1623-standardize-conditions">KEP-1623</a> is
implemented, this will be replaced by metav1.Condition.</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>type</code><br/>
<em>
<a href="#application-networking.k8s.aws/v1alpha1.ServiceExportConditionType">
ServiceExportConditionType
</a>
</em>
</td>
<td>
</td>
</tr>
<tr>
<td>
<code>status</code><br/>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.24/#conditionstatus-v1-core">
Kubernetes core/v1.ConditionStatus
</a>
</em>
</td>
<td>
<p>Status is one of {&ldquo;True&rdquo;, &ldquo;False&rdquo;, &ldquo;Unknown&rdquo;}</p>
</td>
</tr>
<tr>
<td>
<code>lastTransitionTime</code><br/>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.24/#time-v1-meta">
Kubernetes meta/v1.Time
</a>
</em>
</td>
<td>
<em>(Optional)</em>
</td>
</tr>
<tr>
<td>
<code>reason</code><br/>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
</td>
</tr>
<tr>
<td>
<code>message</code><br/>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
</td>
</tr>
</tbody>
</table>
<h3 id="application-networking.k8s.aws/v1alpha1.ServiceExportConditionType">ServiceExportConditionType
(<code>string</code> alias)</h3>
<p>
(<em>Appears on:</em><a href="#application-networking.k8s.aws/v1alpha1.ServiceExportCondition">ServiceExportCondition</a>)
</p>
<div>
<p>ServiceExportConditionType identifies a specific condition.</p>
</div>
<table>
<thead>
<tr>
<th>Value</th>
<th>Description</th>
</tr>
</thead>
<tbody><tr><td><p>&#34;Conflict&#34;</p></td>
<td><p>ServiceExportConflict means that there is a conflict between two
exports for the same Service. When &ldquo;True&rdquo;, the condition message
should contain enough information to diagnose the conflict:
field(s) under contention, which cluster won, and why.
Users should not expect detailed per-cluster information in the
conflict message.</p>
</td>
</tr><tr><td><p>&#34;Valid&#34;</p></td>
<td><p>ServiceExportValid means that the service referenced by this
service export has been recognized as valid by a controller.
This will be false if the service is found to be unexportable
(ExternalName, not found).</p>
</td>
</tr></tbody>
</table>
<h3 id="application-networking.k8s.aws/v1alpha1.ServiceExportStatus">ServiceExportStatus
</h3>
<p>
(<em>Appears on:</em><a href="#application-networking.k8s.aws/v1alpha1.ServiceExport">ServiceExport</a>)
</p>
<div>
<p>ServiceExportStatus contains the current status of an export.</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>conditions</code><br/>
<em>
<a href="#application-networking.k8s.aws/v1alpha1.ServiceExportCondition">
[]ServiceExportCondition
</a>
</em>
</td>
<td>
<em>(Optional)</em>
</td>
</tr>
</tbody>
</table>
<h3 id="application-networking.k8s.aws/v1alpha1.ServiceImportSpec">ServiceImportSpec
</h3>
<p>
(<em>Appears on:</em><a href="#application-networking.k8s.aws/v1alpha1.ServiceImport">ServiceImport</a>)
</p>
<div>
<p>ServiceImportSpec describes an imported service and the information necessary to consume it.</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>ports</code><br/>
<em>
<a href="#application-networking.k8s.aws/v1alpha1.ServicePort">
[]ServicePort
</a>
</em>
</td>
<td>
</td>
</tr>
<tr>
<td>
<code>ips</code><br/>
<em>
[]string
</em>
</td>
<td>
<em>(Optional)</em>
<p>ip will be used as the VIP for this service when type is ClusterSetIP.</p>
</td>
</tr>
<tr>
<td>
<code>type</code><br/>
<em>
<a href="#application-networking.k8s.aws/v1alpha1.ServiceImportType">
ServiceImportType
</a>
</em>
</td>
<td>
<p>type defines the type of this service.
Must be ClusterSetIP or Headless.</p>
</td>
</tr>
<tr>
<td>
<code>sessionAffinity</code><br/>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.24/#serviceaffinity-v1-core">
Kubernetes core/v1.ServiceAffinity
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>Supports &ldquo;ClientIP&rdquo; and &ldquo;None&rdquo;. Used to maintain session affinity.
Enable client IP based session affinity.
Must be ClientIP or None.
Defaults to None.
Ignored when type is Headless
More info: <a href="https://kubernetes.io/docs/concepts/services-networking/service/#virtual-ips-and-service-proxies">https://kubernetes.io/docs/concepts/services-networking/service/#virtual-ips-and-service-proxies</a></p>
</td>
</tr>
<tr>
<td>
<code>sessionAffinityConfig</code><br/>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.24/#sessionaffinityconfig-v1-core">
Kubernetes core/v1.SessionAffinityConfig
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>sessionAffinityConfig contains session affinity configuration.</p>
</td>
</tr>
</tbody>
</table>
<h3 id="application-networking.k8s.aws/v1alpha1.ServiceImportStatus">ServiceImportStatus
</h3>
<p>
(<em>Appears on:</em><a href="#application-networking.k8s.aws/v1alpha1.ServiceImport">ServiceImport</a>)
</p>
<div>
<p>ServiceImportStatus describes derived state of an imported service.</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>clusters</code><br/>
<em>
<a href="#application-networking.k8s.aws/v1alpha1.ClusterStatus">
[]ClusterStatus
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>clusters is the list of exporting clusters from which this service
was derived.</p>
</td>
</tr>
</tbody>
</table>
<h3 id="application-networking.k8s.aws/v1alpha1.ServiceImportType">ServiceImportType
(<code>string</code> alias)</h3>
<p>
(<em>Appears on:</em><a href="#application-networking.k8s.aws/v1alpha1.ServiceImportSpec">ServiceImportSpec</a>)
</p>
<div>
<p>ServiceImportType designates the type of a ServiceImport</p>
</div>
<table>
<thead>
<tr>
<th>Value</th>
<th>Description</th>
</tr>
</thead>
<tbody><tr><td><p>&#34;ClusterSetIP&#34;</p></td>
<td><p>ClusterSetIP are only accessible via the ClusterSet IP.</p>
</td>
</tr><tr><td><p>&#34;Headless&#34;</p></td>
<td><p>Headless services allow backend pods to be addressed directly.</p>
</td>
</tr></tbody>
</table>
<h3 id="application-networking.k8s.aws/v1alpha1.ServicePort">ServicePort
</h3>
<p>
(<em>Appears on:</em><a href="#application-networking.k8s.aws/v1alpha1.ServiceImportSpec">ServiceImportSpec</a>)
</p>
<div>
<p>ServicePort represents the port on which the service is exposed</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>name</code><br/>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>The name of this port within the service. This must be a DNS_LABEL.
All ports within a ServiceSpec must have unique names. When considering
the endpoints for a Service, this must match the &lsquo;name&rsquo; field in the
EndpointPort.
Optional if only one ServicePort is defined on this service.</p>
</td>
</tr>
<tr>
<td>
<code>protocol</code><br/>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.24/#protocol-v1-core">
Kubernetes core/v1.Protocol
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>The IP protocol for this port. Supports &ldquo;TCP&rdquo;, &ldquo;UDP&rdquo;, and &ldquo;SCTP&rdquo;.
Default is TCP.</p>
</td>
</tr>
<tr>
<td>
<code>appProtocol</code><br/>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>The application protocol for this port.
This field follows standard Kubernetes label syntax.
Un-prefixed names are reserved for IANA standard service names (as per
RFC-6335 and <a href="http://www.iana.org/assignments/service-names)">http://www.iana.org/assignments/service-names)</a>.
Non-standard protocols should use prefixed names such as
mycompany.com/my-custom-protocol.
Field can be enabled with ServiceAppProtocol feature gate.</p>
</td>
</tr>
<tr>
<td>
<code>port</code><br/>
<em>
int32
</em>
</td>
<td>
<p>The port that will be exposed by this service.</p>
</td>
</tr>
</tbody>
</table>
<h3 id="application-networking.k8s.aws/v1alpha1.TargetGroupPolicySpec">TargetGroupPolicySpec
</h3>
<p>
(<em>Appears on:</em><a href="#application-networking.k8s.aws/v1alpha1.TargetGroupPolicy">TargetGroupPolicy</a>)
</p>
<div>
<p>TargetGroupPolicySpec defines the desired state of TargetGroupPolicy.</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>protocol</code><br/>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>The protocol to use for routing traffic to the targets. Supported values are HTTP (default) and HTTPS.</p>
<p>Changes to this value results in a replacement of VPC Lattice target group.</p>
</td>
</tr>
<tr>
<td>
<code>protocolVersion</code><br/>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>The protocol version to use. Supported values are HTTP1 (default) and HTTP2. When a policy is behind GRPCRoute,
this field value will be ignored as GRPC is only supported through HTTP/2.</p>
<p>Changes to this value results in a replacement of VPC Lattice target group.</p>
</td>
</tr>
<tr>
<td>
<code>targetRef</code><br/>
<em>
<a href="https://gateway-api.sigs.k8s.io/geps/gep-713/?h=policytargetreference#policy-targetref-api">
sigs.k8s.io/gateway-api/apis/v1alpha2.PolicyTargetReference
</a>
</em>
</td>
<td>
<p>TargetRef points to the kubernetes Service resource that will have this policy attached.</p>
<p>This field is following the guidelines of Kubernetes Gateway API policy attachment.</p>
</td>
</tr>
<tr>
<td>
<code>healthCheck</code><br/>
<em>
<a href="#application-networking.k8s.aws/v1alpha1.HealthCheckConfig">
HealthCheckConfig
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>The health check configuration.</p>
<p>Changes to this value will update VPC Lattice resource in place.</p>
</td>
</tr>
</tbody>
</table>
<h3 id="application-networking.k8s.aws/v1alpha1.TargetGroupPolicyStatus">TargetGroupPolicyStatus
</h3>
<p>
(<em>Appears on:</em><a href="#application-networking.k8s.aws/v1alpha1.TargetGroupPolicy">TargetGroupPolicy</a>)
</p>
<div>
<p>TargetGroupPolicyStatus defines the observed state of TargetGroupPolicy.</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>conditions</code><br/>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.24/#condition-v1-meta">
[]Kubernetes meta/v1.Condition
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>Conditions describe the current conditions of the AccessLogPolicy.</p>
<p>Implementations should prefer to express Policy conditions
using the <code>PolicyConditionType</code> and <code>PolicyConditionReason</code>
constants so that operators and tools can converge on a common
vocabulary to describe AccessLogPolicy state.</p>
<p>Known condition types are:</p>
<ul>
<li>&ldquo;Accepted&rdquo;</li>
<li>&ldquo;Ready&rdquo;</li>
</ul>
</td>
</tr>
</tbody>
</table>
<h3 id="application-networking.k8s.aws/v1alpha1.VpcAssociationPolicySpec">VpcAssociationPolicySpec
</h3>
<p>
(<em>Appears on:</em><a href="#application-networking.k8s.aws/v1alpha1.VpcAssociationPolicy">VpcAssociationPolicy</a>)
</p>
<div>
<p>VpcAssociationPolicySpec defines the desired state of VpcAssociationPolicy.</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>securityGroupIds</code><br/>
<em>
<a href="#application-networking.k8s.aws/v1alpha1.SecurityGroupId">
[]SecurityGroupId
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>SecurityGroupIds defines the security groups enforced on the VpcServiceNetworkAssociation.
Security groups does not take effect if AssociateWithVpc is set to false.</p>
<p>For more details, please check the VPC Lattice documentation <a href="https://docs.aws.amazon.com/vpc-lattice/latest/ug/security-groups.html">https://docs.aws.amazon.com/vpc-lattice/latest/ug/security-groups.html</a></p>
</td>
</tr>
<tr>
<td>
<code>associateWithVpc</code><br/>
<em>
bool
</em>
</td>
<td>
<em>(Optional)</em>
<p>AssociateWithVpc indicates whether the VpcServiceNetworkAssociation should be created for the current VPC of k8s cluster.</p>
<p>This value will be considered true by default.</p>
</td>
</tr>
<tr>
<td>
<code>targetRef</code><br/>
<em>
<a href="https://gateway-api.sigs.k8s.io/geps/gep-713/?h=policytargetreference#policy-targetref-api">
sigs.k8s.io/gateway-api/apis/v1alpha2.PolicyTargetReference
</a>
</em>
</td>
<td>
<p>TargetRef points to the kubernetes Gateway resource that will have this policy attached.</p>
<p>This field is following the guidelines of Kubernetes Gateway API policy attachment.</p>
</td>
</tr>
</tbody>
</table>
<h3 id="application-networking.k8s.aws/v1alpha1.VpcAssociationPolicyStatus">VpcAssociationPolicyStatus
</h3>
<p>
(<em>Appears on:</em><a href="#application-networking.k8s.aws/v1alpha1.VpcAssociationPolicy">VpcAssociationPolicy</a>)
</p>
<div>
<p>VpcAssociationPolicyStatus defines the observed state of VpcAssociationPolicy.</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>conditions</code><br/>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.24/#condition-v1-meta">
[]Kubernetes meta/v1.Condition
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>Conditions describe the current conditions of the VpcAssociationPolicy.</p>
<p>Implementations should prefer to express Policy conditions
using the <code>PolicyConditionType</code> and <code>PolicyConditionReason</code>
constants so that operators and tools can converge on a common
vocabulary to describe VpcAssociationPolicy state.</p>
<p>Known condition types are:</p>
<ul>
<li>&ldquo;Accepted&rdquo;</li>
</ul>
</td>
</tr>
</tbody>
</table>
<hr/>
<p><em>
Generated with <code>gen-crd-api-reference-docs</code>
on git commit <code>5de8f32</code>.
</em></p>
