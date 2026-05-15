package middleware

import "net/http"

type MiddlewareFunc func(http.Handler)http.Handler

func Chain(
	h http.Handler,
	middleware ...MiddlewareFunc,
)http.Handler{

	for i:= len(middleware)-1; i>=0; i--{
		h=middleware[i](h)
	}
	return h
}