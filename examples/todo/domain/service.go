package domain

import "github.com/faintghost/effectgo/examples/todo/result"

// Service is the todo API over a Store.
type Service struct {
	store *Store
}

// NewService returns a service with an empty store.
func NewService() *Service {
	return &Service{store: NewStore()}
}

// ---- idiomatic flavor ----

// Create adds a todo with the given title.
func (s *Service) Create(title string) (Todo, error) {
	t, err := checkTitle(title)
	if err != nil {
		return Todo{}, err
	}
	return s.store.insert(t), nil
}

// Get returns the todo with the given id.
func (s *Service) Get(id int) (Todo, error) {
	t, ok := s.store.get(id)
	if !ok {
		return Todo{}, ErrNotFound
	}
	return t, nil
}

// List returns all todos ordered by id. The result is never nil.
func (s *Service) List() []Todo {
	return s.store.list()
}

// Update changes the title and/or done flag of a todo. Nil fields are left
// unchanged. The read-modify-write is atomic.
func (s *Service) Update(id int, title *string, done *bool) (Todo, error) {
	var newTitle *string
	if title != nil {
		t, err := checkTitle(*title)
		if err != nil {
			return Todo{}, err
		}
		if err := s.store.checkTitle(id, t); err != nil {
			return Todo{}, err
		}
		newTitle = &t
	}
	return s.store.modify(id, func(cur *Todo) error {
		if newTitle != nil {
			cur.Title = *newTitle
		}
		if done != nil {
			cur.Done = *done
		}
		return nil
	})
}

// Delete removes the todo with the given id.
func (s *Service) Delete(id int) error {
	if !s.store.delete(id) {
		return ErrNotFound
	}
	return nil
}

// ---- Result flavor ----

// CreateR is Create returning a Result.
func (s *Service) CreateR(title string) result.Result[Todo] {
	return result.AndThen(result.From(checkTitle(title)), func(t string) result.Result[Todo] {
		return result.Ok(s.store.insert(t))
	})
}

// GetR is Get returning a Result.
func (s *Service) GetR(id int) result.Result[Todo] {
	t, ok := s.store.get(id)
	if !ok {
		return result.Err[Todo](ErrNotFound)
	}
	return result.Ok(t)
}

// updateRHook is a test-only seam, nil in production. When set, UpdateR calls
// it after reading the todo and before writing it back, which widens the
// window in which a concurrent write can be lost. See concurrency_test.go.
var updateRHook func()

// UpdateR is Update composed from GetR and a validate-then-apply step.
// Note: GetR and the write are two separate lock acquisitions, so this
// composition is not atomic (see NOTES.md).
func (s *Service) UpdateR(id int, title *string, done *bool) result.Result[Todo] {
	return result.AndThen(s.GetR(id), func(cur Todo) result.Result[Todo] {
		if title != nil {
			t, err := checkTitle(*title)
			if err != nil {
				return result.Err[Todo](err)
			}
			cur.Title = t
		}
		if done != nil {
			cur.Done = *done
		}
		if updateRHook != nil {
			updateRHook()
		}
		if !s.store.replace(cur) {
			return result.Err[Todo](ErrNotFound)
		}
		return result.Ok(cur)
	})
}

// DeleteR is Delete returning Result[struct{}].
func (s *Service) DeleteR(id int) result.Result[struct{}] {
	return result.AndThen(s.GetR(id), func(Todo) result.Result[struct{}] {
		s.store.delete(id)
		return result.Ok(struct{}{})
	})
}

// ListR is List returning a Result. It cannot fail.
func (s *Service) ListR() result.Result[[]Todo] {
	return result.Ok(s.List())
}
