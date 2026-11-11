package model

import "testing"

// A disabled row with an exact rule must hide exactly that model.
func TestDisabledModelMatcherExactRule(t *testing.T) {
	matcher := newDisabledMatcherForTest([]Model{
		{ModelName: "naiwa/claude-opus-5-5", Status: 0, NameRule: NameRuleExact},
	})
	if matcher == nil {
		t.Fatal("matcher is nil with a disabled row")
	}
	if !matcher("naiwa/claude-opus-5-5") {
		t.Fatal("exact disabled model not matched")
	}
	if matcher("naiwa/claude-haiku-5-5") {
		t.Fatal("exact rule must not match a sibling name")
	}
}

// A disabled prefix rule covers every concrete name it matches — this is the
// case that kept eleven naiwa/* models on the health probe.
func TestDisabledModelMatcherPrefixRule(t *testing.T) {
	matcher := newDisabledMatcherForTest([]Model{
		{ModelName: "naiwa/", Status: 0, NameRule: NameRulePrefix},
	})
	for _, name := range []string{
		"naiwa/claude-fable-5-1",
		"naiwa/gpt-6-astra",
		"naiwa/sora-2-pro",
	} {
		if !matcher(name) {
			t.Fatalf("prefix rule did not disable %s", name)
		}
	}
	if matcher("grok-4.5") {
		t.Fatal("prefix rule must not match an unrelated model")
	}
}

func TestDisabledModelMatcherNoneDisabled(t *testing.T) {
	if matcher := newDisabledMatcherForTest(nil); matcher != nil {
		t.Fatal("expected a nil matcher when nothing is disabled")
	}
}

// Suffix and contains rules must be honoured too, because the catalog uses the
// same four rule types.
func TestDisabledModelMatcherOtherRules(t *testing.T) {
	matcher := newDisabledMatcherForTest([]Model{
		{ModelName: "-free", Status: 0, NameRule: NameRuleSuffix},
		{ModelName: "test-", Status: 0, NameRule: NameRuleContains},
	})
	if !matcher("glm-5.3-flash-free") {
		t.Fatal("suffix rule not applied")
	}
	if !matcher("my-test-model") {
		t.Fatal("contains rule not applied")
	}
	if matcher("gpt-6-astra") {
		t.Fatal("unrelated model marked disabled")
	}
}
