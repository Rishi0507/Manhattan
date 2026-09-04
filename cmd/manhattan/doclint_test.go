package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// The documents are checked the way the settlements are.
//
// This repository's argument is that a figure nobody checked is a claim rather
// than a measurement, and it spent a long time applying that to settlement data
// while typing unchecked claims into its own README. Reviewers found, in order:
// a stale run's numbers throughout, "96 swept configurations" against "57", a
// TWIN_SWAP clause whose condition was inverted so it never rendered, a business
// case worth zero rupees a month, "two misconfigurations" above a list of five,
// one defect rate printed as both 4.0 and 6.0 per cent, and a negative rendered
// as "-,105,236".
//
// Every one of those is cheap to detect and none was detected, because nothing
// read the output. These tests read the output.
//
// They run against the committed documents, so a render that introduces one of
// these fails `go test ./...` rather than reaching a reviewer.

func docPaths(t *testing.T) map[string]string {
	t.Helper()
	root := filepath.Join("..", "..")
	out := map[string]string{}
	for _, name := range []string{"README.md", "LIMITATIONS.md", "RESULTS.md"} {
		b, err := os.ReadFile(filepath.Join(root, name))
		if err != nil {
			t.Skipf("%s not generated yet: %v", name, err)
		}
		// Normalise line endings.
		//
		// The working tree is CRLF on Windows, and every check in this file that
		// mentions a newline silently matched nothing until this line existed.
		// The tests passed, the documents were wrong, and a check that cannot
		// fail is worse than no check because it is mistaken for one.
		out[name] = strings.ReplaceAll(string(b), "\r\n", "\n")
	}
	return out
}

// TestNoHardcodedCountsContradictLists catches prose that states a quantity the
// generator owns.
//
// "Two misconfigurations are modelled" sat above a generated list of five,
// because a merchant was given a second condition and the sentence was not.
// The rule is simple: if a number word introduces a list, it has to match the
// list.
func TestNoHardcodedCountsContradictLists(t *testing.T) {
	words := map[string]int{
		"one": 1, "two": 2, "three": 3, "four": 4, "five": 5,
		"six": 6, "seven": 7, "eight": 8, "nine": 9, "ten": 10,
	}
	// A paragraph that contains a number word and ends in a colon, immediately
	// followed by a bulleted list.
	//
	// The word and the colon usually sit in different sentences of the same
	// paragraph, so the pattern spans sentence boundaries but not paragraph
	// ones. An earlier version excluded "." and therefore never matched the one
	// sentence it was written for, which is why this file probes its own checks.
	intro := regexp.MustCompile(`(?i)\b(one|two|three|four|five|six|seven|eight|nine|ten)\b[^\n]*:\n\n((?:- .+\n)+)`)
	// "every one", "each one", "no one" are determiners rather than counts.
	determiner := regexp.MustCompile(`(?i)\b(every|each|any|no|not|the only|this|that)\s+$`)

	for name, body := range docPaths(t) {
		for _, m := range intro.FindAllStringSubmatch(body, -1) {
			word := strings.ToLower(m[1])
			if before := strings.LastIndex(body, m[0]); before > 12 &&
				determiner.MatchString(body[before-12:before]) {
				continue
			}
			want := words[word]
			got := strings.Count(strings.TrimSpace(m[2]), "\n- ") + 1
			if want != got {
				t.Errorf("%s: prose says %q but the list that follows has %d entries.\n"+
					"    A count typed into a sentence goes stale the moment the generated list\n"+
					"    it introduces changes. Render the count instead.\n    list:\n%s",
					name, m[1], got, m[2])
			}
		}
	}
}

// TestOneQuantityOneLabel catches the same figure printed under one name with
// two values.
//
// The report defect rate appeared as 4.0 per cent in README and LIMITATIONS and
// 6.0 per cent in the RESULTS sensitivity table, because one was the realised
// rate and the other the configured rate and both were called "the defect
// rate". Both were true. Together they read as a documentation bug, which in a
// repository whose pitch is that its figures cannot drift is worse than either.
func TestOneQuantityOneLabel(t *testing.T) {
	docs := docPaths(t)
	// Only the phrase that names the modelled rate. "if fewer than X per cent of
	// reports are defective" is a break-even threshold, a different quantity,
	// and it is phrased differently on purpose.
	pct := regexp.MustCompile(`(?i)(?:configured |generated |report )defect rate of (?:about )?(\d+\.\d)\s*(?:per cent|%)`)

	seen := map[string][]string{}
	for name, body := range docs {
		for _, m := range pct.FindAllStringSubmatch(body, -1) {
			seen[m[1]] = append(seen[m[1]], name)
		}
	}
	if len(seen) > 1 {
		var lines []string
		for v, where := range seen {
			lines = append(lines, fmt.Sprintf("      %s%% in %s", v, strings.Join(where, ", ")))
		}
		t.Errorf("the report defect rate is printed with %d different values under one name:\n%s\n"+
			"    If two quantities are meant, name them apart: the CONFIGURED rate and the\n"+
			"    REALISED rate are different things and calling both \"the defect rate\" reads\n"+
			"    as drift.", len(seen), strings.Join(lines, "\n"))
	}
}

// TestNoMalformedNumbers catches formatting that produces an impossible figure.
//
// commas() grouped a negative from the right and published "-,105,236".
func TestNoMalformedNumbers(t *testing.T) {
	bad := []struct {
		pat  *regexp.Regexp
		what string
	}{
		{regexp.MustCompile(`-,\d`), "a separator immediately after a minus sign"},
		{regexp.MustCompile(`\d+,\d{1,2}[^\d,]`), "a thousands group shorter than three digits"},
		{regexp.MustCompile(`(?i)\bNaN\b|\b[+-]?Inf\b`), "a non-finite number"},
		{regexp.MustCompile(`%!\w`), "a Go format verb that did not match its argument"},
		{regexp.MustCompile(`<no value>`), "a template field that does not exist"},
	}
	for name, body := range docPaths(t) {
		for _, b := range bad {
			if m := b.pat.FindString(body); m != "" {
				t.Errorf("%s contains %s: %q", name, b.what, m)
			}
		}
	}
}

// TestProviderDependentFiguresAreAnnotated catches a table that quotes model
// activity or machine timings without saying what produced them.
//
// A compliance table printing "48,194 settlements per hour" and "1,498 model
// calls" with no annotation lets a reader form an impression the provider line
// two hundred lines later then contradicts. Disclosing it elsewhere is not the
// same as not misleading here.
func TestProviderDependentFiguresAreAnnotated(t *testing.T) {
	body := docPaths(t)["README.md"]
	start := strings.Index(body, "## Track compliance")
	if start < 0 {
		t.Skip("no compliance table in this render")
	}
	end := strings.Index(body[start+1:], "\n## ")
	if end < 0 {
		end = len(body) - start - 1
	}
	table := body[start : start+end]

	for _, c := range []struct {
		trigger string
		need    []string
		why     string
	}{
		{"model calls", []string{"stub", "offline provider", "claude-"},
			"a model-call count must name the provider that served it, in the same table"},
		{"settlements per hour", []string{"ms median"},
			"a throughput figure must carry what it was measured against"},
		{"settlements per hour", []string{"cores", "go1."},
			"a throughput figure must name the machine"},
	} {
		if !strings.Contains(table, c.trigger) {
			continue
		}
		lower := strings.ToLower(table)
		ok := false
		for _, n := range c.need {
			if strings.Contains(lower, strings.ToLower(n)) {
				ok = true
				break
			}
		}
		if !ok {
			t.Errorf("the compliance table quotes %q without any of %v. %s",
				c.trigger, c.need, c.why)
		}
	}
}

// TestNoEmDashes enforces the house style, which is a standing instruction
// rather than a preference.
func TestNoEmDashes(t *testing.T) {
	for name, body := range docPaths(t) {
		if i := strings.Index(body, "—"); i >= 0 {
			lo := i - 60
			if lo < 0 {
				lo = 0
			}
			t.Errorf("%s contains an em dash at offset %d: %q", name, i, body[lo:i+20])
		}
	}
}

// TestNoDanglingAnchors catches a heading that was renamed while a link to it
// was not.
func TestNoDanglingAnchors(t *testing.T) {
	body := docPaths(t)["README.md"]
	head := regexp.MustCompile(`(?m)^#{2,4} (.+)$`)
	strip := regexp.MustCompile(`[^a-z0-9 -]`)

	have := map[string]bool{}
	for _, m := range head.FindAllStringSubmatch(body, -1) {
		slug := strings.ReplaceAll(strip.ReplaceAllString(strings.ToLower(m[1]), ""), " ", "-")
		have[slug] = true
	}
	for _, m := range regexp.MustCompile(`\]\(#([a-z0-9-]+)\)`).FindAllStringSubmatch(body, -1) {
		if !have[m[1]] {
			t.Errorf("README links to #%s, which is not a heading in it", m[1])
		}
	}
}

// TestNoBrokenRelativeLinks catches a path that does not exist.
func TestNoBrokenRelativeLinks(t *testing.T) {
	root := filepath.Join("..", "..")
	link := regexp.MustCompile(`\]\(([^)#][^)]*?)\)`)
	for name, body := range docPaths(t) {
		for _, m := range link.FindAllStringSubmatch(body, -1) {
			target := m[1]
			if strings.HasPrefix(target, "http") {
				continue
			}
			if i := strings.Index(target, "#"); i >= 0 {
				target = target[:i]
			}
			if target == "" {
				continue
			}
			if _, err := os.Stat(filepath.Join(root, target)); err != nil {
				t.Errorf("%s links to %q, which does not exist", name, target)
			}
		}
	}
}
