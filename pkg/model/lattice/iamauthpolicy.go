package lattice

type IAMAuthPolicy struct {
	ResourceId string
	Policy     string
}

type IAMAuthPolicyStatus struct {
	ResourceId string
	State      string
}
