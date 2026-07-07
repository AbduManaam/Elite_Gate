package service

import (
	"errors"
	"testing"
)

func TestParsePagination(t *testing.T) {
	tests := []struct {
		name      string
		pageStr   string
		limitStr  string
		wantPage  int
		wantLimit int
		wantErr   error
	}{
		{name: "Defaults", pageStr: "", limitStr: "", wantPage: 1, wantLimit: 25, wantErr: nil},
		{name: "Custom valid values", pageStr: "2", limitStr: "10", wantPage: 2, wantLimit: 10, wantErr: nil},
		{name: "Invalid page non-numeric", pageStr: "abc", limitStr: "25", wantErr: ErrInvalidPage},
		{name: "Invalid page < 1", pageStr: "0", limitStr: "25", wantErr: ErrInvalidPage},
		{name: "Invalid limit non-numeric", pageStr: "1", limitStr: "xyz", wantErr: ErrInvalidLimit},
		{name: "Invalid limit < 1", pageStr: "1", limitStr: "0", wantErr: ErrInvalidLimit},
		{name: "Invalid limit > 100", pageStr: "1", limitStr: "101", wantErr: ErrInvalidLimit},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			page, limit, err := ParsePagination(tt.pageStr, tt.limitStr)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("expected error %v, got %v", tt.wantErr, err)
			}
			if err == nil && (page != tt.wantPage || limit != tt.wantLimit) {
				t.Errorf("expected page=%d limit=%d, got page=%d limit=%d", tt.wantPage, tt.wantLimit, page, limit)
			}
		})
	}
}

func TestParsePaginationOffset(t *testing.T) {
	page, limit, offset, err := ParsePaginationOffset("3", "10")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if page != 3 || limit != 10 || offset != 20 {
		t.Errorf("expected page=3 limit=10 offset=20, got page=%d limit=%d offset=%d", page, limit, offset)
	}
}

func TestBuildPagination(t *testing.T) {
	res := BuildPagination(2, 10, 25)
	if res.Page != 2 || res.Limit != 10 || res.Total != 25 || res.TotalPages != 3 {
		t.Errorf("incorrect pagination build: %+v", res)
	}

	resEmpty := BuildPagination(1, 25, 0)
	if resEmpty.TotalPages != 0 {
		t.Errorf("expected 0 total pages, got %d", resEmpty.TotalPages)
	}
}
