package main

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
)

const (
	mediaTypeOCIImageIndex    = "application/vnd.oci.image.index.v1+json"
	mediaTypeDockerImageIndex = "application/vnd.docker.distribution.manifest.list.v2+json"
	ociGraphRuleName          = "oci-graph-orphan"
	dispositionNone           = graphDisposition("")
	dispositionReachable      = graphDisposition("reachable")
	dispositionGrace          = graphDisposition("grace")
	dispositionCandidate      = graphDisposition("candidate")
)

var ociReferrerTagRegexp = regexp.MustCompile(`^sha256-([0-9a-f]{64})$`)

type manifestGraphClient interface {
	ManifestReferences(ctx context.Context, owner, pkg, digest string) ([]manifestReference, error)
}

type manifestReference struct {
	Digest    string
	MediaType string
}

type ociGraphResult struct {
	Reachable map[string]struct{}
	Roots     int
}

type graphDisposition string

func resolveOCIGraph(
	ctx context.Context,
	owner, pkg string,
	versions []packageVersion,
	client manifestGraphClient,
) (ociGraphResult, error) {
	result := ociGraphResult{Reachable: make(map[string]struct{})}
	if client == nil {
		return result, errors.New("OCI graph client is required")
	}

	referrerBySubject := make(map[string]packageVersion)
	roots := make([]packageVersion, 0)
	for _, version := range versions {
		normalTags, subjects := splitGraphTags(version.Tags)
		if len(normalTags) > 0 {
			roots = append(roots, version)
		}
		for _, subject := range subjects {
			if existing, ok := referrerBySubject[subject]; ok && existing.ID != version.ID {
				return result, fmt.Errorf(
					"subject %s has multiple OCI referrer indexes (%d and %d)",
					subject,
					existing.ID,
					version.ID,
				)
			}
			referrerBySubject[subject] = version
		}
	}
	result.Roots = len(roots)

	queue := make([]string, 0, len(roots))
	queued := make(map[string]struct{})
	fetched := make(map[string]struct{})

	var markReachable func(digest string, fetch bool)
	markReachable = func(digest string, fetch bool) {
		digest = strings.TrimSpace(digest)
		if digest == "" {
			return
		}
		result.Reachable[digest] = struct{}{}
		if fetch {
			if _, ok := queued[digest]; !ok {
				queued[digest] = struct{}{}
				queue = append(queue, digest)
			}
		}
		if referrer, ok := referrerBySubject[digest]; ok {
			if _, reachable := result.Reachable[referrer.Name]; !reachable {
				markReachable(referrer.Name, true)
			}
		}
	}

	for _, root := range roots {
		markReachable(root.Name, true)
	}

	for len(queue) > 0 {
		digest := queue[0]
		queue = queue[1:]
		if _, ok := fetched[digest]; ok {
			continue
		}
		fetched[digest] = struct{}{}

		references, err := client.ManifestReferences(ctx, owner, pkg, digest)
		if err != nil {
			return result, fmt.Errorf("resolve manifest %s: %w", digest, err)
		}
		for _, reference := range references {
			shouldFetch := reference.MediaType == "" || isImageIndexMediaType(reference.MediaType)
			markReachable(reference.Digest, shouldFetch)
		}
	}

	return result, nil
}

func splitGraphTags(tags []string) ([]string, []string) {
	normalTags := make([]string, 0, len(tags))
	subjects := make([]string, 0, 1)
	for _, tag := range tags {
		match := ociReferrerTagRegexp.FindStringSubmatch(tag)
		if len(match) == 2 {
			subjects = append(subjects, "sha256:"+match[1])
			continue
		}
		normalTags = append(normalTags, tag)
	}
	return normalTags, subjects
}

func isImageIndexMediaType(mediaType string) bool {
	return mediaType == mediaTypeOCIImageIndex || mediaType == mediaTypeDockerImageIndex
}

func graphCandidate(
	version packageVersion,
	normalTags []string,
	reachable map[string]struct{},
	orphanTTLDays int,
	now time.Time,
) (candidateReport, graphDisposition) {
	isGraphManaged := len(version.Tags) == 0 || len(normalTags) == 0
	if !isGraphManaged {
		return candidateReport{}, dispositionNone
	}
	if _, ok := reachable[version.Name]; ok {
		return candidateReport{}, dispositionReachable
	}

	ageDays := versionAgeDays(version, now)
	if ageDays < orphanTTLDays {
		return candidateReport{}, dispositionGrace
	}

	kind := candidateKindOCIOrphan
	if len(version.Tags) > 0 {
		kind = candidateKindOCIReferrerOrphan
	}
	return candidateReport{
		ID:              version.ID,
		Name:            version.Name,
		Kind:            kind,
		UpdatedAt:       version.UpdatedAt.UTC().Format(time.RFC3339),
		AgeDays:         ageDays,
		RequiredAgeDays: orphanTTLDays,
		Tags:            append([]string{}, version.Tags...),
		MatchedRules:    []string{ociGraphRuleName},
	}, dispositionCandidate
}

func versionAgeDays(version packageVersion, now time.Time) int {
	ageDays := int(now.Sub(version.UpdatedAt).Hours() / 24)
	if ageDays < 0 {
		return 0
	}
	return ageDays
}
