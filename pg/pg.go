// Package pg contains postgresql session provider based on jackc connection to PG.
// Requirements:
//	jackc connection to PG Sql https://github.com/jackc/pgx
//	PG_CRYPTO extension must be installed CREATE EXTENSION pgrypto, PGP_SYM_DECRYPT, PGP_SYM_ENCRYPT functions are used,
//	If encryption is not necessary - correct sql in SessionRead/SessionClose functions
//	Some SQL scripts are nesessary:
//		session_vals.sql contains table for holding session values
//		session_vals_process.sql trigger function for updating login information (logins table must be present in database)
//		session_vals_trigger.sql creating trigger script
package pg

import (
	"container/list"	
	"sync"
	"time"
	"context"
	"errors"
	"reflect"
	"encoding/gob"
	"bytes"
	
	"github.com/jackc/pgx/v4/pgxpool"
	"github.com/jackc/pgx/v4"
	
	"github.com/dronM/session"
)

// Session key ID length.
const SESS_ID_LEN = 36

// pder holds pointer to Provider struct.
var pder = &Provider{list: list.New()}

// storeValue holds session key-value pares.
type storeValue map[string]interface{}

// SessionStore contains session information.
type SessionStore struct {
	sid          string                     //session id
	mx sync.RWMutex
	timeAccessed time.Time                  //last modified
	timeCreated  time.Time                  //when created
	value        storeValue 		//key-value pair
	valueModified bool
}

// Set sets inmemory value. No database flush is done.
func (st *SessionStore) Set(key string, value interface{}) error {
	if !reflect.DeepEqual(st.value[key], value) {
		st.mx.Lock()
		st.value[key] = value
		st.valueModified = true
		st.mx.Unlock()
		pder.sessionUpdate(st.sid)
	}	
	return nil
}

// Flush performs the actual write to database.
func (st *SessionStore) Flush() error {
	//flush val only if it's been modified
	if st.valueModified {
		//modified
		val, err := getForDb(&st.value)
		if err != nil {
			return err
		}
		if _, err = pder.dbpool.Exec(context.Background(),
			`UPDATE session_vals
			SET
				val = pgp_sym_encrypt_bytea($1,$2),
				accessed_time = now()
			WHERE id = $3`,
			val,
			pder.encrkey,
			st.sid); err != nil {
			
			return err	
		}
		st.mx.Lock()
		st.valueModified = false
		st.mx.Unlock()
	}
	
	return nil
}

// Get returns session value by its key. Value is retrieved from memory.
func (st *SessionStore) Get(key string) interface{} {
	pder.sessionUpdate(st.sid)
	if v, ok := st.value[key]; ok {
		return v
	} else {
		return nil
	}
	return nil
}

// GetBool returns bool value by key.
func (st *SessionStore) GetBool(key string) bool {
	v := st.Get(key)
	if v != nil {
		if v_bool, ok := v.(bool); ok {
			return v_bool
		}		
	}
	return false
}

// GetString returns string value by key.
func (st *SessionStore) GetString(key string) string {
	v := st.Get(key)
	if v != nil {
		if v_str, ok := v.(string); ok {
			return v_str
			
		}else if v_str, ok := v.([]byte); ok {
			return string(v_str)
		}
	}
	return ""
}

// GetInt returns int value by key.
func (st *SessionStore) GetInt(key string) int64 {
	v := st.Get(key)
	if v != nil {
		if v_i, ok := v.(int64); ok {
			return v_i
			
		}else if v_i, ok := v.(int); ok {
			return int64(v_i)
		}
	}
	return 0
}

// GetFloat returns float value by key.
func (st *SessionStore) GetFloat(key string) float64 {
	v := st.Get(key)
	if v != nil {
		if v_f, ok := v.(float64); ok {
			return v_f
			
		}else if v_f, ok := v.(float32); ok {
			return float64(v_f)
		}
	}
	return 0
}

// GetDate returns time.Time value by key.
func (st *SessionStore) GetDate(key string) time.Time {
	v := st.Get(key)
	if v != nil {
		if v_t, ok := v.(time.Time); ok {
			return v_t			
		}
	}
	return time.Time{}
}

//Delete deletes session value from memmory by key. No flushing is done.
func (st *SessionStore) Delete(key string) error {
	delete(st.value, key)
	pder.sessionUpdate(st.sid)
	
	return nil
}

// SessionID returns session unique ID.
func (st *SessionStore) SessionID() string {
	return st.sid
}

// TimeCreated returns timeCreated property.
func (st *SessionStore) TimeCreated() time.Time {
	return st.timeCreated
}

// Provider structure holds provider information.
type Provider struct {
	lock     sync.Mutex               
	sessions map[string]*list.Element 
	list     *list.List
	dbpool	 *pgxpool.Pool
	encrkey  string	
}

func (pder *Provider) sessionMemInit(sid string) (element *list.Element, newSess session.Session) {
	pder.lock.Lock()
	defer pder.lock.Unlock()
	
	v := make(map[string]interface{}, 0)
	
	newSess = &SessionStore{
		sid: sid,
		timeAccessed: time.Now(),
		timeCreated: time.Now(),
		value: v,
	}
	element = pder.list.PushBack(newSess)
	pder.sessions[sid] = element
	return 
}

// SessionInit initializes session with given ID.
func (pder *Provider) SessionInit(sid string) (session.Session, error) {
	if pder.dbpool == nil {
		return nil, errors.New("Provider not initialized")
	}
	
	if len(sid) > SESS_ID_LEN {
		return nil, errors.New("Session key length exceeded max value")
	}
	
	_, new_sess := pder.sessionMemInit(sid)
	
	_, err := pder.dbpool.Exec(context.Background(),
		"INSERT INTO session_vals(id) VALUES($1)",
		sid,
	)	
	return new_sess, err
}

func setFromDb(strucVal *storeValue, dbVal []byte) error{
	if len(dbVal) == 0 {
		return nil
	}
	dec := gob.NewDecoder(bytes.NewBuffer(dbVal))
	if err := dec.Decode(strucVal); err != nil{
		return err		
	}
	return nil
}

func getForDb(strucVal *storeValue) ([]byte, error){
	var b bytes.Buffer
	enc := gob.NewEncoder(&b)
	err := enc.Encode(strucVal)
	if err != nil {
		return []byte{}, err
	}
	return b.Bytes(), nil
}	
	
// SessionRead reads session data from db to memory.
func (pder *Provider) SessionRead(sid string) (session.Session, error) {
	element,_ := pder.sessionMemInit(sid)
	var val []byte
	
	err := pder.dbpool.QueryRow(context.Background(),
		`SELECT
			accessed_time,
			create_time,
			pgp_sym_decrypt_bytea(val, $1)
		FROM session_vals
		WHERE id=$2`,
	pder.encrkey,
	sid).Scan(&element.Value.(*SessionStore).timeAccessed,
		&element.Value.(*SessionStore).timeCreated,
		&val)
	if err == pgx.ErrNoRows {
		//no such session
		return pder.SessionInit(sid)
		
	}else if err != nil {
		return nil, err
	}
	if err := setFromDb(&element.Value.(*SessionStore).value, val); err != nil {
		return nil, err
	}				
	return element.Value.(*SessionStore), nil
}

// SessionClose writes session data to db.
func (pder *Provider) SessionClose(sid string) error {	
	if element, ok := pder.sessions[sid]; ok {
		if err := element.Value.(*SessionStore).Flush(); err != nil {
			return err
		}
	}

	return nil
}

// SessionDestroy destoys session by its ID.
func (pder *Provider) SessionDestroy(sid string) error {
	return pder.removeSession(sid)
}

// SessionGC 
func (pder *Provider) SessionGC(maxLifeTime int64, maxIdleTime int64) {
	pder.lock.Lock()
	defer pder.lock.Unlock()

	for {
		element := pder.list.Back()
		if element == nil {
			break
		}
		tm := time.Now().Unix()
		if ((element.Value.(*SessionStore).timeCreated.Unix() + maxLifeTime) < tm) || ((element.Value.(*SessionStore).timeAccessed.Unix() + maxIdleTime) < tm) {
			//pder.list.Remove(element)
			//delete(pder.sessions, element.Value.(*SessionStore).sid)
			pder.removeSession(element.Value.(*SessionStore).sid)
		} else {
			break
		}
	}
}

// InitProvider initializes postgresql provider.
// Function expects two parameters:
// 	First parameter: ConnectionString string in pg format: postgresql://{USER_NAME}@{HOST}:{PORT}/{DATABASE}
// 	Second parameter: encryptKey application unique,if to set no encryption used
func (pder *Provider) InitProvider(provParams []interface{}) (err error) {
	if len(provParams)<2 {
		return errors.New("Missing parameters: pgxpool.Pool, encryptKey")
	}	
	//pder.dbpool, err = pgxpool.Connect(context.Background(), provParams[0].(string))
	pder.dbpool = provParams[0].(*pgxpool.Pool)
	
	pder.encrkey = provParams[1].(string)
	
	return err
}


//helper function for SessionDestroy and SessionGC
func (pder *Provider) removeSession(sid string) (error) {	
	if _, err := pder.dbpool.Exec(context.Background(), `DELETE FROM session_vals WHERE id=$1`, sid); err != nil {
		return err
	}

	if el, ok := pder.sessions[sid]; ok {
		delete(pder.sessions, sid)
		pder.list.Remove(el)
	}
	return nil
}

//protected
func (pder *Provider) sessionUpdate(sid string) error {
	pder.lock.Lock()
	defer pder.lock.Unlock()
	if element, ok := pder.sessions[sid]; ok {
		element.Value.(*SessionStore).timeAccessed = time.Now()
		pder.list.MoveToFront(element)
		return nil
	}
	return nil
}

func (pder *Provider) GetSessionIDLen() int {
	return SESS_ID_LEN
}



func init() {
	pder.sessions = make(map[string]*list.Element, 0)
	session.Register("pg", pder)
}
