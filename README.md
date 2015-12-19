# Backdrop - Go Web Framework Agnostic Context Management

## Abstract

Context is critical for web applications.  Many frameworks solve the context
issue by locking you into their framework.  This project attempts to provide
a clean interface to use net.context within your framework agnostic application.

## Design

1. Global Context Repository protected by goroutine synchronization
2. Contexts are per request and cancelable
3. Underlying contexts are net.context.Context

## Examples

```go

package main

import (
	"fmt"
	"net/http"
	"time"

	"golang.org/x/net/context"

	"github.com/codegangsta/negroni"
	"github.com/husobee/backdrop"
	"github.com/husobee/vestigo"
	"github.com/satori/go.uuid"
	"github.com/tylerb/graceful"
)

func main() {
	// using negroni for this example to show middlewares can access context
	n := negroni.Classic()
	// start backdrop - here you can provide a backdrop.Options with a base
	// context to inherit from if you choose.
	backdrop.Start(nil)

	// set up awesome router ;)
	router := vestigo.NewRouter()
	router.Get("/:name", f)

	n.Use(&ridMiddleware{})
	// add router to middleware
	n.UseHandler(router)

	// graceful start/stop server
	srv := &graceful.Server{
		Timeout: 5 * time.Second,
		Server: &http.Server{
			Addr: ":1234",
			// top level handler needs to clear the context
			// per each request, use this wrapper handler
			Handler: backdrop.NewClearContextHandler(n),
		},
	}
	srv.ListenAndServe()
}

func f(w http.ResponseWriter, r *http.Request) {
	// get the id from the context
	id, err := backdrop.Get(r, "id")
	if err != nil {
		fmt.Println("err: ", err.Error())
	}
	fmt.Printf("request id is: %v\n", id)
	// you can also get the entire context if you are more comfortable with that
	ctx := backdrop.GetContext(r)
	ctx = context.WithValue(ctx, "key", "value")
	// and setting the newly created context in backdrop
	backdrop.SetContext(r, ctx)
}

type ridMiddleware struct{}

func (rid *ridMiddleware) ServeHTTP(w http.ResponseWriter, r *http.Request, next http.HandlerFunc) {
	// example of setting a value to the context
	backdrop.Set(r, "id", uuid.NewV4())
	next(w, r)
}


```

## Licensing

* *The MIT License* covered under this [License][backdrop-main-license].

# Contributing

If you wish to contribute, please fork this repository, submit an issue, or pull request with your suggestions.  
_Please use gofmt and golint before trying to contribute._


[backdrop-main-license]: https://github.com/husobee/backdrop/blob/master/LICENSE
