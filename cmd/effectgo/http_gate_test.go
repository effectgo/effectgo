package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"path/filepath"
	"strings"
	"testing"

	ginhttp "github.com/effectgo/effectgo/adapters/gin"
)

func TestHTTPGateThresholds(t *testing.T) {
	counts := map[string]int{"high": 1, "medium": 1, "leak": 1, "info": 1}
	for _, tc := range []struct {
		threshold string
		want      int
	}{
		{"none", 0},
		{"high", httpGateExitCode},
		{"medium", httpGateExitCode},
		{"leak", httpGateExitCode},
		{"info", httpGateExitCode},
	} {
		t.Run(tc.threshold, func(t *testing.T) {
			got := gateHTTP(&httpOptions{FailOn: tc.threshold}, counts, 0)
			if got != tc.want {
				t.Fatalf("gateHTTP(%q) = %d, want %d", tc.threshold, got, tc.want)
			}
		})
	}
	if got := gateHTTP(&httpOptions{FailOn: "high"}, map[string]int{"medium": 1}, 0); got != 0 {
		t.Fatalf("high threshold failed for medium finding: %d", got)
	}
	if got := gateHTTP(&httpOptions{FailOn: "none", FailUnresolved: true}, nil, 1); got != httpGateExitCode {
		t.Fatalf("unresolved coverage gate = %d, want %d", got, httpGateExitCode)
	}
}

func TestHTTPOptionValuesRejectInvalidInput(t *testing.T) {
	fs := flag.NewFlagSet("http", flag.ContinueOnError)
	fs.SetOutput(bytes.NewBuffer(nil))
	bindHTTPOptions(fs)
	if err := fs.Parse([]string{"-fail-on=critical"}); err == nil {
		t.Fatal("invalid -fail-on value was accepted")
	}
	fs = flag.NewFlagSet("http", flag.ContinueOnError)
	fs.SetOutput(bytes.NewBuffer(nil))
	bindHTTPOptions(fs)
	if err := fs.Parse([]string{"-coverage=text"}); err == nil {
		t.Fatal("invalid -coverage value was accepted")
	}
}

func TestCoverageJSONKeepsFrameworkAndRouteCounts(t *testing.T) {
	run := &ginRun{
		Handlers: 1,
		Mappers:  []ginhttp.MapperInfo{{Name: "map"}},
		Routes: []ginhttp.Route{
			{Method: "GET", Path: "/ok", Pos: "routes.go:1"},
			{Method: "POST", PathUnresolved: "path is not a constant string", Pos: "routes.go:2"},
		},
		Unresolved: 1,
	}
	var buf bytes.Buffer
	if err := writeCoverageJSON(&buf, ginCoverage(run, map[string]int{"medium": 2})); err != nil {
		t.Fatal(err)
	}
	var got struct {
		Framework  string         `json:"framework"`
		Routes     int            `json:"routes"`
		Resolved   int            `json:"resolved"`
		Unresolved int            `json:"unresolved"`
		Findings   map[string]int `json:"findings"`
	}
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Framework != "gin" || got.Routes != 2 || got.Resolved != 1 || got.Unresolved != 1 || got.Findings["medium"] != 2 {
		t.Fatalf("coverage = %+v", got)
	}
}

func TestHTTPPoliciesCLI(t *testing.T) {
	bin := buildEffectgo(t)
	dir, err := filepath.Abs("testdata/integrate")
	if err != nil {
		t.Fatal(err)
	}
	for _, command := range []string{"http", "http-gin", "http-huma"} {
		t.Run(command, func(t *testing.T) {
			stdout, stderr, code := runIn(t, bin, dir, command, "-json", "-coverage=json", "./gate")
			if code != 0 {
				t.Fatalf("default exit %d: %s", code, stderr)
			}
			var coverage map[string]json.RawMessage
			if err := json.Unmarshal([]byte(stderr), &coverage); err != nil {
				t.Fatalf("coverage is not one JSON object: %v: %s", err, stderr)
			}
			if command == "http" {
				for _, framework := range []string{"gin", "huma"} {
					var c httpCoverage
					if err := json.Unmarshal(coverage[framework], &c); err != nil || c.Unresolved != 1 {
						t.Fatalf("%s coverage: %+v, %v", framework, c, err)
					}
					if framework == "gin" && (c.Routes != 2 || c.Resolved != 1 || c.Limit == "") {
						t.Fatalf("Gin variable registration was claimed as resolved: %+v", c)
					}
				}
			} else {
				var c httpCoverage
				if err := json.Unmarshal([]byte(stderr), &c); err != nil || c.Unresolved != 1 {
					t.Fatalf("coverage: %+v, %v", c, err)
				}
				if command == "http-gin" && (c.Routes != 2 || c.Resolved != 1 || c.Limit == "") {
					t.Fatalf("Gin variable registration was claimed as resolved: %+v", c)
				}
			}
			if strings.TrimSpace(stdout) == "" {
				t.Fatal("fixture produced no consumer findings")
			}
			for line := range strings.SplitSeq(strings.TrimSpace(stdout), "\n") {
				var finding map[string]any
				if err := json.Unmarshal([]byte(line), &finding); err != nil || len(finding) != 9 || finding["via"] == "" {
					t.Fatalf("legacy finding schema/witness changed: %s (%v)", line, err)
				}
			}
			for _, tc := range []struct {
				flags []string
				want  int
			}{
				{[]string{"-fail-on=none"}, 0},
				{[]string{"-fail-on=high"}, 0},
				{[]string{"-fail-on=medium"}, 3},
				{[]string{"-fail-on=leak"}, 3},
				{[]string{"-fail-on=info"}, 3},
				{[]string{"-fail-unresolved"}, 3},
				{[]string{"-fail-on=critical"}, 2},
				{[]string{"-coverage=text"}, 2},
				{[]string{"-fail-unresolved=maybe"}, 2},
			} {
				args := append([]string{command, "-json"}, tc.flags...)
				args = append(args, "./gate")
				_, stderr, code := runIn(t, bin, dir, args...)
				if code != tc.want {
					t.Errorf("%v: exit %d, want %d: %s", args, code, tc.want, stderr)
				}
			}
		})
	}
}

func TestHTTPGinPathCoverageCLI(t *testing.T) {
	bin := buildEffectgo(t)
	dir, err := filepath.Abs("testdata/integrate")
	if err != nil {
		t.Fatal(err)
	}
	for _, command := range []string{"http-gin", "http"} {
		for _, tc := range []struct {
			pattern                            string
			routes, resolved, unresolved, exit int
		}{
			{"./gatepaths/empty", 1, 1, 0, 0},
			{"./gatepaths", 3, 1, 2, 3},
		} {
			_, stderr, code := runIn(t, bin, dir, command, "-json", "-coverage=json", "-fail-unresolved", tc.pattern)
			if code != tc.exit {
				t.Errorf("%s %s exit %d, want %d: %s", command, tc.pattern, code, tc.exit, stderr)
			}
			data := []byte(stderr)
			if command == "http" {
				var union map[string]json.RawMessage
				if err := json.Unmarshal(data, &union); err != nil {
					t.Fatal(err)
				}
				data = union["gin"]
			}
			var coverage httpCoverage
			if err := json.Unmarshal(data, &coverage); err != nil {
				t.Fatalf("invalid coverage JSON: %v: %s", err, data)
			}
			if coverage.Routes != tc.routes || coverage.Resolved != tc.resolved || coverage.Unresolved != tc.unresolved {
				t.Errorf("%s %s coverage: %+v", command, tc.pattern, coverage)
			}
			for _, route := range coverage.UnresolvedRoute {
				if !strings.Contains(route.Reason, "prefix") {
					t.Errorf("dynamic group lacks explicit prefix reason: %+v", route)
				}
			}
		}
	}
}
