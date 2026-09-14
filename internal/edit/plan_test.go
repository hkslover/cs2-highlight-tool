package edit

import (
	"math"
	"strings"
	"testing"
)

func TestNormalizeTransitionsSupportsLegacyAndExplicitPlacement(t *testing.T) {
	legacy, err := NormalizeTransitions(3, []Transition{
		{Type: "fade", Duration: 0.3},
		{Type: "fade", Duration: 1},
	})
	if err != nil {
		t.Fatalf("legacy transitions: %v", err)
	}
	if legacy[0].AfterIndex != 0 || legacy[1].AfterIndex != 1 {
		t.Fatalf("legacy placement = %+v", legacy)
	}

	explicit, err := NormalizeTransitions(3, []Transition{{Type: "fade", Duration: 0.5, AfterIndex: 1}})
	if err != nil {
		t.Fatalf("explicit transitions: %v", err)
	}
	if explicit[1].Duration != 0.5 || len(explicit) != 1 {
		t.Fatalf("explicit placement = %+v", explicit)
	}
}

func TestResolveTrimRangeClampsOnlyProbeBoundary(t *testing.T) {
	end := 2.5006
	start, resolvedEnd, trimmed, err := ResolveTrimRange(Clip{EndSeconds: &end}, 2.5, 0)
	if err != nil || !trimmed || start != 0 || resolvedEnd != 2.5 {
		t.Fatalf("near-end range = %.3f..%.3f trimmed=%v err=%v", start, resolvedEnd, trimmed, err)
	}

	tooFar := 2.51
	if _, _, _, err := ResolveTrimRange(Clip{EndSeconds: &tooFar}, 2.5, 0); err == nil {
		t.Fatal("trim past boundary should fail")
	}
	if _, _, _, err := ResolveTrimRange(Clip{StartSeconds: ptr(1), EndSeconds: ptr(1)}, 2.5, 0); err == nil {
		t.Fatal("empty trim range should fail")
	}
}

func TestBuildTransitionFilterGraphPreservesSARAndMixesCuts(t *testing.T) {
	plan, err := BuildTransitionFilterGraph([]ResolvedClip{
		{Duration: 3.203, Width: 1440, Height: 1080, SampleAspectRatio: "4:3"},
		{Duration: 2.801, Width: 1280, Height: 720},
		{Duration: 4, Width: 1920, Height: 1080},
	}, map[int]Transition{0: {Type: "fade", Duration: 0.301}}, 60)
	if err != nil {
		t.Fatalf("build graph: %v", err)
	}
	if !strings.Contains(plan.Filter, "setsar=4/3") || !strings.Contains(plan.Filter, "xfade=transition=fade:duration=0.300000") {
		t.Fatalf("graph lost SAR/fade semantics: %s", plan.Filter)
	}
	if !strings.Contains(plan.Filter, "concat=n=2:v=1:a=1[v][a]") {
		t.Fatalf("graph lost hard-cut semantics: %s", plan.Filter)
	}
	if math.Abs(plan.TotalDuration-9.7) > 1e-9 {
		t.Fatalf("total duration=%f want 9.7", plan.TotalDuration)
	}
}

func TestResolveClipUsesAuthoritativeProbeFacts(t *testing.T) {
	start, end := 1.25, 3.75
	clip, err := ResolveClip(Clip{VideoPath: "clip.mp4", Duration: 99, StartSeconds: &start, EndSeconds: &end}, 99, &ProbedVideoInfo{
		Duration:           4.25,
		Width:              1600,
		Height:             900,
		SampleAspectRatio:  "1:1",
		DisplayAspectRatio: "16:9",
		HasAudio:           true,
		AudioKnown:         true,
	}, 0, true)
	if err != nil {
		t.Fatalf("resolve clip: %v", err)
	}
	if clip.Duration != 4.25 || clip.Width != 1600 || clip.TrimStart != start || clip.TrimEnd != end || !clip.HasTrim {
		t.Fatalf("resolved clip = %+v", clip)
	}
}

func TestClipSampleAspectRatioDerivesFromDAR(t *testing.T) {
	got := ClipSampleAspectRatio(ResolvedClip{Width: 1440, Height: 1080, DisplayAspectRatio: "16:9"})
	if got != "4/3" {
		t.Fatalf("derived SAR=%q want 4/3", got)
	}
}

func ptr(value float64) *float64 { return &value }
