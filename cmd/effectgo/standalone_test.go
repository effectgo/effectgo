package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime/pprof"
	"strings"
	"testing"

	"github.com/effectgo/effectgo/effects"
)

// The standalone run and go vet, end to end on a scratch module with a test
// file: the test variant is analyzed by default (-test), and only the
// standalone run prints the summary line (see docs/reference/cli.md).

// buildEffectgo builds this command into a temporary directory and returns the
// path of the binary.
func buildEffectgo(t *testing.T) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "effectgo")
	cmd := exec.Command("go", "build", "-o", bin, ".")
	cmd.Env = append(os.Environ(), "GOFLAGS=-buildvcs=false")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("building effectgo: %v\n%s", err, out)
	}
	return bin
}

// scratchModule writes a module whose test file has its own //effectgo:pure
// directive; the function it checks calls time.Now. When clean is true, the
// test function does not break its contract.
func scratchModule(t *testing.T, clean bool) string {
	t.Helper()
	dir := t.TempDir()
	write := func(name, body string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("go.mod", "module example.com/scratch\n\ngo 1.26.0\n")
	write("add.go", "package scratch\n\nfunc Add(a, b int) int { return a + b }\n")
	body := "package scratch\n\nimport (\n\t\"testing\"\n\t\"time\"\n)\n\n//effectgo:pure\nfunc TestClock(t *testing.T) { _ = time.Now() }\n"
	if clean {
		body = "package scratch\n\nimport \"testing\"\n\n//effectgo:pure\nfunc TestAdd(t *testing.T) { _ = Add(1, 2) }\n"
	}
	write("add_test.go", body)
	return dir
}

// runIn runs bin with args in dir and returns its stdout, stderr and exit code.
func runIn(t *testing.T, bin, dir string, args ...string) (string, string, int) {
	t.Helper()
	cmd := exec.Command(bin, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOFLAGS=-buildvcs=false", "EFFECTGO_STUBS=")
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	code := 0
	if ee, ok := err.(*exec.ExitError); ok {
		code = ee.ExitCode()
	} else if err != nil {
		t.Fatalf("running %s %v: %v", bin, args, err)
	}
	return stdout.String(), stderr.String(), code
}

// lastLine returns the last non-empty line of s.
func lastLine(s string) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	return lines[len(lines)-1]
}

// TestStandaloneAnalyzesTestFiles (B2): a test function with its own directive
// is reported by the standalone run by default, and skipped with -test=false.
func TestStandaloneAnalyzesTestFiles(t *testing.T) {
	bin := buildEffectgo(t)
	dir := scratchModule(t, false)

	_, stderr, code := runIn(t, bin, dir, "./...")
	if code != 3 {
		t.Fatalf("default run: exit %d, want 3; stderr:\n%s", code, stderr)
	}
	if !strings.Contains(stderr, "TestClock is declared //effectgo:pure but has effect time via TestClock") {
		t.Errorf("default run does not report the test function:\n%s", stderr)
	}

	_, stderr, code = runIn(t, bin, dir, "-test=false", "./...")
	if code != 0 {
		t.Fatalf("-test=false: exit %d, want 0; stderr:\n%s", code, stderr)
	}
	if strings.Contains(stderr, "TestClock") {
		t.Errorf("-test=false still reports the test function:\n%s", stderr)
	}
}

// TestSummaryOnStandalone (B3): the standalone run ends with the summary on stderr, with
// or without findings; go vet prints none; the summary does not look like a
// diagnostic.
func TestSummaryOnStandalone(t *testing.T) {
	bin := buildEffectgo(t)
	summary := regexp.MustCompile(`^effectgo: 1 package analysed \(1 test variant\), (\d+) findings?$`)

	clean := scratchModule(t, true)
	_, stderr, code := runIn(t, bin, clean, "./...")
	if code != 0 {
		t.Fatalf("clean run: exit %d; stderr:\n%s", code, stderr)
	}
	if m := summary.FindStringSubmatch(lastLine(stderr)); m == nil || m[1] != "0" {
		t.Errorf("clean run: last line %q, want the 0-findings summary", lastLine(stderr))
	}

	dirty := scratchModule(t, false)
	_, stderr, code = runIn(t, bin, dirty, "./...")
	if code != 3 {
		t.Fatalf("dirty run: exit %d, want 3", code)
	}
	if m := summary.FindStringSubmatch(lastLine(stderr)); m == nil || m[1] != "1" {
		t.Errorf("dirty run: last line %q, want the 1-finding summary", lastLine(stderr))
	}
	diag := regexp.MustCompile(`^\S+:\d+:\d+: `)
	if diag.MatchString(lastLine(stderr)) {
		t.Errorf("the summary matches the diagnostic shape: %q", lastLine(stderr))
	}

	// go vet: the same module, through the vettool, prints no summary.
	cmd := exec.Command("go", "vet", "-vettool="+bin, "./...")
	cmd.Dir = dirty
	cmd.Env = append(os.Environ(), "GOFLAGS=-buildvcs=false")
	var vetErr bytes.Buffer
	cmd.Stderr = &vetErr
	_ = cmd.Run()
	if strings.Contains(vetErr.String(), "effectgo: ") {
		t.Errorf("go vet printed the summary:\n%s", vetErr.String())
	}
	if !strings.Contains(vetErr.String(), "TestClock is declared") {
		t.Errorf("go vet did not report the test function:\n%s", vetErr.String())
	}
}

// TestStandaloneErrorsAsType exercises the real source-to-contract path, with
// errors.As as the compatibility baseline and no fixture stubs.
func TestStandaloneErrorsAsType(t *testing.T) {
	bin := buildEffectgo(t)
	for _, tc := range []struct {
		name     string
		code     int
		findings string
		violated []string
	}{
		{name: "positive", code: 0, findings: "0"},
		{name: "negative", code: 3, findings: "2", violated: []string{"Mismatch", "NonStdlib"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir, err := filepath.Abs(filepath.Join("testdata", "astype", tc.name))
			if err != nil {
				t.Fatal(err)
			}
			stdout, stderr, code := runIn(t, bin, dir, "./...")
			if stdout != "" || code != tc.code {
				t.Fatalf("standalone run: exit %d, stdout %q, want exit %d and no stdout\n%s", code, stdout, tc.code, stderr)
			}
			summary := regexp.MustCompile(`^effectgo: \d+ packages? analysed \(\d+ test variants?\), ` + tc.findings + ` findings$`)
			if !summary.MatchString(lastLine(stderr)) {
				t.Errorf("last line %q, want the %s-findings summary\n%s", lastLine(stderr), tc.findings, stderr)
			}
			const contractViolation = ", not in its //effectgo:errors contract"
			if count := strings.Count(stderr, contractViolation); count != len(tc.violated) {
				t.Errorf("got %d contract violations, want %d\n%s", count, len(tc.violated), stderr)
			}
			for _, name := range tc.violated {
				diagnostic := regexp.MustCompile(regexp.QuoteMeta(name+" may return astypenegative.BusyError") + `(?: \(via [^\n]+\))?` + regexp.QuoteMeta(contractViolation))
				if count := len(diagnostic.FindAllString(stderr, -1)); count != 1 {
					t.Errorf("%s has %d BusyError contract violations, want exactly one\n%s", name, count, stderr)
				}
			}
		})
	}
}

func TestStandaloneProfiles(t *testing.T) {
	bin := buildEffectgo(t)
	dir := scratchModule(t, true)
	if err := os.WriteFile(filepath.Join(dir, "add.go"), []byte("package scratch\n\n//effectgo:pure\nfunc Fail() { panic(\"failure\") }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name string
		args []string
		code int
	}{
		{"findings", []string{"./..."}, 3},
		{"json", []string{"-json", "./..."}, 0},
		{"load error", []string{"./missing"}, 1},
		{"stub error", []string{"-stubs=missing.stubs", "./..."}, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			profiles := t.TempDir()
			cpu := filepath.Join(profiles, "cpu.pprof")
			heap := filepath.Join(profiles, "heap.pprof")
			args := append([]string{"-test=false", "-cpuprofile", cpu, "-memprofile", heap}, tc.args...)
			stdout, stderr, code := runIn(t, bin, dir, args...)
			if code != tc.code {
				t.Fatalf("exit %d, want %d; stderr:\n%s", code, tc.code, stderr)
			}
			if tc.name == "json" {
				if !json.Valid([]byte(stdout)) || !strings.Contains(stderr, "1 finding") {
					t.Fatalf("profiling changed native JSON or findings: stdout %q, stderr %q", stdout, stderr)
				}
			} else if stdout != "" {
				t.Errorf("unexpected stdout: %q", stdout)
			}
			for _, profile := range []string{cpu, heap} {
				cmd := exec.CommandContext(t.Context(), "go", "tool", "pprof", "-top", "-symbolize=none", bin, profile)
				if out, err := cmd.CombinedOutput(); err != nil {
					t.Errorf("reading %s: %v\n%s", profile, err, out)
				}
			}
		})
	}

	for _, profileFlag := range []string{"-cpuprofile", "-memprofile"} {
		t.Run(profileFlag+" create error", func(t *testing.T) {
			path := t.TempDir() // A directory cannot be opened as a profile file.
			stdout, stderr, code := runIn(t, bin, dir, profileFlag, path, "-test=false", "./...")
			if code != 1 || stdout != "" || !strings.Contains(stderr, path) || strings.Contains(stderr, "analysed") {
				t.Fatalf("exit %d, stdout %q, stderr %q; want profile creation error before analysis", code, stdout, stderr)
			}
		})
		if _, err := os.Stat("/dev/full"); err == nil {
			t.Run(profileFlag+" write error", func(t *testing.T) {
				stdout, stderr, code := runIn(t, bin, dir, profileFlag, "/dev/full", "-test=false", "./...")
				if code != 1 || stdout != "" || !strings.Contains(stderr, "profile") || !strings.Contains(stderr, "no space left on device") {
					t.Fatalf("exit %d, stdout %q, stderr %q; want profile write error", code, stdout, stderr)
				}
			})
		}
	}
}

func TestStandaloneHelp(t *testing.T) {
	bin := buildEffectgo(t)
	dir := t.TempDir()
	for _, help := range []string{"-h", "-help", "--help"} {
		stdout, stderr, code := runIn(t, bin, dir, help)
		if code != 0 || stdout != "" {
			t.Fatalf("%s: exit %d, stdout %q, stderr %q", help, code, stdout, stderr)
		}
		compareFlagSet(t, help, usageFlags(stderr), standaloneFlagSet())
		for _, name := range []string{"cpuprofile", "memprofile", "test", "json", "c"} {
			if _, ok := usageFlags(stderr)[name]; !ok {
				t.Errorf("%s omits -%s:\n%s", help, name, stderr)
			}
		}
	}
	stdout, stderr, code := runIn(t, bin, dir, "help", "effects")
	if code != 0 || !strings.Contains(stdout+stderr, effects.Analyzer.Doc) {
		t.Fatalf("analyzer help: exit %d, stdout %q, stderr %q", code, stdout, stderr)
	}
}

func TestStandaloneCPUProfileStartError(t *testing.T) {
	if err := pprof.StartCPUProfile(io.Discard); err != nil {
		t.Fatal(err)
	}
	defer pprof.StopCPUProfile()

	previousFlags, previousStderr := flag.CommandLine, os.Stderr
	previousTests, previousJSON, previousContext := includeTests, jsonOut, contextLines
	previousCPU, previousMem := cpuProfile, memProfile
	stderr, err := os.CreateTemp(t.TempDir(), "stderr")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		flag.CommandLine, os.Stderr = previousFlags, previousStderr
		includeTests, jsonOut, contextLines = previousTests, previousJSON, previousContext
		cpuProfile, memProfile = previousCPU, previousMem
		if err := stderr.Close(); err != nil {
			t.Error(err)
		}
	})
	flag.CommandLine = flag.NewFlagSet("effectgo", flag.ContinueOnError)
	os.Stderr = stderr
	path := filepath.Join(t.TempDir(), "cpu.pprof")
	code := runStandalone([]string{"effectgo", "-cpuprofile", path, "./missing"})
	out, err := os.ReadFile(stderr.Name())
	if err != nil {
		t.Fatal(err)
	}
	if code != 1 || !strings.Contains(string(out), "starting CPU profile") || !strings.Contains(string(out), "already in use") {
		t.Fatalf("exit %d, stderr %q; want CPU profile startup error", code, out)
	}
}
