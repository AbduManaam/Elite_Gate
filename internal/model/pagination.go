package model

type Pagination struct {
	Page       int `json:"page"`
	Limit      int `json:"limit"`
	Total      int `json:"total"`
	TotalPages int `json:"total_pages"`
}

// Generic over item type — Go 1.25.4 (per go.mod) supports this comfortably,
// and it catches an Items/type mismatch at compile time instead of runtime,
// unlike an interface{}-typed field.
type PaginatedResponse[T any] struct {
	Items      []T        `json:"items"`
	Pagination Pagination `json:"pagination"`
}
