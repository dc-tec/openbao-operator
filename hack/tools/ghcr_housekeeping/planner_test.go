package main

import (
	"context"
	"errors"
	"testing"
)

func TestPlanDeletionsAppliesGlobalAndPerPackageBudgets(t *testing.T) {
	t.Parallel()

	report := housekeepingReport{Packages: []packageReport{
		{
			Name: "package-b",
			CandidateItems: []candidateReport{
				{ID: 2, UpdatedAt: "2026-01-01T00:00:00Z"},
				{ID: 4, UpdatedAt: "2026-01-03T00:00:00Z"},
			},
		},
		{
			Name: "package-a",
			CandidateItems: []candidateReport{
				{ID: 1, UpdatedAt: "2026-01-01T00:00:00Z"},
				{ID: 3, UpdatedAt: "2026-01-02T00:00:00Z"},
				{ID: 5, UpdatedAt: "2026-01-04T00:00:00Z"},
			},
		},
	}}

	planDeletions(&report, 2, 3)

	if report.Packages[0].Planned != 1 || report.Packages[1].Planned != 2 {
		t.Fatalf(
			"planned counts = [%d,%d], want [1,2]",
			report.Packages[0].Planned,
			report.Packages[1].Planned,
		)
	}
	planned := map[int64]bool{}
	for _, pkg := range report.Packages {
		for _, candidate := range pkg.CandidateItems {
			planned[candidate.ID] = candidate.Planned
		}
	}
	for _, id := range []int64{1, 2, 3} {
		if !planned[id] {
			t.Errorf("candidate %d was not planned", id)
		}
	}
	for _, id := range []int64{4, 5} {
		if planned[id] {
			t.Errorf("candidate %d was planned past the budget", id)
		}
	}
}

func TestApplyDeletionPlanStopsAfterFirstError(t *testing.T) {
	t.Parallel()

	report := housekeepingReport{Packages: []packageReport{
		{
			Name: "openbao-operator",
			CandidateItems: []candidateReport{
				{ID: 1, Planned: true},
				{ID: 2, Planned: true},
			},
		},
	}}
	client := &fakePackageClient{deleteErrors: map[int64]error{1: errors.New("forbidden")}}
	opts := options{Owner: "dc-tec", OwnerKind: ownerKindUser}

	err := applyDeletionPlan(context.Background(), opts, client, &report)
	if err == nil {
		t.Fatalf("applyDeletionPlan() error = nil, want failure")
	}
	if len(client.deleteCalls) != 1 || client.deleteCalls[0] != 1 {
		t.Fatalf("delete calls = %v, want only the first candidate", client.deleteCalls)
	}
	if report.Packages[0].Deleted != 0 || len(report.Packages[0].Errors) != 1 {
		t.Fatalf("package report = %#v", report.Packages[0])
	}
}

func TestPlanDeletionsPrioritizesTaggedTransientsOverOrphans(t *testing.T) {
	t.Parallel()

	report := housekeepingReport{Packages: []packageReport{
		{
			Name: "openbao-operator",
			CandidateItems: []candidateReport{
				{ID: 1, Kind: candidateKindOCIOrphan, UpdatedAt: "2026-01-01T00:00:00Z"},
				{ID: 2, Kind: candidateKindTaggedTransient, UpdatedAt: "2026-07-01T00:00:00Z"},
				{ID: 3, Kind: candidateKindOCIReferrerOrphan, UpdatedAt: "2026-02-01T00:00:00Z"},
			},
		},
	}}

	planDeletions(&report, 100, 2)
	if report.Packages[0].CandidateItems[0].Planned {
		t.Fatalf("untagged orphan was planned before higher-priority candidates")
	}
	if !report.Packages[0].CandidateItems[1].Planned {
		t.Fatalf("tagged transient was not planned")
	}
	if !report.Packages[0].CandidateItems[2].Planned {
		t.Fatalf("detached referrer index was not planned before untagged orphan")
	}
}

func TestApplyDeletionPlanUsesPlannerPriority(t *testing.T) {
	t.Parallel()

	report := housekeepingReport{Packages: []packageReport{
		{
			Name: "openbao-operator",
			CandidateItems: []candidateReport{
				{ID: 1, Kind: candidateKindOCIOrphan, UpdatedAt: "2026-01-01T00:00:00Z", Planned: true},
			},
		},
		{
			Name: "pr-e2e-openbao-operator",
			CandidateItems: []candidateReport{
				{ID: 2, Kind: candidateKindTaggedTransient, UpdatedAt: "2026-07-01T00:00:00Z", Planned: true},
			},
		},
	}}
	client := &fakePackageClient{}
	opts := options{Owner: "dc-tec", OwnerKind: ownerKindUser}

	if err := applyDeletionPlan(context.Background(), opts, client, &report); err != nil {
		t.Fatalf("applyDeletionPlan() error = %v", err)
	}
	if len(client.deleteCalls) != 2 || client.deleteCalls[0] != 2 || client.deleteCalls[1] != 1 {
		t.Fatalf("delete calls = %v, want tagged candidate before orphan", client.deleteCalls)
	}
}

func TestValidateTaggedCandidateSafetyIgnoresOCIOrphanBacklog(t *testing.T) {
	t.Parallel()

	report := housekeepingReport{Packages: []packageReport{
		{Name: "openbao-operator", TaggedCandidates: 1, OrphanCandidates: 500, Candidates: 501},
	}}
	if problems := validateTaggedCandidateSafety(&report, 100); len(problems) != 0 {
		t.Fatalf("validateTaggedCandidateSafety() problems = %v, want none", problems)
	}

	report.Packages[0].TaggedCandidates = 101
	if problems := validateTaggedCandidateSafety(&report, 100); len(problems) != 1 {
		t.Fatalf("validateTaggedCandidateSafety() problems = %v, want one", problems)
	}
}
