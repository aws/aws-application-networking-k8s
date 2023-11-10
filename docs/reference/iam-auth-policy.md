# IAMAuthPolicy API Reference

## Introduction

VPC Lattice auth policies are IAM policy documents that you attach to service networks or services to control whether a specified principal has access to a group of services or specific service (AuthZ). 
By attaching Kubernetes IAMAuthPolicy CRD to the k8s gateway or k8s route, you could apply auth policy to corresponding VPC Lattice service network or VPC Lattice service that you want to control access. 
Please check [VPC Lattice auth policy documentation](https://docs.aws.amazon.com/vpc-lattice/latest/ug/auth-policies.html) for more details.

[This article](https://aws.amazon.com/blogs/containers/implement-aws-iam-authentication-with-amazon-vpc-lattice-and-amazon-eks/) is also a good reference on how to set up VPC Lattice auth policy in the kubernetes.

## API Specification

<h3 id="application-networking.k8s.aws/v1alpha1.IAMAuthPolicy">IAMAuthPolicy</h3>
<div></div>
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
                     sigs.k8s.io/gateway-api/apis/v1alpha2.PolicyTargetReference
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
<h3 id="application-networking.k8s.aws/v1alpha1.IAMAuthPolicySpec">IAMAuthPolicySpec</h3>
<p>
   (<em>Appears on:</em><a href="#application-networking.k8s.aws/v1alpha1.IAMAuthPolicy">IAMAuthPolicy</a>)
</p>
<div>
   <p>IAMAuthPolicySpec defines the desired state of IAMAuthPolicy.
      When the controller handles IAMAuthPolicy creation, if the targetRef k8s and VPC Lattice resource exists, the controller will change the auth_type of that VPC Lattice resource to AWS_IAM and attach this policy.
      When the controller handles IAMAuthPolicy deletion, if the targetRef k8s and VPC Lattice resource exists, the controller will change the auth_type of that VPC Lattice resource to NONE and detach this policy.
   </p>
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
            sigs.k8s.io/gateway-api/apis/v1alpha2.PolicyTargetReference
            </em>
         </td>
         <td>
            <p>TargetRef points to the Kubernetes Gateway, HTTPRoute, or GRPCRoute resource that will have this policy attached.</p>
            <p>This field is following the guidelines of Kubernetes Gateway API policy attachment.</p>
         </td>
      </tr>
   </tbody>
</table>
<h3 id="application-networking.k8s.aws/v1alpha1.IAMAuthPolicyStatus">IAMAuthPolicyStatus</h3>
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
               vocabulary to describe IAMAuthPolicy state.
            </p>
            <p>Known condition types are:</p>
            <ul>
               <li>&ldquo;Accepted&rdquo;</li>
               <li>&ldquo;Ready&rdquo;</li>
            </ul>
         </td>
      </tr>
   </tbody>
</table>


## IAMAauthPolicy Example

```yaml
apiVersion: application-networking.k8s.aws/v1alpha1
kind: IAMAuthPolicy
metadata:
    name: test-iam-auth-policy
spec:
    targetRef:
        group: "gateway.networking.k8s.io"
        kind: HTTPRoute
        name: my-route
    policy: |
        {
            "Version": "2012-10-17",
            "Statement": [
                {
                    "Effect": "Allow",
                    "Principal": "*",
                    "Action": "vpc-lattice-svcs:Invoke",
                    "Resource": "*",
                    "Condition": {
                        "StringEquals": {
                            "vpc-lattice-svcs:RequestHeader/header1": "value1"
                        }
                    }
                }
            ]
        }
```

If you create the above IAMAuthPolicy in the k8s cluster, the `my-route` (and it's corresponding VPC Lattice service) will be attached with the given IAM auth policy. 
Only HTTP traffic with header `header1:value1` will be allowed to access the `my-route`. Please check the [VPC Lattice documentation](https://docs.aws.amazon.com/vpc-lattice/latest/ug/auth-policies.html#auth-policies-common-elements) to get more detail on how lattice auth policy work.



