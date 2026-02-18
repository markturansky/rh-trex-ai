package mocks

import (
	"github.com/openshift-online/rh-trex-ai/pkg/client/apiclient"
	pkgmocks "github.com/openshift-online/rh-trex-ai/pkg/testutil/mocks"
)

type AuthzValidatorMock = pkgmocks.AuthzValidatorMock

func NewAuthzValidatorMockClient() (*AuthzValidatorMock, *apiclient.Client) {
	return pkgmocks.NewAuthzValidatorMockClient()
}
