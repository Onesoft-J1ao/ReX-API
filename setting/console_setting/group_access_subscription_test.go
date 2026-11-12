package console_setting

import "testing"

// The subscription condition gates a group on "this user holds one of these
// plans". Validation must accept the canonical type and the aliases, and must
// reject a condition that names no plan — otherwise the rule would silently
// deny the group to everybody.
func TestNormalizeConditionSubscription(t *testing.T) {
	for _, typ := range []string{"subscription", "has_subscription", "plan"} {
		got, err := normalizeCondition(GroupAccessCondition{
			Type:       typ,
			PlanTitles: []string{"  DeepSeek 50M  ", "", "DeepSeek 50M"},
		})
		if err != nil {
			t.Fatalf("%s: unexpected error: %v", typ, err)
		}
		if got.Type != "subscription" {
			t.Fatalf("%s: normalized type = %q, want subscription", typ, got.Type)
		}
		if len(got.PlanTitles) != 1 || got.PlanTitles[0] != "DeepSeek 50M" {
			t.Fatalf("%s: plan titles = %v, want one trimmed entry", typ, got.PlanTitles)
		}
	}
}

func TestNormalizeConditionSubscriptionRequiresTitle(t *testing.T) {
	if _, err := normalizeCondition(GroupAccessCondition{Type: "subscription"}); err == nil {
		t.Fatal("expected an error when no plan title is given")
	}
	if _, err := normalizeCondition(GroupAccessCondition{
		Type:       "subscription",
		PlanTitles: []string{"   ", ""},
	}); err == nil {
		t.Fatal("expected an error when every plan title is blank")
	}
}

// A rule carrying the new condition must survive the whole normalization path,
// which is what the settings form submits. A group that does not exist in
// GroupRatio is dropped by design, so the assertion is on the condition type
// being accepted rather than on the group surviving.
func TestNormalizeGroupAccessRulesWithSubscription(t *testing.T) {
	raw := `[{"group":"ds套餐","logic":"and","conditions":[{"type":"subscription","plan_titles":["DeepSeek 50M"]}]}]`
	if _, err := NormalizeGroupAccessRules(raw); err != nil {
		t.Fatalf("subscription rule rejected: %v", err)
	}
}
