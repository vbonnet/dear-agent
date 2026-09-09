//go:build !darwin || !arm64

package buildauthority

func platformSourcePrimitives() (sourcePrimitives, *sourcePrimitiveFailure) {
	return nil, newSourcePrimitiveFailure(OperationValidate, CauseUnsupported)
}
