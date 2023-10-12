package lattice

import pkg_aws "github.com/aws/aws-application-networking-k8s/pkg/aws"

var TestCloudConfig = pkg_aws.CloudConfig{
	VpcId:       "vpc-id",
	AccountId:   "account-id",
	Region:      "region",
	ClusterName: "cluster",
}
