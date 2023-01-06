# GoLang sessions for web apps

## Usage:
```golang
import (
	"fmt"
	"context"
	"encoding/gob"				//used for custom data types encoding
	
	"github.com/dronM/session"      	//session manager
	_ "github.com/dronM/session/pg" 	//postgresql session provider
	
	"github.com/jackc/pgx/v4/pgxpool"	//pg driver		
)

// ENC_KEY holds session encrypt key
const ENC_KEY = "4gWv64T54583v8t410-45vkUiopgjw4gwmjRcGkck,ld"

// SomeStruct holds custom data type
type SomeStruct struct {
	IntVal   int
	FloatVal float32
	StrVal   string
}

func main(){
	dbpool, err := pgxpool.Connect(context.Background(), postgresql://{USER_NAME}@{HOST}:{PORT}/{DATABASE})
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

	// Starting new session with unique random ID
	currentSession, er := SessManager.SessionStart("")
	if er != nil {
		panic(fmt.Sprintf("SessionStart() fail: %v\n", err))
	}

	// Setting data
	currentSession.Set("strVal", "Some string")
	currentSession.Set("intVal", 125)
	currentSession.Set("floatVal", 3.14)	
	currentSession.Set("customVal", SomeStruct{IntVal: 375, FloatVal: 3.14, StrVal: "Some string value in struct"})
	
	// Flushing to database
	if err := currentSession.Flush(); err != nil {
		panic(fmt.Sprintf("Flush() fail: %v\n", err))
	}

	if err := SessManager.SessionClose(currentSession.SessionID()); err != nil {
		panic(fmt.Sprintf("SessionClose() fail: %v\n", err))
	}	
}
```
