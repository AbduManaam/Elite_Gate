package middleware

import (
	"log"
	"net/http"
	"runtime/debug"
)

//Full stack trace

func Recovery(next http.Handler)http.Handler{
	return http.HandlerFunc(func(
		w http.ResponseWriter,
		 r *http.Request) {

			defer func(){

				if ppanic:= recover(); ppanic!=nil{
					log.Printf(
                    "Panic: %v\n%s",
					ppanic,
                    debug.Stack(),
					)
				http.Error(
					w,
					"internal server error",
					http.StatusInternalServerError,
				)	
				}
			}()
			next.ServeHTTP(w,r)
		 })
}