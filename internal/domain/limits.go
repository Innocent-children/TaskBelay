package domain

import "time"

const (
	MaxRepositoryPathBytes                = 4_096
	MaxRepositoryKeyBytes                 = 128
	MaxAdditionalRepositories             = 7
	MaxRepositoryScopeEntries             = 1 + MaxAdditionalRepositories
	MaxGoalBytes                          = 8_192
	MaxScopeItems                         = 64
	MaxScopeItemBytes                     = 1_024
	MaxOutOfScopeItems                    = 64
	MaxOutOfScopeItemBytes                = 1_024
	MaxAcceptanceCriteriaItems            = 64
	MaxAcceptanceCriterionBytes           = 2_048
	MaxContractAggregateBytes             = 262_144
	MaxEvidencePerAction                  = 32
	MaxEvidenceNameBytes                  = 256
	MaxEvidenceSummaryBytes               = 2_048
	MaxRetainedEvidenceItems              = 256
	MaxRetainedVerificationAttempts       = 3
	MaxVerificationBudgetAdjustments      = 64
	MaxFileScopeRecords                   = 64
	MaxFileScopePaths                     = 64
	MaxBoundedStringListItems             = 64
	MaxBlockerMessageBytes                = 4_096
	MaxReasonBytes                        = 4_096
	MaxGuidanceBytes                      = 4_096
	MaxOutcomeSummaryBytes                = 4_096
	MaxOutcomeNarrativeAggregateBytes     = 131_072
	MaxResolutionTextBytes                = 4_096
	MaxErrorMessageBytes                  = 4_096
	MaxIdentifierBytes                    = 128
	MaxActionPayloadBytes                 = 131_072
	MaxTaskAggregateBytes                 = 786_432
	MaxResultEnvelopeOverheadBytes        = 131_072
	MaxResultEnvelopeBytes                = 1_048_576
	MaxPersistedTaskSnapshotBytes         = 1_048_576
	MaxBaselineHistoryPageEntries         = 32 // Read-page limit, never a retention limit.
	MaxBaselineHistoryPageBytes           = 65_536
	MaxArtifactReferencesPerAction        = 16
	MaxMethodEvidencePerAction            = 16
	MaxWorkItemsPerTaskPlan               = 64
	MaxDependenciesPerWorkItem            = 64
	MaxExplainedComponents                = 64
	MaxStandardProcessTransitions         = 64
	MaxStandardProcessNodes               = 16
	MaxFingerprintPaths                   = 1_024
	MaxRepositoryDeltaPaths               = MaxFingerprintPaths * 2
	MaxGitCommandOutputBytes              = 1_048_576
	MaxAutomaticVerificationCommands      = 20
	MaxTotalAutomaticVerificationCommands = MaxAutomaticVerificationCommands * (MaxVerificationBudgetAdjustments + 1)
	GitCommandTimeout                     = 10 * time.Second
	SQLiteBusyTimeout                     = 5 * time.Second
)
