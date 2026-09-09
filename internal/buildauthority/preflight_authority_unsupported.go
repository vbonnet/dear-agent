//go:build !darwin || !arm64

package buildauthority

func platformPreflightAuthorityPrimitives() (
	preflightAuthorityPrimitives,
	*FailureRecord,
) {
	return preflightAuthorityPrimitives{}, authorityFailure(
		OperationValidate,
		CauseUnsupported,
	)
}
