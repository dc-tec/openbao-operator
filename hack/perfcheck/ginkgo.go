package main

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"
)

type ginkgoSuiteReport struct {
	SpecReports []ginkgoSpecReport `json:"SpecReports"`
}

type ginkgoSpecReport struct {
	SpecEvents []ginkgoSpecEvent `json:"SpecEvents"`
}

type ginkgoSpecEvent struct {
	SpecEventType    string                 `json:"SpecEventType"`
	Message          string                 `json:"Message"`
	TimelineLocation ginkgoTimelineLocation `json:"TimelineLocation"`
}

type ginkgoTimelineLocation struct {
	Order int       `json:"Order"`
	Time  time.Time `json:"Time"`
}

func parseGinkgoPhaseEvents(path string) ([]phaseEvent, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var reports []ginkgoSuiteReport
	if err := json.Unmarshal(data, &reports); err != nil {
		return nil, fmt.Errorf("parse Ginkgo JSON report: %w", err)
	}

	type orderedPhase struct {
		phase phaseEvent
		order int
	}
	ordered := []orderedPhase{}
	for _, suite := range reports {
		for _, spec := range suite.SpecReports {
			for _, event := range spec.SpecEvents {
				if event.SpecEventType != "By" || strings.TrimSpace(event.Message) == "" {
					continue
				}
				at := event.TimelineLocation.Time
				if at.IsZero() {
					at = time.Now().UTC()
				}
				ordered = append(ordered, orderedPhase{
					phase: phaseEvent{
						Name:   phaseNameFromText("ginkgo_by", event.Message),
						At:     at.UTC(),
						Source: "ginkgo_by",
					},
					order: event.TimelineLocation.Order,
				})
			}
		}
	}
	sort.SliceStable(ordered, func(i, j int) bool {
		if !ordered[i].phase.At.Equal(ordered[j].phase.At) {
			return ordered[i].phase.At.Before(ordered[j].phase.At)
		}
		return ordered[i].order < ordered[j].order
	})

	seen := map[string]int{}
	phases := make([]phaseEvent, 0, len(ordered))
	for _, item := range ordered {
		name := item.phase.Name
		seen[name]++
		if seen[name] > 1 {
			item.phase.Name = fmt.Sprintf("%s_%d", name, seen[name])
		}
		phases = append(phases, item.phase)
	}
	return phases, nil
}

func phaseNameFromText(prefix string, text string) string {
	var builder strings.Builder
	lastUnderscore := false
	for _, r := range strings.ToLower(text) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			builder.WriteRune(r)
			lastUnderscore = false
		default:
			if !lastUnderscore {
				builder.WriteRune('_')
				lastUnderscore = true
			}
		}
	}
	slug := strings.Trim(builder.String(), "_")
	if slug == "" {
		slug = "event"
	}
	name := strings.Trim(strings.TrimSpace(prefix), "_")
	if name == "" {
		name = "phase"
	}
	out := name + "_" + slug
	if len(out) > 96 {
		out = strings.Trim(out[:96], "_")
	}
	return out
}
