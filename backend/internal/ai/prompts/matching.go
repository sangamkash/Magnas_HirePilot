package prompts

import (
	"bytes"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"text/template"
	"text/template/parse"
)

const MatchingVersion = "job-match-v1"

const matchingSystem = `You assess how well a verified candidate profile matches a job description. Evaluate skills, experience, responsibilities, role, seniority, education, certifications, location, work mode, salary, and eligibility where information exists. Treat missing information as unknown, never as a mismatch. Do not invent candidate facts. Determine the overall score yourself; do not apply a fixed weighted formula. Return only JSON matching the requested structure.`

const matchingUser = `Evaluate this candidate profile against this job description.

Candidate profile:
{{.Profile}}

Job:
{{.Job}}

Return JSON with match_score (0-100), summary, strengths (string array), gaps (string array), score_breakdown containing technical_skills, experience, role_alignment, location, salary, and eligibility. Each dimension must have score (0-100 or null when unknown) and reason; technical_skills may also include matched and missing string arrays. Include explanation as an object with experience_analysis, role_alignment, location_analysis, salary_analysis, eligibility_analysis; unknowns must be explicit. recommendation must be HIGHLY_RECOMMENDED, RECOMMENDED, CONSIDER, NOT_RECOMMENDED, or UNKNOWN.`

type MatchingPrompts struct {
	Version      string
	System       string
	UserTemplate string
}

func DefaultMatching() MatchingPrompts {
	return MatchingPrompts{Version: MatchingVersion, System: matchingSystem, UserTemplate: matchingUser}
}

func (p MatchingPrompts) Render(profile, job string) (string, string, error) {
	if p.Version == "" || p.System == "" || p.UserTemplate == "" {
		return "", "", errors.New("matching prompt version, system prompt, and user template are required")
	}
	if strings.TrimSpace(profile) == "" || strings.TrimSpace(job) == "" {
		return "", "", errors.New("matching prompt requires non-empty profile and job inputs")
	}
	t, err := template.New("matching").Option("missingkey=error").Parse(p.UserTemplate)
	if err != nil {
		return "", "", fmt.Errorf("parse matching prompt: %w", err)
	}
	fields := map[string]bool{}
	for _, node := range []parse.Node{t.Tree.Root} {
		walkTemplateNode(node, fields)
	}
	if !fields["Profile"] || !fields["Job"] {
		return "", "", errors.New("matching prompt template must include Profile and Job")
	}
	var out bytes.Buffer
	if err = t.Execute(&out, map[string]string{"Profile": profile, "Job": job}); err != nil {
		return "", "", fmt.Errorf("render matching prompt: %w", err)
	}
	if !strings.Contains(out.String(), profile) || !strings.Contains(out.String(), job) {
		return "", "", errors.New("rendered matching prompt must contain the verified profile and job inputs")
	}
	return p.System, out.String(), nil
}

func walkTemplateNode(node parse.Node, fields map[string]bool) {
	if node == nil || (reflect.ValueOf(node).Kind() == reflect.Ptr && reflect.ValueOf(node).IsNil()) {
		return
	}
	switch n := node.(type) {
	case *parse.ListNode:
		for _, child := range n.Nodes {
			walkTemplateNode(child, fields)
		}
	case *parse.ActionNode:
		walkTemplateNode(n.Pipe, fields)
	case *parse.PipeNode:
		for _, command := range n.Cmds {
			walkTemplateNode(command, fields)
		}
	case *parse.CommandNode:
		for _, arg := range n.Args {
			walkTemplateNode(arg, fields)
		}
	case *parse.FieldNode:
		for _, ident := range n.Ident {
			fields[ident] = true
		}
	case *parse.IfNode:
		walkTemplateNode(n.Pipe, fields)
		walkTemplateNode(n.List, fields)
		walkTemplateNode(n.ElseList, fields)
	case *parse.RangeNode:
		walkTemplateNode(n.Pipe, fields)
		walkTemplateNode(n.List, fields)
		walkTemplateNode(n.ElseList, fields)
	case *parse.WithNode:
		walkTemplateNode(n.Pipe, fields)
		walkTemplateNode(n.List, fields)
		walkTemplateNode(n.ElseList, fields)
	}
}
