package llm

import "testing"

func TestDeepseekBuildRequest(t *testing.T) {
	cases := []struct {
		name     string
		payload  map[string]any
		expected map[string]any
	}{
		{
			name:    "effort high unchanged",
			payload: map[string]any{"thinking": map[string]string{"type": "enabled"}, "reasoning_effort": "high"},
			expected: map[string]any{"thinking": map[string]string{"type": "enabled"}, "reasoning_effort": "high"},
		},
		{
			name:    "effort max unchanged",
			payload: map[string]any{"thinking": map[string]string{"type": "enabled"}, "reasoning_effort": "max"},
			expected: map[string]any{"thinking": map[string]string{"type": "enabled"}, "reasoning_effort": "max"},
		},
		{
			name:    "effort low normalized to high",
			payload: map[string]any{"thinking": map[string]string{"type": "enabled"}, "reasoning_effort": "low"},
			expected: map[string]any{"thinking": map[string]string{"type": "enabled"}, "reasoning_effort": "high"},
		},
		{
			name:    "effort medium normalized to high",
			payload: map[string]any{"thinking": map[string]string{"type": "enabled"}, "reasoning_effort": "medium"},
			expected: map[string]any{"thinking": map[string]string{"type": "enabled"}, "reasoning_effort": "high"},
		},
		{
			name:    "effort xhigh normalized to max",
			payload: map[string]any{"thinking": map[string]string{"type": "enabled"}, "reasoning_effort": "xhigh"},
			expected: map[string]any{"thinking": map[string]string{"type": "enabled"}, "reasoning_effort": "max"},
		},
		{
			name:    "effort removed when thinking disabled",
			payload: map[string]any{"thinking": map[string]string{"type": "disabled"}, "reasoning_effort": "high"},
			expected: map[string]any{"thinking": map[string]string{"type": "disabled"}},
		},
		{
			name:    "no thinking no effort untouched",
			payload: map[string]any{"model": "deepseek-v4-flash"},
			expected: map[string]any{"model": "deepseek-v4-flash"},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := deepseekBuildRequest(c.payload)
			if got["reasoning_effort"] != c.expected["reasoning_effort"] {
				t.Errorf("reasoning_effort: got %v, want %v", got["reasoning_effort"], c.expected["reasoning_effort"])
			}
			_, gotHasEffort := got["reasoning_effort"]
			_, wantHasEffort := c.expected["reasoning_effort"]
			if gotHasEffort != wantHasEffort {
				t.Errorf("reasoning_effort presence: got %v, want %v", gotHasEffort, wantHasEffort)
			}
		})
	}
}
