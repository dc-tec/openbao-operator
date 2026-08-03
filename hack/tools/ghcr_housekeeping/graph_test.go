package main

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestResolveOCIGraphFollowsRootsIndexesAndReferrers(t *testing.T) {
	t.Parallel()

	root := testDigest("a")
	child := testDigest("b")
	referrerIndex := testDigest("c")
	artifact := testDigest("d")
	orphan := testDigest("e")
	detachedSubject := testDigest("f")
	detachedReferrerIndex := testDigest("1")

	versions := []packageVersion{
		{ID: 1, Name: root, Tags: []string{"edge"}},
		{ID: 2, Name: child},
		{ID: 3, Name: referrerIndex, Tags: []string{referrerTag(root)}},
		{ID: 4, Name: artifact},
		{ID: 5, Name: orphan},
		{ID: 6, Name: detachedReferrerIndex, Tags: []string{referrerTag(detachedSubject)}},
	}
	client := &fakeManifestGraphClient{
		references: map[string][]manifestReference{
			root: {
				{Digest: child, MediaType: "application/vnd.oci.image.manifest.v1+json"},
			},
			referrerIndex: {
				{Digest: artifact, MediaType: "application/vnd.oci.image.manifest.v1+json"},
			},
		},
	}

	result, err := resolveOCIGraph(
		context.Background(),
		"dc-tec",
		"openbao-operator",
		versions,
		client,
	)
	if err != nil {
		t.Fatalf("resolveOCIGraph() error = %v", err)
	}
	if result.Roots != 1 {
		t.Fatalf("roots = %d, want 1", result.Roots)
	}
	for _, digest := range []string{root, child, referrerIndex, artifact} {
		if _, ok := result.Reachable[digest]; !ok {
			t.Errorf("reachable set is missing %s", digest)
		}
	}
	for _, digest := range []string{orphan, detachedReferrerIndex} {
		if _, ok := result.Reachable[digest]; ok {
			t.Errorf("reachable set unexpectedly contains %s", digest)
		}
	}
	if got := strings.Join(client.calls, ","); got != root+","+referrerIndex {
		t.Fatalf("manifest calls = %q, want root then referrer index", got)
	}
}

func TestResolveOCIGraphFailsClosedOnManifestError(t *testing.T) {
	t.Parallel()

	root := testDigest("a")
	client := &fakeManifestGraphClient{errors: map[string]error{root: errors.New("registry unavailable")}}
	_, err := resolveOCIGraph(
		context.Background(),
		"dc-tec",
		"openbao-operator",
		[]packageVersion{{ID: 1, Name: root, Tags: []string{"edge"}}},
		client,
	)
	if err == nil || !strings.Contains(err.Error(), "registry unavailable") {
		t.Fatalf("resolveOCIGraph() error = %v, want registry failure", err)
	}
}

func TestSplitGraphTagsKeepsLegacyCosignTagAsRoot(t *testing.T) {
	t.Parallel()

	subject := testDigest("a")
	normalTags, subjects := splitGraphTags([]string{
		referrerTag(subject),
		referrerTag(subject) + ".sig",
		"edge",
	})
	if got := strings.Join(normalTags, ","); got != referrerTag(subject)+".sig,edge" {
		t.Fatalf("normal tags = %q", got)
	}
	if len(subjects) != 1 || subjects[0] != subject {
		t.Fatalf("subjects = %v", subjects)
	}
}

func TestGraphCandidateClassifiesReachableGraceAndOrphan(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 8, 3, 12, 0, 0, 0, time.UTC)
	reachableDigest := testDigest("a")
	reachable := map[string]struct{}{reachableDigest: {}}

	_, disposition := graphCandidate(
		packageVersion{ID: 1, Name: reachableDigest, UpdatedAt: now.AddDate(0, 0, -90)},
		nil,
		reachable,
		30,
		now,
	)
	if disposition != dispositionReachable {
		t.Fatalf("reachable disposition = %q, want reachable", disposition)
	}

	_, disposition = graphCandidate(
		packageVersion{ID: 2, Name: testDigest("b"), UpdatedAt: now.AddDate(0, 0, -29)},
		nil,
		reachable,
		30,
		now,
	)
	if disposition != dispositionGrace {
		t.Fatalf("young orphan disposition = %q, want grace", disposition)
	}

	candidate, disposition := graphCandidate(
		packageVersion{ID: 3, Name: testDigest("c"), UpdatedAt: now.AddDate(0, 0, -31)},
		nil,
		reachable,
		30,
		now,
	)
	if disposition != dispositionCandidate {
		t.Fatalf("old orphan disposition = %q, want candidate", disposition)
	}
	if candidate.Kind != candidateKindOCIOrphan || candidate.RequiredAgeDays != 30 {
		t.Fatalf("orphan candidate = %#v", candidate)
	}

	candidate, disposition = graphCandidate(
		packageVersion{
			ID:        5,
			Name:      testDigest("e"),
			UpdatedAt: now.AddDate(0, 0, -31),
			Tags:      []string{referrerTag(testDigest("f"))},
		},
		nil,
		reachable,
		30,
		now,
	)
	if disposition != dispositionCandidate || candidate.Kind != candidateKindOCIReferrerOrphan {
		t.Fatalf("detached referrer candidate = %#v, disposition = %q", candidate, disposition)
	}

	_, disposition = graphCandidate(
		packageVersion{ID: 4, Name: testDigest("d"), Tags: []string{"edge"}, UpdatedAt: now.AddDate(0, 0, -90)},
		[]string{"edge"},
		reachable,
		30,
		now,
	)
	if disposition != "" {
		t.Fatalf("normal tagged version disposition = %q, want policy evaluation", disposition)
	}
}

func TestRunHousekeepingPlansGraphOrphansWithinGlobalBudget(t *testing.T) {
	t.Parallel()

	_, rules := testPolicy(t)
	now := time.Date(2026, 8, 3, 12, 0, 0, 0, time.UTC)
	root := testDigest("a")
	child := testDigest("b")
	oldOrphan := testDigest("c")
	youngOrphan := testDigest("d")
	client := &fakePackageClient{versionsByPackage: map[string][]packageVersion{
		"openbao-operator": {
			{ID: 1, Name: root, UpdatedAt: now.AddDate(0, 0, -1), Tags: []string{"edge"}},
			{ID: 2, Name: child, UpdatedAt: now.AddDate(0, 0, -1)},
			{ID: 3, Name: oldOrphan, UpdatedAt: now.AddDate(0, 0, -40)},
			{ID: 4, Name: youngOrphan, UpdatedAt: now.AddDate(0, 0, -20)},
			{ID: 5, Name: testDigest("e"), UpdatedAt: now.AddDate(0, 0, -8), Tags: []string{"e2e-123-1"}},
		},
	}}
	graphClient := &fakeManifestGraphClient{references: map[string][]manifestReference{
		root: {
			{Digest: child, MediaType: "application/vnd.oci.image.manifest.v1+json"},
		},
	}}
	policy := policyConfig{
		ProtectUnknown: true,
		OCIGraph: ociGraphPolicy{
			Enabled:       true,
			OrphanTTLDays: 30,
		},
	}
	opts := options{
		Owner:               "dc-tec",
		OwnerKind:           ownerKindUser,
		Packages:            []string{"openbao-operator"},
		Mode:                modeDryRun,
		PolicyFile:          "test-policy.json",
		MaxDeletePerPackage: 100,
		MaxDeleteTotal:      1,
		ReportJSON:          "dist/report.json",
	}

	report, err := runHousekeeping(context.Background(), opts, policy, rules, client, graphClient, now)
	if err != nil {
		t.Fatalf("runHousekeeping() error = %v", err)
	}
	got := report.Packages[0]
	if got.Candidates != 2 || got.TaggedCandidates != 1 || got.OrphanCandidates != 1 {
		t.Fatalf(
			"candidate counts = total:%d tagged:%d orphan:%d",
			got.Candidates,
			got.TaggedCandidates,
			got.OrphanCandidates,
		)
	}
	if got.KeptGraphReachable != 1 || got.KeptOrphanGrace != 1 || got.KeptProtected != 1 {
		t.Fatalf(
			"retention counts = reachable:%d grace:%d protected:%d",
			got.KeptGraphReachable,
			got.KeptOrphanGrace,
			got.KeptProtected,
		)
	}
	if got.Planned != 1 {
		t.Fatalf("planned = %d, want 1", got.Planned)
	}
	for _, candidate := range got.CandidateItems {
		if candidate.ID == 3 && candidate.Planned {
			t.Fatalf("orphan was planned before a tagged candidate: %#v", candidate)
		}
		if candidate.ID == 5 && !candidate.Planned {
			t.Fatalf("tagged candidate was not planned first: %#v", candidate)
		}
	}
}

type fakeManifestGraphClient struct {
	references map[string][]manifestReference
	errors     map[string]error
	calls      []string
}

func (f *fakeManifestGraphClient) ManifestReferences(
	_ context.Context,
	_, _ string,
	digest string,
) ([]manifestReference, error) {
	f.calls = append(f.calls, digest)
	if err := f.errors[digest]; err != nil {
		return nil, err
	}
	return append([]manifestReference{}, f.references[digest]...), nil
}

func testDigest(character string) string {
	return "sha256:" + strings.Repeat(character, 64)
}

func referrerTag(digest string) string {
	return strings.Replace(digest, ":", "-", 1)
}
