package state

import (
	"sync"
)

// State holds global application state as a singleton.
type State struct {
	mu        sync.RWMutex
	homePath  string
	tenancies map[string]*string // name -> OCID
}

var (
	instance *State
	once     sync.Once
)

// GetInstance returns the singleton State instance.
func GetInstance() *State {
	once.Do(func() {
		instance = &State{
			tenancies: make(map[string]*string),
		}
	})
	return instance
}

// GetHomePath returns the configured home path.
func (s *State) GetHomePath() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.homePath
}

// SetHomePath sets the home path.
func (s *State) SetHomePath(path string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.homePath = path
}

// GetTenancyByName looks up a tenancy OCID by name.
func (s *State) GetTenancyByName(name string) (*string, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	ocid, ok := s.tenancies[name]
	return ocid, ok
}

// SetTenancy registers a tenancy name to OCID mapping.
func (s *State) SetTenancy(name string, ocid *string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tenancies[name] = ocid
}

// SetTenancies sets multiple tenancy mappings from a map.
func (s *State) SetTenancies(tenancies map[string]*string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for name, ocid := range tenancies {
		s.tenancies[name] = ocid
	}
}

// GetAllTenancies returns a copy of all tenancy mappings.
func (s *State) GetAllTenancies() map[string]*string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make(map[string]*string, len(s.tenancies))
	for k, v := range s.tenancies {
		result[k] = v
	}
	return result
}
