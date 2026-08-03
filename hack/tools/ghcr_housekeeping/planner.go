package main

import (
	"context"
	"errors"
	"fmt"
	"sort"
)

type candidatePosition struct {
	PackageIndex   int
	CandidateIndex int
}

func validateTaggedCandidateSafety(report *housekeepingReport, maxPerPackage int) []string {
	problems := make([]string, 0)
	for i := range report.Packages {
		pkgReport := &report.Packages[i]
		if pkgReport.TaggedCandidates <= maxPerPackage {
			continue
		}
		errMsg := fmt.Sprintf(
			"%s: tagged candidate count %d exceeds max-delete-per-package=%d; use workflow_dispatch override to continue",
			pkgReport.Name,
			pkgReport.TaggedCandidates,
			maxPerPackage,
		)
		pkgReport.Errors = append(pkgReport.Errors, errMsg)
		problems = append(problems, errMsg)
	}
	return problems
}

func planDeletions(report *housekeepingReport, maxPerPackage, maxTotal int) {
	positions := make([]candidatePosition, 0)
	for packageIndex := range report.Packages {
		pkgReport := &report.Packages[packageIndex]
		pkgReport.Planned = 0
		for candidateIndex := range pkgReport.CandidateItems {
			pkgReport.CandidateItems[candidateIndex].Planned = false
			positions = append(positions, candidatePosition{
				PackageIndex:   packageIndex,
				CandidateIndex: candidateIndex,
			})
		}
	}
	positions = sortedCandidatePositions(report, positions)

	plannedPerPackage := make([]int, len(report.Packages))
	plannedTotal := 0
	for _, position := range positions {
		if plannedTotal >= maxTotal {
			break
		}
		if plannedPerPackage[position.PackageIndex] >= maxPerPackage {
			continue
		}

		pkgReport := &report.Packages[position.PackageIndex]
		pkgReport.CandidateItems[position.CandidateIndex].Planned = true
		pkgReport.Planned++
		plannedPerPackage[position.PackageIndex]++
		plannedTotal++
	}
}

func sortedCandidatePositions(report *housekeepingReport, positions []candidatePosition) []candidatePosition {
	sort.Slice(positions, func(i, j int) bool {
		leftPosition := positions[i]
		rightPosition := positions[j]
		leftPackage := &report.Packages[leftPosition.PackageIndex]
		rightPackage := &report.Packages[rightPosition.PackageIndex]
		left := leftPackage.CandidateItems[leftPosition.CandidateIndex]
		right := rightPackage.CandidateItems[rightPosition.CandidateIndex]
		if candidatePriority(left.Kind) != candidatePriority(right.Kind) {
			return candidatePriority(left.Kind) < candidatePriority(right.Kind)
		}
		if left.UpdatedAt != right.UpdatedAt {
			return left.UpdatedAt < right.UpdatedAt
		}
		if leftPackage.Name != rightPackage.Name {
			return leftPackage.Name < rightPackage.Name
		}
		return left.ID < right.ID
	})
	return positions
}

func candidatePriority(kind string) int {
	if kind == candidateKindTaggedTransient {
		return 0
	}
	if kind == candidateKindOCIReferrerOrphan {
		return 1
	}
	if kind == candidateKindOCIOrphan {
		return 2
	}
	return 3
}

func applyDeletionPlan(
	ctx context.Context,
	opts options,
	client packageClient,
	report *housekeepingReport,
) error {
	positions := make([]candidatePosition, 0)
	for packageIndex := range report.Packages {
		for candidateIndex, candidate := range report.Packages[packageIndex].CandidateItems {
			if candidate.Planned {
				positions = append(positions, candidatePosition{
					PackageIndex:   packageIndex,
					CandidateIndex: candidateIndex,
				})
			}
		}
	}
	for _, position := range sortedCandidatePositions(report, positions) {
		pkgReport := &report.Packages[position.PackageIndex]
		candidate := pkgReport.CandidateItems[position.CandidateIndex]
		if err := client.DeletePackageVersion(
			ctx,
			opts.OwnerKind,
			opts.Owner,
			pkgReport.Name,
			candidate.ID,
		); err != nil {
			errMsg := fmt.Sprintf("%s: delete version %d failed: %v", pkgReport.Name, candidate.ID, err)
			pkgReport.Errors = append(pkgReport.Errors, errMsg)
			return errors.New(errMsg)
		}
		pkgReport.Deleted++
	}
	return nil
}
