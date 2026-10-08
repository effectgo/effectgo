package domain

import (
	"fmt"
	"runtime"
	"strings"
	"sync"
	"testing"
)

// The todo starts with title "init" and done=false. Writers are of two kinds:
//   - title writers call Update(id, title="x-<i>", nil)
//   - done writers call Update(id, nil, done=true) and never reset it.
//
// After all writers finish, a correct implementation must report done=true
// (only done writers set it, and nothing sets it back). A title writer that
// overwrites a stale done=false would show up as done=false: a lost update.
// Titles are always written as "x-<i>", so the final title must start with "x-".
// A stale write-back by a done writer also carries the old title ("init"),
// which shows up as a title lost update.

const (
	stressWriters = 200 // total goroutines per round, half of each kind
	stressRounds  = 20
)

func newTodo(t *testing.T, svc *Service) int {
	t.Helper()
	td, err := svc.Create("init")
	if err != nil {
		t.Fatal(err)
	}
	return td.ID
}

func boolPtr(b bool) *bool { return &b }

// TestUpdateRLostUpdateReproduced forces the interleaving deterministically:
// UpdateR reads the todo, pauses (test seam), a concurrent Update sets done=true,
// then UpdateR writes back its stale copy. This test asserts the defect is
// present. If UpdateR becomes atomic, this assertion must be inverted.
func TestUpdateRLostUpdateReproduced(t *testing.T) {
	svc := NewService()
	id := newTodo(t, svc)

	readDone := make(chan struct{})
	release := make(chan struct{})
	var once sync.Once
	updateRHook = func() {
		once.Do(func() {
			close(readDone)
			<-release
		})
	}
	defer func() { updateRHook = nil }()

	title := "new"
	finished := make(chan struct{})
	var rErr error
	go func() {
		defer close(finished)
		_, rErr = svc.UpdateR(id, &title, nil).Get()
	}()

	<-readDone
	if _, err := svc.Update(id, nil, boolPtr(true)); err != nil {
		t.Fatal(err)
	}
	close(release)
	<-finished
	if rErr != nil {
		t.Fatal(rErr)
	}

	got, err := svc.Get(id)
	if err != nil {
		t.Fatal(err)
	}
	if got.Title != "new" {
		t.Fatalf("title write lost: got %+v", got)
	}
	if got.Done {
		t.Fatalf("expected the known lost update (Done overwritten back to false), got %+v", got)
	}
}

type stressResult struct {
	doneLost  int // rounds ending with done=false (a done=true was overwritten)
	titleLost int // rounds ending with a title that is not an x-<i> title (the "init" title came back)
}

// stress runs stressRounds rounds of stressWriters concurrent writers using
// update, and counts rounds that lost a done=true or a title write.
func stress(t *testing.T, update func(svc *Service, id, i int) error) stressResult {
	t.Helper()
	var res stressResult
	for round := 0; round < stressRounds; round++ {
		svc := NewService()
		id := newTodo(t, svc)

		var wg sync.WaitGroup
		start := make(chan struct{})
		for i := 0; i < stressWriters; i++ {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				<-start
				if err := update(svc, id, i); err != nil {
					t.Errorf("writer %d: %v", i, err)
				}
			}(i)
		}
		close(start)
		wg.Wait()

		got, err := svc.Get(id)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.HasPrefix(got.Title, "x-") {
			res.titleLost++
		}
		if !got.Done {
			res.doneLost++
		}
	}
	return res
}

func titleOrDone(update func(svc *Service, id int, title *string, done *bool) error) func(*Service, int, int) error {
	return func(svc *Service, id, i int) error {
		if i%2 == 0 {
			title := fmt.Sprintf("x-%d", i)
			return update(svc, id, &title, nil)
		}
		return update(svc, id, nil, boolPtr(true))
	}
}

// TestUpdateConcurrentStress: the atomic Update must never lose a done=true.
func TestUpdateConcurrentStress(t *testing.T) {
	res := stress(t, titleOrDone(func(s *Service, id int, title *string, done *bool) error {
		_, err := s.Update(id, title, done)
		return err
	}))
	t.Logf("Update: done lost %d/%d rounds, title lost %d/%d rounds (writers=%d)",
		res.doneLost, stressRounds, res.titleLost, stressRounds, stressWriters)
	if res.doneLost != 0 || res.titleLost != 0 {
		t.Fatalf("Update lost updates: done %d/%d rounds, title %d/%d rounds", res.doneLost, stressRounds, res.titleLost, stressRounds)
	}
}

// TestUpdateRConcurrentStress reports (does not assert) how often UpdateR loses
// done=true under the same load, with and without the test seam that yields
// between read and write. The deterministic test above is the assertion.
func TestUpdateRConcurrentStress(t *testing.T) {
	run := func() stressResult {
		return stress(t, titleOrDone(func(s *Service, id int, title *string, done *bool) error {
			_, err := s.UpdateR(id, title, done).Get()
			return err
		}))
	}
	natural := run()
	updateRHook = runtime.Gosched
	widened := run()
	updateRHook = nil
	t.Logf("UpdateR without seam: done lost %d/%d rounds, title lost %d/%d rounds", natural.doneLost, stressRounds, natural.titleLost, stressRounds)
	t.Logf("UpdateR with runtime.Gosched seam: done lost %d/%d rounds, title lost %d/%d rounds", widened.doneLost, stressRounds, widened.titleLost, stressRounds)
}
