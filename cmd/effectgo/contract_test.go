package main

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

func TestContractCLI(t *testing.T) {
	bin := buildEffectgo(t)
	dir, err := filepath.Abs("testdata/integrate")
	if err != nil {
		t.Fatal(err)
	}
	t.Run("all analyzers and HTTP in one report", func(t *testing.T) {
		args := []string{"contract", "-format=json", "-coverage=json", "-fail-on=none", "./contractcase", "./gate"}
		stdout, stderr, code := runIn(t, bin, dir, args...)
		if code != 3 {
			t.Fatalf("native violations were suppressed: exit %d: %s", code, stderr)
		}
		var report contractReport
		if err := json.Unmarshal([]byte(stdout), &report); err != nil {
			t.Fatalf("not one report object: %v: %s", err, stdout)
		}
		if report.SchemaVersion != 1 || report.ExitCode != code || report.Coverage.Packages != 2 {
			t.Fatalf("invalid report contract: %+v", report)
		}
		seen := map[string]bool{}
		for _, d := range report.Diagnostics {
			seen[d.RuleID] = true
			if filepath.IsAbs(d.Position.File) || d.Position.Line < 1 || d.Position.Column < 0 || d.Severity == "error" && d.Position.Column == 0 {
				t.Errorf("unusable source location: %+v", d)
			}
			if d.RuleID == "effects" && (!strings.Contains(d.Message, "time.Now") || d.Severity != "error") {
				t.Errorf("lost native effect witness: %+v", d)
			}
			if strings.HasPrefix(d.RuleID, "http.") && (d.Route != "GET /domain" || d.Status != 500 || d.Via == "") {
				t.Errorf("lost HTTP evidence: %+v", d)
			}
		}
		for _, rule := range []string{"effects", "errset", "mustuse", "exhaustive", "schematag", "directive", "http.gin", "http.huma"} {
			if !seen[rule] {
				t.Errorf("missing %s violation in %s", rule, stdout)
			}
		}
		var coverage contractCoverage
		if err := json.Unmarshal([]byte(stderr), &coverage); err != nil {
			t.Fatalf("coverage stderr is not JSON: %v: %s", err, stderr)
		}
		if coverage.Gin.Routes != 2 || coverage.Gin.Unresolved != 1 || coverage.Gin.Limit == "" || coverage.Huma.Routes != 4 || coverage.Huma.Unresolved != 2 {
			t.Fatalf("coverage overclaims resolution: %+v", coverage)
		}
		unknownPath, unknownMapping := false, false
		for _, route := range coverage.Huma.UnresolvedRoute {
			unknownPath = unknownPath || strings.Contains(route.Reason, "constant")
			unknownMapping = unknownMapping || strings.Contains(route.Reason, "errdefault")
		}
		if !unknownPath || !unknownMapping {
			t.Errorf("missing path or native-mapper coverage limitation: %+v", coverage.Huma)
		}
		// Package-pattern order must not change the machine contract.
		args[len(args)-2], args[len(args)-1] = args[len(args)-1], args[len(args)-2]
		again, _, code := runIn(t, bin, dir, args...)
		if code != 3 || again != stdout {
			t.Fatalf("package order changed the report: exit %d\n%s\n%s", code, stdout, again)
		}
	})
	t.Run("HTTP gates and informational findings", func(t *testing.T) {
		for _, tc := range []struct {
			flags []string
			code  int
			info  bool
		}{
			{nil, 3, false},
			{[]string{"-fail-on=high"}, 0, false},
			{[]string{"-fail-on=none", "-fail-unresolved"}, 3, false},
			{[]string{"-fail-on=none", "-all"}, 0, true},
		} {
			args := append([]string{"contract", "-format=json"}, tc.flags...)
			stdout, stderr, code := runIn(t, bin, dir, append(args, "./httpcontract")...)
			if code != tc.code {
				t.Fatalf("%v: exit %d, want %d: %s", tc.flags, code, tc.code, stderr)
			}
			var report contractReport
			if err := json.Unmarshal([]byte(stdout), &report); err != nil || report.ExitCode != code {
				t.Fatalf("gate/report disagree: %s (%v)", stdout, err)
			}
			hasInfo := false
			for _, d := range report.Diagnostics {
				if d.Severity == "info" {
					hasInfo = true
				}
			}
			if hasInfo != tc.info {
				t.Errorf("informational policy %v: %s", tc.flags, stdout)
			}
		}
	})
	t.Run("text retains repair evidence", func(t *testing.T) {
		stdout, stderr, code := runIn(t, bin, dir, "contract", "./contractcase", "./gate")
		if code != 3 || !strings.Contains(stdout, "[effects]") || !strings.Contains(stdout, "time.Now") || !strings.Contains(stdout, "GET /domain") || !strings.Contains(stdout, "HTTP 500") {
			t.Fatalf("text lost repair evidence: exit %d\n%s\n%s", code, stdout, stderr)
		}
	})
	t.Run("clean production code excludes test faults", func(t *testing.T) {
		stdout, stderr, code := runIn(t, bin, scratchModule(t, false), "contract", "--format=json")
		var report contractReport
		if code != 0 || json.Unmarshal([]byte(stdout), &report) != nil || report.ExitCode != 0 || report.Coverage.Packages != 1 || report.Diagnostics == nil || len(report.Diagnostics) != 0 {
			t.Fatalf("clean module did not produce a clean array report: exit %d\n%s\n%s", code, stdout, stderr)
		}
	})
	t.Run("usage and load failures are not clean reports", func(t *testing.T) {
		for _, tc := range []struct {
			args []string
			code int
		}{
			{[]string{"contract", "-format=yaml", "./gate"}, 2},
			{[]string{"contract", "-fail-on=critical", "./gate"}, 2},
			{[]string{"contract", "./gate", "-format=json"}, 2},
			{[]string{"contract", "-format=json", "./missing-package"}, 1},
			{[]string{"contract", "-format=json", "./missing-pattern/..."}, 1},
			{[]string{"contract", "-format=json", "-stubs=missing.stubs", "./gate"}, 1},
		} {
			stdout, stderr, code := runIn(t, bin, dir, tc.args...)
			if code != tc.code || stdout != "" {
				t.Errorf("%v: exit %d, want %d without a success report; stdout=%s stderr=%s", tc.args, code, tc.code, stdout, stderr)
			}
		}
	})
}
