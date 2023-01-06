// Package session_test provides testing functions for session/pg.
// Testing asumes the following:
//
//	Postgres cluster is running on localhost.
//	Database test_proj already exists with PGCRYPTO extension.
//	User postgres has no password access to the database.
package session_test

import (
	"context"
	"encoding/gob"
	"fmt"
	"reflect"
	"testing"

	"github.com/dronm/session"      //session manager
	_ "github.com/dronm/session/pg" //postgresql session provider

	"github.com/jackc/pgx/v4/pgxpool"
)

// CONN_STR holds Pg connection string in format postgresql://{USER_NAME}@{HOST}:{PORT}/{DATABASE}
const CONN_STR = "postgresql://postgres@:5432/test_proj"

// ENC_KEY holds session encoding key.
const ENC_KEY = "er4gf1tc43t84gbcfw5e4c6we5c40x5r4wfc2wrt4h54677b1hvg4e854xdwgbyujk467bv46er5014gr4w4gctk78k34r"

// SESS_KEY
//const SESS_KEY = "db788f06-898d-5969-12b1-68a82d2b5d26"

const (
	TEST_VAL_INT   = 125
	TEST_VAL_STR   = "Some string"
	TEST_VAL_FLOAT = 35.85
)

// SomeStruct custom struct for use in session.
type SomeStruct struct {
	IntVal   int
	FloatVal float32
	StrVal   string
}

func NewSomeStruct() SomeStruct {
	return SomeStruct{IntVal: 375, FloatVal: 3.14, StrVal: "Some string value in struct"}
}

// TestSession starts new session, writes some data (int, float, string, struct), flushes, then retrieves and compares.
func TestSession(t *testing.T) {
	dbpool, err := pgxpool.Connect(context.Background(), CONN_STR)
	if err != nil {
		panic(fmt.Sprintf("pgxpool.Connect() fail: %v\n", err))
	}
	defer dbpool.Close()

	//Register custom struct for marshaling.
	gob.Register(SomeStruct{})

	SessManager, er := session.NewManager("pg", 3600, 3600, dbpool, ENC_KEY)
	if er != nil {
		panic(fmt.Sprintf("NewManager() fail: %v\n", err))
	}
	
	//start new session
	currentSession, er := SessManager.SessionStart("")

	if er != nil {
		panic(fmt.Sprintf("SessionStart() fail: %v\n", err))
	}

	sid := currentSession.SessionID()
	fmt.Println("SessionID=", sid)
	
	fmt.Println("Setting string value")
	currentSession.Set("strVal", TEST_VAL_STR)
	
	fmt.Println("Setting int value")
	currentSession.Set("intVal", TEST_VAL_INT)
	
	fmt.Println("Setting string value")
	currentSession.Set("floatVal", TEST_VAL_FLOAT)
	
	fmt.Println("Setting custom value")
	currentSession.Set("structVal", NewSomeStruct())

	if err := currentSession.Flush(); err != nil {
		panic(fmt.Sprintf("Flush() fail: %v\n", err))
	}

	if err := SessManager.SessionClose(sid); err != nil {
		panic(fmt.Sprintf("SessionClose() fail: %v\n", err))
	}

	currentSession, er = SessManager.SessionStart(sid)
	if er != nil {
		panic(fmt.Sprintf("SessionStart() fail: %v\n", err))
	}
	
	if currentSession.GetInt("intVal") != TEST_VAL_INT {
		panic("Int value comparison failed")
	}
	fmt.Println("Int value compared OK")

	if currentSession.GetFloat("floatVal") != TEST_VAL_FLOAT {
		panic("float value comparison failed")
	}
	fmt.Println("float value compared OK")

	if currentSession.GetString("strVal") != TEST_VAL_STR {
		panic("string value comparison failed")
	}
	fmt.Println("string value compared OK")
	
	if !reflect.DeepEqual(currentSession.Get("structVal"), NewSomeStruct()) {
		panic("Custom value comparison failed")
	}
	fmt.Println("Custom value compared OK")
	
	if err := SessManager.SessionClose(currentSession.SessionID()); err != nil {
		panic(fmt.Sprintf("SessionClose() fail: %v\n", err))
	}	
}

