package httpx

import (
	"net/url"
	"strconv"
)

const (
	defaultPageSize = 20
	maxPageSize     = 100
)

// ParsePage reads page and page_size, falling back to page 1 and 20 items and
// capping the size at 100.
func ParsePage(rawQuery string) (page, size int) {
	q, _ := url.ParseQuery(rawQuery)
	page, err := strconv.Atoi(q.Get("page"))
	if err != nil || page < 1 {
		page = 1
	}
	size, err = strconv.Atoi(q.Get("page_size"))
	if err != nil || size < 1 {
		size = defaultPageSize
	}
	return page, min(size, maxPageSize)
}
