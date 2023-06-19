package envoy

import "errors"

type PolicyType string

const (
	IngressPolicyType PolicyType = "ingress"

	EgressPolicyType PolicyType = "egress"

	AppPolicyType PolicyType = "app"
)

func (lt PolicyType) IsValid() error {
	switch lt {
	case IngressPolicyType, EgressPolicyType, AppPolicyType:
		return nil
	}
	return errors.New("invalid policy type")
}
