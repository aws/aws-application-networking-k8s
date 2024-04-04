# Share Kubernetes Gateway (VPC Lattice Service Network) between different AWS accounts

[AWS Resource Access Manager](https://aws.amazon.com/ram/) (AWS RAM) helps you share your resources across AWS Accounts, within your [AWS Organization](https://aws.amazon.com/organizations/) or Organizational Units (OUs).  RAM supports 2 types of VPC Lattice resource sharing: VPC Lattice Services and Service Networks.

Let's build an example where **<span style="color:green">Account A (*sharer account*)</span>**  shares its Service Network with **<span style="color:red">Account B (*sharee account*)</span>**, and **<span style="color:red">Account B</span>** can access all Kubernetes `services` (VPC Lattice Target Groups) and Kubernetes `HTTPRoutes`(VPC Lattice Services) within this sharer account's service network.

**Create a VPC Lattice Resources**

In **<span style="color:green">Account A</span>**, set up a cluster with the Controller and an example application installed. You can follow the [Getting Started guide](../guides/getstarted.md)  up to the ["Single Cluster"](../guides/getstarted.md#single-cluster) section.


**Share the VPC Lattice Service Network**

Now that we have a VPC Lattice Service Network and Service in **<span style="color:green">Account A </span>**, share this Service Network to **<span style="color:red">Account B</span>**.

1. Retrieve the `my-hotel`Service Network Identifier:
    ```bash
    aws vpc-lattice list-service-networks --query "items[?name=="\'my-hotel\'"].id" | jq -r '.[]'
    ```

1. Share the `my-hotel` Service Network, using the identifier retrieved in the previous step.
    1. Open the AWS RAM console in **<span style="color:green">Account A</span>** and create a resource share.
    <figure markdown="span">
    ![Resource Shares](../images/resource-share1.png){ width="800" }
    </figure>
    1. Select `VPC Lattice Service Network` resource sharing type.
    <figure markdown="span">
    ![Resource Shares](../images/resource-share2.png){ width="800" }
    </figure>
    1. Select the `my-hotel` Service Network identifier retrieved in the previous step.
    <figure markdown="span">
    ![Resource Shares](../images/resource-share3.png){ width="800" }
    </figure>
    1. Associate AWS Managed Permissions.
    <figure markdown="span">
    ![Resource Shares](../images/resource-share4.png){ width="800" }
    </figure>
    1. Set **<span style="color:red">Account B</span>** as principal.
    <figure markdown="span">
    ![Resource Shares](../images/resource-share5.png){ width="800" }
    </figure>
    1. Review and create the resource share.
    <figure markdown="span">
    ![Resource Shares](../images/resource-share6.png){ width="800" }
    </figure>

1. Open the **<span style="color:red">Account B</span>**'s AWS RAM console and accept **<span style="color:green">Account A</span>**'s Service Network sharing invitation in the *"Shared with me"* section. 

    <figure markdown="span">
    ![Resource Shares](../images/accept-resource-share.png){ width="800" }
    </figure>


1. Switch back to **<span style="color:green">Account A</span>**, retrieve the Service Network ID.

    ```bash
    SERVICE_NETWORK_ID=$(aws vpc-lattice list-service-networks --query "items[?name=="\'my-hotel\'"].id" | jq -r '.[]')
    echo $SERVICE_NETWORK_ID
    ```

1.  Switch to **<span style="color:red">Account B</span>** and verify that `my-hotel` Service Network resource is available in **<span style="color:red">Account B</span>** (referring to the `SERVICE_NETWORK_ID` retrived in the previous step).

1. Now choose an Amazon VPC in **<span style="color:red">Account B</span>** to attach to the `my-hotel` Service Network.

    ```sh
    VPC_ID=<your_vpc_id>
    aws vpc-lattice create-service-network-vpc-association --service-network-identifier $SERVICE_NETWORK_ID --vpc-identifier $VPC_ID
    ```

    !!!Warning
        VPC Lattice is a regional service, therefore the VPC **must** be in the same AWS Region of the Service Network you created in **<span style="color:green">Account A</span>**. 

**Test cross-account connectivity**

You can verify that the `parking` and `review` microservices - in **<span style="color:green">Account A</span>** - can be consumed from resources in  the assocuated VPC in **<span style="color:red">Account B</span>**. 

1. To simplify, let's [create and connect to a Cloud9 environment](https://docs.aws.amazon.com/cloud9/latest/user-guide/tutorial-create-environment.html) in the VPC you previously attached to the `my-hotel` Service Network. 

1. In **<span style="color:green">Account A</span>**, retrieve the VPC Lattice Services urls.
    ```sh
    ratesFQDN=$(aws vpc-lattice list-services --query "items[?name=="\'rates-default\'"].dnsEntry" | jq -r '.[].domainName')
    inventoryFQDN=$(aws vpc-lattice list-services --query "items[?name=="\'inventory-default\'"].dnsEntry" | jq -r '.[].domainName')
    echo "$ratesFQDN \n$inventoryFQDN"
    ```
    ```

1. In the Cloud9 instance in **<span style="color:red">Account B</span>**, install `curl` in the instance and curl `parking` and `rates` microservices:

    ```sh
    sudo apt-get install curl
    curl $ratesFQDN/parking $ratesFQDN/review
    ```

**Cleanup**

To avoid additional charges, remove the demo infrastructure from your AWS Accounts.

1. Delete the Service Network Association you created in **<span style="color:red">Account B</span>**. In **<span style="color:green">Account A</span>**:
    ```bash
    VPC_ID=<accountB_vpc_id>
    SERVICE_NETWORK_ASSOCIATION_IDENTIFIER=$(aws vpc-lattice list-service-network-vpc-associations --vpc-id $VPC_ID --query "items[?serviceNetworkName=="\'my-hotel\'"].id" | jq -r '.[]')
    aws vpc-lattice delete-service-network-vpc-association  --service-network-vpc-association-identifier $SERVICE_NETWORK_ASSOCIATION_IDENTIFIER
    ```

    Ensure the Service Network Association is deleted:
    ```bash 
    aws vpc-lattice list-service-network-vpc-associations --vpc-id $VPC_ID
    ```

1. [Delete the Service Network RAM share resource](https://docs.aws.amazon.com/ram/latest/userguide/working-with-sharing-delete.html) in AWS RAM Console.

1. Follow the [cleanup section of the getting Started guide](../guides/getstarted.md/#cleanup) to delete Cluster and Service Network Resources in **<span style="color:green">Account A</span>**.

1. [Delete the Cloud9 Environment](https://docs.aws.amazon.com/cloud9/latest/user-guide/tutorial-clean-up.html) in **<span style="color:red">Account B</span>**. 