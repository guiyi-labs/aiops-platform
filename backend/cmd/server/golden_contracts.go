package main

import (
	"k8s-aiops.local/backend/internal/aiinvestigator"
	"k8s-aiops.local/backend/internal/automation"
	"k8s-aiops.local/backend/internal/correlation"
	"k8s-aiops.local/backend/internal/golden"
	"k8s-aiops.local/backend/internal/signal"
	"k8s-aiops.local/backend/internal/slo"
)

// goldenEngineContracts builds the EngineContracts snapshot from the
// actual package-level version constants and status enumerations of the
// M39-M44 AIOps engine packages. This adapter keeps the golden package
// free of imports to the engine packages (avoiding potential cycles)
// while ensuring the runner sees the live engine contracts.
func goldenEngineContracts() golden.EngineContracts {
	return golden.EngineContracts{
		Versions: golden.EngineVersions{
			SignalVersion:       signal.SchemaVersionV1,
			TopologyVersion:     "1.0",
			SLOVersion:          slo.TemplateVersion,
			CorrelationVersion:  correlation.CorrelationVersion,
			InvestigatorVersion: aiinvestigator.InvestigatorVersion,
			AutomationVersion:   automation.AutomationVersion,
			VerifierVersion:     automation.VerifierVersion,
		},
		ValidPlanStatuses: map[string]bool{
			string(automation.StatusDraft):     true,
			string(automation.StatusPreviewed): true,
			string(automation.StatusApproved):  true,
			string(automation.StatusExecuting): true,
			string(automation.StatusSucceeded): true,
			string(automation.StatusFailed):    true,
			string(automation.StatusExpired):   true,
			string(automation.StatusCancelled): true,
			string(automation.StatusVerified):  true,
		},
		ValidVerificationStatuses: map[string]bool{
			string(automation.VerificationStatusPending):     true,
			string(automation.VerificationStatusEffective):   true,
			string(automation.VerificationStatusIneffective): true,
			string(automation.VerificationStatusFailed):      true,
			string(automation.VerificationStatusUnknown):     true,
		},
	}
}
