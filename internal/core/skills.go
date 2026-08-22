package core

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Syluxso/byzclaw/internal/ports"
)

// LoadSkillsDir reads skills/*/SKILL.md with YAML-ish frontmatter.
func LoadSkillsDir(dir string) ([]ports.Skill, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []ports.Skill
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		path := filepath.Join(dir, e.Name(), "SKILL.md")
		sk, err := ParseSkillFile(path)
		if err != nil {
			continue
		}
		if sk.ID == "" {
			sk.ID = e.Name()
		}
		out = append(out, sk)
	}
	return out, nil
}

func ParseSkillFile(path string) (ports.Skill, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return ports.Skill{}, err
	}
	return ParseSkillMarkdown(string(b))
}

func ParseSkillMarkdown(raw string) (ports.Skill, error) {
	raw = strings.ReplaceAll(raw, "\r\n", "\n")
	if !strings.HasPrefix(raw, "---\n") {
		return ports.Skill{Name: "untitled", Body: strings.TrimSpace(raw)}, nil
	}
	rest := strings.TrimPrefix(raw, "---\n")
	end := strings.Index(rest, "\n---\n")
	if end < 0 {
		return ports.Skill{}, fmt.Errorf("unclosed frontmatter")
	}
	fm := rest[:end]
	body := strings.TrimSpace(rest[end+len("\n---\n"):])
	sk := ports.Skill{Body: body}
	sc := bufio.NewScanner(strings.NewReader(fm))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, val, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		val = strings.TrimSpace(val)
		switch key {
		case "id":
			sk.ID = val
		case "name":
			sk.Name = val
		case "tools":
			val = strings.Trim(val, "[]")
			for _, p := range strings.Split(val, ",") {
				p = strings.TrimSpace(p)
				if p != "" {
					sk.Tools = append(sk.Tools, p)
				}
			}
		}
	}
	if sk.Name == "" {
		sk.Name = sk.ID
	}
	return sk, nil
}

// SkillsPrompt appends skill bodies for the system prompt.
func SkillsPrompt(skills []ports.Skill) string {
	if len(skills) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("\n# Skills\n")
	for _, s := range skills {
		b.WriteString("\n## ")
		b.WriteString(s.Name)
		b.WriteString("\n")
		b.WriteString(s.Body)
		b.WriteString("\n")
	}
	return b.String()
}

// FilterToolsBySkills intersects registered tools with union of skill allowlists.
// If no skill declares tools, all tools remain available.
func FilterToolsBySkills(all map[string]ports.Tool, skills []ports.Skill) map[string]ports.Tool {
	allowed := map[string]bool{}
	any := false
	for _, s := range skills {
		for _, t := range s.Tools {
			allowed[t] = true
			any = true
		}
	}
	if !any {
		return all
	}
	out := map[string]ports.Tool{}
	for name, t := range all {
		if allowed[name] {
			out[name] = t
		}
	}
	// Always keep core workspace tools if any skill mentioned them; if intersection empty, fall back to all.
	if len(out) == 0 {
		return all
	}
	return out
}
