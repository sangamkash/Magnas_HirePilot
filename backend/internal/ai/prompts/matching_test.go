package prompts

import "testing"

func TestDefaultMatchingPromptRendersVerifiedInputs(t *testing.T) {
	system, user, err := DefaultMatching().Render("profile facts", "job details")
	if err != nil {
		t.Fatal(err)
	}
	if system == "" || user == "" || !contains(user, "profile facts") || !contains(user, "job details") {
		t.Fatalf("prompt missing inputs: %q", user)
	}
}

func TestMatchingPromptRejectsMissingVerifiedInputsAndPlaceholders(t *testing.T) {
	p := DefaultMatching()
	for _, tc := range []struct{ profile, job, template string }{
		{"", "job details", p.UserTemplate}, {"profile facts", "  ", p.UserTemplate},
		{"profile facts", "job details", "Candidate: {{.Profile}}"},
	} {
		p.UserTemplate = tc.template
		if _, _, err := p.Render(tc.profile, tc.job); err == nil {
			t.Fatalf("accepted invalid prompt input/template: %+v", tc)
		}
	}
	p = DefaultMatching()
	p.UserTemplate = `{{if false}}{{.Profile}}{{end}}{{.Job}}`
	if _, _, err := p.Render("profile facts", "job details"); err == nil {
		t.Fatal("accepted a profile placeholder that does not render")
	}
}
func contains(s, part string) bool {
	for i := 0; i+len(part) <= len(s); i++ {
		if s[i:i+len(part)] == part {
			return true
		}
	}
	return false
}
