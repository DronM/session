// Package session provides support for sessions in web applications.
// Session providers are implemented as specific packages.
package session

import (
	"fmt"
	"math/rand"
	"sync"
	"time"
)

// Session interface for session functionality.
type Session interface {
	Set(key string, value interface{}) error //set session value
	Get(key string) interface{}              //get session value
	GetBool(key string) bool                 //get bool session value
	GetString(key string) string             //get string session value
	GetInt(key string) int64                 //get int64 session value
	GetFloat(key string) float64             //get float64 session value
	Delete(key string) error                 //delete session value
	SessionID() string                       //back current sessionID
	Flush() error
	TimeCreated() time.Time
}

// Provider interface for session provider.
type Provider interface {
	InitProvider(provParams []interface{}) error
	SessionInit(sid string) (Session, error)
	SessionRead(sid string) (Session, error)
	SessionDestroy(sid string) error
	SessionClose(sid string) error
	SessionGC(maxLifeTime int64, maxIdleTime int64)
	GetSessionIDLen() int
}

var provides = make(map[string]Provider)

// Register makes a session provider available by the provided name.
// If Register is called twice with the same name or if driver is nil,
// it panics.
func Register(name string, provide Provider) {
	if provide == nil {
		panic("session: Register provide is nil")
	}
	if _, dup := provides[name]; dup {
		panic("session: Register called twice for provide " + name)
	}
	provides[name] = provide
}

// Manager structure for holding provider.
type Manager struct {
	lock        sync.Mutex
	provider    Provider
	maxLifeTime int64
	maxIdleTime int64
}

// NewManager is a Manager create function.
func NewManager(provideName string, maxLifeTime int64, maxIdleTime int64, provParams ...interface{}) (*Manager, error) {
	provider, ok := provides[provideName]
	if !ok {
		return nil, fmt.Errorf("session: unknown provide %q (forgotten import?)", provideName)
	}
	manager := &Manager{provider: provider, maxLifeTime: maxLifeTime, maxIdleTime: maxIdleTime}
	if len(provParams) > 0 {
		er := manager.provider.InitProvider(provParams)
		if er != nil {
			return nil, er
		}
	}
	return manager, nil
}

// GetSessionIDLen returns session ID length specific for provider.
func (manager *Manager) GetSessionIDLen() int {
	return manager.provider.GetSessionIDLen()
}

// SessionStart opens session with the given ID.
func (manager *Manager) SessionStart(sid string) (Session, error) {
	manager.lock.Lock()
	defer manager.lock.Unlock()

	if sid == "" {
		sid := manager.genSessionID()
		return manager.provider.SessionInit(sid)
	}
	return manager.provider.SessionRead(sid)
}

// SessionClose closes session with the given ID.
func (manager *Manager) SessionClose(sid string) error {
	manager.lock.Lock()
	defer manager.lock.Unlock()
	if sid != "" {
		return manager.provider.SessionClose(sid)
	}
	return nil
}

// InitProvider initializes provider with its specific parameters.
// Should consult specific provider package to know its parameters.
func (manager *Manager) InitProvider(provParams []interface{}) error {
	return manager.provider.InitProvider(provParams)
}

// SessionDestroy destroys session by its ID.
func (manager *Manager) SessionDestroy(sid string) {
	if sid == "" {
		return
	} else {
		manager.lock.Lock()
		defer manager.lock.Unlock()
		manager.provider.SessionDestroy(sid)
	}
}

func (manager *Manager) GC() {
	manager.lock.Lock()
	defer manager.lock.Unlock()
	manager.provider.SessionGC(manager.maxLifeTime, manager.maxIdleTime)
	time.AfterFunc(time.Duration(manager.maxIdleTime)*time.Second, func() { manager.GC() })
}

// genSessionID generates unique ID for a session.
func (manager *Manager) genSessionID() string {
	source := rand.NewSource(time.Now().UnixNano())
	r := rand.New(source)
	b := make([]byte, 16)
	_, err := r.Read(b)
	if err != nil {
		return ""
	}
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}
