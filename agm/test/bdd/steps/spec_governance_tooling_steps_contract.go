package steps

const (
	specGovernancePinnedInventoryStep      = `^AGM runs the focused pinned SPEC inventory unit check$`
	specGovernanceNonVerdictLeadStep       = `^AGM runs the focused non-verdict SPEC audit lead unit check$`
	specGovernanceReciprocalDiagnosticStep = `^AGM runs the focused reciprocal SPEC and BDD diagnostic unit check$`
	specGovernanceFindingValidationStep    = `^AGM runs the focused pinned finding validation unit check$`
	specGovernanceOfflineRenderingStep     = `^AGM runs the focused bounded offline rendering unit check$`
	specGovernanceFindingFilterStep        = `^AGM runs the focused candidate and boundary card filtering unit check$`
	specGovernanceReadOnlyBoundaryStep     = `^AGM runs the focused read-only audit boundary unit check$`
	specGovernanceResultStep               = `^the focused SPEC audit unit check should pass$`
)
