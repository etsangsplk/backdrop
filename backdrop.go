// Copyright 2015 - Husobee Associates, LLC.  All rights reserved.
// Use of this source code is governed by The MIT License, which can be found
// in the LICENSE file included.

// Package backdrop - Go Web Framework Agnostic Context Management
// We all want to retain context through our web application, and
// many are uncomfortable tying themselves to a particular framework.
// Backdrop was created to give a minimal per request context manipulation
// structure and flow control.
package backdrop

import (
	"errors"
	"fmt"
	"net/http"
	"sync"

	"golang.org/x/net/context"
)

var (
	// fetchCtx - private package ctx message channel
	fetchCtx chan fetchCtxMessage
	// setCtx - private package ctx message channel
	setCtx chan setCtxMessage
	// fetch - private package ctx message channel
	fetch chan valueMessage
	// set - private package ctx message channel
	set chan valueMessage
	// kill - private package ctx message channel
	kill chan killMessage
	// stopped - private package ctx message channel
	stopped chan bool
	// initOnce - make sure we only initialize once
	initOnce = sync.Once{}
	// contexts - global map of contexts
	contexts map[*http.Request]context.Context
	// ErrSettingToBackdrop - error when backdrop Set() is unable to set
	ErrSettingToBackdrop = errors.New("failed to set variable to backdrop")
	// ErrGettingFromBackdrop - error when backdrop Get() is unable to get
	ErrGettingFromBackdrop = errors.New("failed to get variable from backdrop")
	// ErrEvictingFromBackdrop - error when backdrop Evict() is unable to evict
	ErrEvictingFromBackdrop = errors.New("failed to evict context from backdrop")
)

const (
	// workerDoneKey - this is the global context key for when to end the workers
	workerDoneKey int = iota
	// cancelKey - this is the global key for cancel function
	cancelKey
)

// ctxMessage - a message structure for channel communications
type setCtxMessage struct {
	Request   *http.Request
	Context   context.Context
	RespondTo chan error
}

// fetchCtxMessage - a message structure for channel communications
type fetchCtxMessage struct {
	Request   *http.Request
	RespondTo chan context.Context
}

// killMessage - a message structure for channel communications
type killMessage struct {
	Request   *http.Request
	RespondTo chan error
}

// valueMessage - a message structure for channel communications
type valueMessage struct {
	Request   *http.Request
	Key       interface{}
	Value     interface{}
	RespondTo chan interface{}
}

// done - a context.CancelFunc variable to override
var done func()

// Options - structure that defines the options for Start
type Options struct {
	BufferSize int
	NumWorkers int
	Context    context.Context
}

// NewOptions - create new options
func NewOptions(ctx context.Context, numWorkers, bufferSize int) *Options {
	return &Options{
		BufferSize: bufferSize,
		NumWorkers: numWorkers,
		Context:    ctx,
	}
}

// SaneDefaults - make the defaults sane
func (o *Options) SaneDefaults() *Options {
	if o.BufferSize <= 0 {
		o.BufferSize = 1
	}
	if o.NumWorkers <= 0 {
		o.NumWorkers = 1
	}
	if o.Context == nil {
		o.Context = context.Background()
	}
	return o
}

// Start - create a new context backdrop for http requests contexts
func Start(options *Options) {
	if options == nil {
		options = NewOptions(nil, 0, 0)
	}
	options.SaneDefaults()
	startBackdrop(options.Context, options.NumWorkers, options.BufferSize)
}

// Stop - create a new context backdrop for http requests contexts
var Stop func()

// ClearContextHandler - Wrapper Handler to clear the context after
// wrapped handler runs
type ClearContextHandler struct {
	h http.Handler
}

// NewClearContextHandler - create a new ClearContextHandler
func NewClearContextHandler(h http.Handler) http.Handler {
	return &ClearContextHandler{h: h}
}

// ServeHTTP - implementation of http.Handler
func (hw *ClearContextHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	hw.h.ServeHTTP(w, r)
	Evict(r)
}

// Evict - Evict a request's context
func Evict(r *http.Request) error {
	err := make(chan error)
	kill <- killMessage{
		Request:   r,
		RespondTo: err,
	}
	return <-err
}

// GetContext - get the context associated with the request
func GetContext(r *http.Request) context.Context {
	ctx := make(chan context.Context)
	fetchCtx <- fetchCtxMessage{
		Request:   r,
		RespondTo: ctx,
	}
	return <-ctx
}

// SetContext - set the context associated with the request
func SetContext(r *http.Request, ctx context.Context) error {
	err := make(chan error)
	setCtx <- setCtxMessage{
		Request:   r,
		Context:   ctx,
		RespondTo: err,
	}
	return <-err
}

// Set - Set a variable k/v pair onto the request's context
func Set(r *http.Request, k interface{}, v interface{}) error {
	response := make(chan interface{})
	set <- valueMessage{
		Request:   r,
		Key:       k,
		Value:     v,
		RespondTo: response,
	}
	if r := <-response; r == nil {
		return ErrSettingToBackdrop
	}
	return nil
}

// Get - Get a variable k/v pair from the request's context
func Get(r *http.Request, k interface{}) (interface{}, error) {
	response := make(chan interface{})
	fetch <- valueMessage{
		Request:   r,
		Key:       k,
		RespondTo: response,
	}
	value := <-response
	if value == nil {
		return nil, ErrGettingFromBackdrop
	}
	return value, nil
}

// startBackdrop - Initalize our backdrop
func startBackdrop(ctx context.Context, workers, bufferSize int) {
	initOnce.Do(func() {
		// our operations will be fetch, set, and kill,
		fetchCtx = make(chan fetchCtxMessage, bufferSize)
		setCtx = make(chan setCtxMessage, bufferSize)
		fetch = make(chan valueMessage, bufferSize)
		set = make(chan valueMessage, bufferSize)
		kill = make(chan killMessage, bufferSize)
		stopped = make(chan bool, workers)

		contexts = make(map[*http.Request]context.Context)

		// set initial context for global context
		if ctx == nil {
			ctx = context.Background()
		}
		ctx, done = context.WithCancel(ctx)

		// setup our worker pool, collect the channels
		var workerChannels []chan bool
		for i := 0; i < workers; i++ {
			workerChannels = append(workerChannels, make(chan bool))
			go worker(ctx, workerChannels[len(workerChannels)-1])
		}

		go func() {
			// if we get word that the global context should die, stop the workers
			<-ctx.Done()
			for _, halt := range workerChannels {
				halt <- true
			}
		}()

		Stop = func() {
			done()
			for _ = range workerChannels {
				<-stopped
			}
		}
	})
}

// worker - a worker who will fetch/set to the global context as needed.
func worker(baseCtx context.Context, halt chan bool) {
Loop:
	for {
		select {

		case <-halt:
			// finish this worker
			break Loop

		case message := <-fetchCtx:
			// fetch the context alone
			if ctx, ok := contexts[message.Request]; ok && ctx != nil {
				// if there is a request context, grab the message key from it and reply
				message.RespondTo <- ctx
				continue
			}
			// a context was never created for this request, create a cancel
			// context for the caller
			ctx, cancel := context.WithCancel(baseCtx)
			ctx = context.WithValue(ctx, cancelKey, cancel)
			// set this request context on the global context
			contexts[message.Request] = ctx
			// reply to note set is finished
			message.RespondTo <- ctx

		case message := <-fetch:
			// fetch a value from the context
			if ctx, ok := contexts[message.Request]; ok && ctx != nil {
				// if there is a request context, grab the message key from it and reply
				message.RespondTo <- ctx.Value(message.Key)
				continue
			}
			message.RespondTo <- nil

		case message := <-kill:
			// evict the context from the map
			if v, exists := contexts[message.Request]; exists {
				fmt.Println(exists, contexts)
				if ctx, ok := v.(context.Context); ok && ctx != nil {

					fmt.Println("here, ctx: ", ctx)

					// if there is a request context, use it's cancel function to end that context
					if v := ctx.Value(cancelKey); v != nil {
						if cancel, ok := v.(context.CancelFunc); ok {
							cancel()
							<-ctx.Done()
							delete(contexts, message.Request)
						}
					}
				}
			}
			message.RespondTo <- nil

		case message := <-setCtx:
			// set the context outright
			if _, exists := contexts[message.Request]; exists {
				contexts[message.Request] = message.Context
			}
			message.RespondTo <- nil

		case message := <-set:
			// set a value on the context
			// get on the global for the request specific context
			if ctx, ok := contexts[message.Request]; ok && ctx != nil {
				// set the key to the existing context
				ctx = context.WithValue(ctx, message.Key, message.Value)
				contexts[message.Request] = ctx
				message.RespondTo <- ctx
				continue
			}
			ctx, cancel := context.WithCancel(baseCtx)
			// add our value to the request context associated with our key
			ctx = context.WithValue(ctx, message.Key, message.Value)
			// create a cancelable context, with the cancel function on said context
			// for easy access
			ctx = context.WithValue(ctx, cancelKey, cancel)

			// set this request context on the global context
			contexts[message.Request] = ctx
			// reply to note set is finished
			message.RespondTo <- ctx
		}
	}
	stopped <- true
}
