package projects

import (
	"errors"
	"sort"
	"sync"
)

var ErrNotFound = errors.New("project not found")
var ErrAlreadyExists = errors.New("project already exists")

type Project struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type Store interface {
	Create(Project) error
	List() []Project
	Delete(id string) error
}

// MemoryStore is a prototype store; records are lost when the API process stops.
type MemoryStore struct {
	mu       sync.RWMutex
	projects map[string]Project
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{projects: make(map[string]Project)}
}

func (s *MemoryStore) Create(project Project) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.projects[project.ID]; exists {
		return ErrAlreadyExists
	}
	s.projects[project.ID] = project
	return nil
}

func (s *MemoryStore) List() []Project {
	s.mu.RLock()
	defer s.mu.RUnlock()
	ids := make([]string, 0, len(s.projects))
	for id := range s.projects {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	result := make([]Project, 0, len(ids))
	for _, id := range ids {
		result = append(result, s.projects[id])
	}
	return result
}

func (s *MemoryStore) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.projects[id]; !exists {
		return ErrNotFound
	}
	delete(s.projects, id)
	return nil
}
