package backdrop_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/husobee/backdrop"
)

func TestClearContextHandler(t *testing.T) {
	backdrop.Start(nil)
	r, _ := http.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()

	ranHandler := false
	var handler http.HandlerFunc = func(w http.ResponseWriter, r *http.Request) {
		err := backdrop.Set(r, "test", "testing")
		if err != nil {
			t.Error("failed the set: ", err.Error())
		}

		ctx := backdrop.GetContext(r)
		if v, ok := ctx.Value("test").(string); !ok || v != "testing" {
			t.Error("failed the get value: ", err.Error())
		}

		err = backdrop.SetContext(r, ctx)
		if err != nil {
			t.Error("failed the set context: ", err.Error())
		}

		v, err := backdrop.Get(r, "test")
		if err != nil {
			t.Error("failed the get value: ", err.Error())
		}
		if vv, ok := v.(string); !ok || vv != "testing" {
			t.Error("failed the get value: ", err.Error())
		}
		ranHandler = true
	}

	clearHandler := backdrop.NewClearContextHandler(handler)
	clearHandler.ServeHTTP(w, r)

	if !ranHandler {
		t.Error("failed to run handler: ")
	}

	vs, err := backdrop.Get(r, "test")
	if err == nil {
		t.Error("Should have failed the get, context should have been clear ", vs)
	}
}

func BenchmarkBackdropSet(b *testing.B) {
	backdrop.Start(nil)
	done := make(chan bool)
	r, _ := http.NewRequest("GET", "/", nil)
	for i := 0; i < b.N; i++ {
		go func() {
			backdrop.Set(r, i, "testing")
			done <- true
		}()
	}
	for i := 0; i < b.N; i++ {
		<-done
	}
}

func BenchmarkBackdropGet(b *testing.B) {
	backdrop.Start(nil)
	r, _ := http.NewRequest("GET", "/", nil)
	//	backdrop.Set(r, "test", "testing")
	done := make(chan bool)
	for i := 0; i < b.N; i++ {
		go func() {
			backdrop.GetContext(r)
			done <- true
		}()
	}
	for i := 0; i < b.N; i++ {
		<-done
	}
}
