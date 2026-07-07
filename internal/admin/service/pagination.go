package service

import (
	"errors"
	"strconv"

	"elitegate/internal/model"
)

var (
	ErrInvalidPage  = errors.New("page must be greater than or equal to 1")
	ErrInvalidLimit = errors.New("limit must be greater than 0 and less than or equal to 100")
)

func ParsePagination(pageStr, limitStr string) (int, int, error) {
	page := 1
	if pageStr != "" {
		p, err := strconv.Atoi(pageStr)
		if err != nil || p < 1 {
			return 0, 0, ErrInvalidPage
		}
		page = p
	}

	limit := 25
	if limitStr != "" {
		l, err := strconv.Atoi(limitStr)
		if err != nil || l < 1 || l > 100 {
			return 0, 0, ErrInvalidLimit
		}
		limit = l
	}

	return page, limit, nil
}

// ParsePaginationOffset wraps ParsePagination and folds in the offset
// calculation every handler needs, so that formula lives in exactly one
// place instead of being repeated across 8 handler files.
func ParsePaginationOffset(pageStr, limitStr string) (page, limit, offset int, err error) {
	page, limit, err = ParsePagination(pageStr, limitStr)
	if err != nil {
		return 0, 0, 0, err
	}
	return page, limit, (page - 1) * limit, nil
}

func BuildPagination(page, limit, total int) model.Pagination {
	totalPages := 0
	if total > 0 && limit > 0 {
		totalPages = total / limit
		if total%limit != 0 {
			totalPages++
		}
	}
	return model.Pagination{
		Page:       page,
		Limit:      limit,
		Total:      total,
		TotalPages: totalPages,
	}
}
